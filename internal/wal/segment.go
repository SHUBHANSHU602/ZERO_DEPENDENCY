package wal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	// DefaultSegmentSize is the size at which a segment manager rolls to a
	// new segment when no other threshold is configured.
	DefaultSegmentSize int64 = 4 << 20

	firstSegmentNumber uint64 = 1
	segmentExtension          = ".seg"
)

// SegmentWriter appends records to one numbered WAL segment.
type SegmentWriter struct {
	mu        sync.Mutex
	file      *os.File
	number    uint64
	size      int64
	threshold int64
}

// NewSegmentWriter opens or creates a numbered segment in dir. A threshold at
// or below zero selects DefaultSegmentSize.
func NewSegmentWriter(dir string, number uint64, threshold int64) (*SegmentWriter, error) {
	if number == 0 {
		return nil, InvalidRecordError("segment number must be positive")
	}
	if threshold <= 0 {
		threshold = DefaultSegmentSize
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	path := filepath.Join(dir, segmentFilename(number))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	return &SegmentWriter{
		file:      file,
		number:    number,
		size:      info.Size(),
		threshold: threshold,
	}, nil
}

// Append encodes rec and appends it to the segment, returning the record's
// starting byte offset. A write is not considered durable until the file's
// Sync method returns successfully; Append therefore calls Sync before it
// reports success.
func (w *SegmentWriter) Append(rec Record) (offset int64, err error) {
	encoded, err := EncodeRecord(rec)
	if err != nil {
		return 0, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}

	offset = w.size
	written, err := writeAll(w.file, encoded)
	w.size += int64(written)
	if err != nil {
		return offset, err
	}
	if err := w.file.Sync(); err != nil {
		return offset, err
	}
	return offset, nil
}

// Size returns the current size of the segment in bytes.
func (w *SegmentWriter) Size() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.size
}

// ShouldRoll reports whether the segment has reached its configured size
// threshold. The record that crosses the threshold remains in this segment.
func (w *SegmentWriter) ShouldRoll() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.size >= w.threshold
}

// Number returns the segment's sequence number.
func (w *SegmentWriter) Number() uint64 {
	return w.number
}

// Close closes the segment file.
func (w *SegmentWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// SegmentManager owns the active WAL segment and rolls it when its configured
// size threshold is reached. It deliberately uses no separate manifest file:
// the active segment is derived by scanning the directory for the
// highest-numbered segment, avoiding a second piece of state that could drift
// out of sync with the segment files after a crash.
type SegmentManager struct {
	mu        sync.Mutex
	dir       string
	threshold int64
	active    *SegmentWriter
}

// NewSegmentManager opens the most recent segment in dir, or creates segment
// 000001.seg when the directory contains no segments. A threshold at or below
// zero selects DefaultSegmentSize.
func NewSegmentManager(dir string, threshold int64) (*SegmentManager, error) {
	if threshold <= 0 {
		threshold = DefaultSegmentSize
	}
	segments, err := ListSegments(dir)
	if err != nil {
		return nil, err
	}

	number := firstSegmentNumber
	if len(segments) > 0 {
		number, err = segmentNumber(filepath.Base(segments[len(segments)-1]))
		if err != nil {
			return nil, err
		}
	}
	active, err := NewSegmentWriter(dir, number, threshold)
	if err != nil {
		return nil, err
	}

	manager := &SegmentManager{
		dir:       dir,
		threshold: threshold,
		active:    active,
	}
	if active.ShouldRoll() {
		if err := manager.roll(); err != nil {
			_ = active.Close()
			return nil, err
		}
	}
	return manager, nil
}

// Append writes rec durably to the active segment. It returns the segment
// number and byte offset at which the record was written. If the append reaches
// the size threshold, Append creates the next segment before returning.
func (m *SegmentManager) Append(rec Record) (segment uint64, offset int64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return 0, 0, os.ErrClosed
	}

	segment = m.active.Number()
	offset, err = m.active.Append(rec)
	if err != nil {
		return segment, offset, err
	}
	if m.active.ShouldRoll() {
		if err := m.roll(); err != nil {
			return segment, offset, err
		}
	}
	return segment, offset, nil
}

// ActiveSegmentNumber returns the sequence number of the active segment. It
// returns zero after the manager has been closed.
func (m *SegmentManager) ActiveSegmentNumber() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return 0
	}
	return m.active.Number()
}

// Close closes the active segment.
func (m *SegmentManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return nil
	}
	err := m.active.Close()
	m.active = nil
	return err
}

// ListSegments returns paths to valid numbered segment files in dir, ordered
// by ascending segment number. Files not matching the segment naming scheme
// are ignored. A missing directory is treated as empty.
func ListSegments(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	type numberedPath struct {
		number uint64
		path   string
	}
	segments := make([]numberedPath, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		number, err := segmentNumber(entry.Name())
		if err != nil {
			continue
		}
		segments = append(segments, numberedPath{
			number: number,
			path:   filepath.Join(dir, entry.Name()),
		})
	}
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].number < segments[j].number
	})

	paths := make([]string, len(segments))
	for i := range segments {
		paths[i] = segments[i].path
	}
	return paths, nil
}

// SegmentPath returns the path for a numbered WAL segment.
func SegmentPath(dir string, number uint64) (string, error) {
	if number == 0 {
		return "", InvalidRecordError("segment number must be positive")
	}
	return filepath.Join(dir, segmentFilename(number)), nil
}

func (m *SegmentManager) roll() error {
	nextNumber := m.active.Number() + 1
	if nextNumber == 0 {
		return InvalidRecordError("segment number exhausted")
	}
	if err := m.active.Close(); err != nil {
		return err
	}

	next, err := NewSegmentWriter(m.dir, nextNumber, m.threshold)
	if err != nil {
		m.active = nil
		return err
	}
	m.active = next
	return nil
}

func segmentFilename(number uint64) string {
	return fmt.Sprintf("%06d%s", number, segmentExtension)
}

func segmentNumber(name string) (uint64, error) {
	if !strings.HasSuffix(name, segmentExtension) {
		return 0, InvalidRecordError("invalid segment filename")
	}
	stem := strings.TrimSuffix(name, segmentExtension)
	if len(stem) < 6 {
		return 0, InvalidRecordError("invalid segment filename")
	}
	number, err := strconv.ParseUint(stem, 10, 64)
	if err != nil || number == 0 || segmentFilename(number) != name {
		return 0, InvalidRecordError("invalid segment filename")
	}
	return number, nil
}

func writeAll(writer io.Writer, data []byte) (int, error) {
	written := 0
	for len(data) > 0 {
		n, err := writer.Write(data)
		written += n
		data = data[n:]
		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}
