// Package pulselog provides a zero-dependency, crash-safe embedded key-value store.
package pulselog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
	"uuid"

	"pulselog/internal/compaction"
	"pulselog/internal/index"
	"pulselog/internal/wal"
)

// ErrNotFound is returned when a key does not exist in the database.
var ErrNotFound = errors.New("pulselog: key not found")

// Record is a live key-value record returned by RangeQuery.
type Record struct {
	ID        uuid.UUID
	Key       string
	Value     []byte
	Timestamp time.Time
}

type storedValue struct {
	ID    uuid.UUID `json:"id"`
	Value []byte    `json:"value"`
}

// DB is a crash-safe, append-only key-value store.
type DB struct {
	mu       sync.RWMutex
	dir      string
	index    *index.Index
	segments *wal.SegmentManager
	closed   bool
}

// Open opens or creates a database in dir and rebuilds its in-memory index.
func Open(dir string) (*DB, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	idx := index.New()
	records, err := wal.ReplayWithMetadata(dir)
	if err != nil {
		return nil, err
	}
	for _, replayed := range records {
		if replayed.SegmentID > math.MaxUint32 {
			return nil, fmt.Errorf("pulselog: segment ID %d exceeds index capacity", replayed.SegmentID)
		}
		key := string(replayed.Record.Key)
		if replayed.Record.Type == wal.RecordDelete {
			idx.Delete(key)
			continue
		}
		idx.Set(key, index.Entry{
			SegmentID: uint32(replayed.SegmentID),
			Offset:    replayed.Offset,
			Length:    replayed.Length,
			Timestamp: replayed.Record.Timestamp,
		})
	}

	segments, err := wal.NewSegmentManager(dir, wal.DefaultSegmentSize)
	if err != nil {
		return nil, err
	}
	if segments.ActiveSegmentNumber() > math.MaxUint32 {
		_ = segments.Close()
		return nil, errors.New("pulselog: active segment ID exceeds index capacity")
	}
	return &DB{dir: dir, index: idx, segments: segments}, nil
}

// Put stores value for key. The write is durable before Put returns.
func (db *DB) Put(key string, value []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return os.ErrClosed
	}
	// Package Killer: replaces github.com/google/uuid with the Go 1.27 standard library uuid package.
	valueBytes, err := json.Marshal(storedValue{ID: uuid.NewV7(), Value: value})
	if err != nil {
		return err
	}
	record := wal.Record{Type: wal.RecordPut, Timestamp: time.Now().UnixNano(), Key: []byte(key), Value: valueBytes}
	segmentID, offset, length, err := db.segments.Append(record)
	if err != nil {
		return err
	}
	if segmentID > math.MaxUint32 {
		return fmt.Errorf("pulselog: segment ID %d exceeds index capacity", segmentID)
	}
	db.index.Set(key, index.Entry{
		SegmentID: uint32(segmentID),
		Offset:    offset,
		Length:    uint32(length),
		Timestamp: record.Timestamp,
	})
	return nil
}

// Delete durably records a tombstone and removes key from the live index.
func (db *DB) Delete(key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return os.ErrClosed
	}
	record := wal.Record{Type: wal.RecordDelete, Timestamp: time.Now().UnixNano(), Key: []byte(key)}
	if _, _, _, err := db.segments.Append(record); err != nil {
		return err
	}
	db.index.Delete(key)
	return nil
}

// Get returns a copy of the value stored for key.
func (db *DB) Get(key string) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return nil, os.ErrClosed
	}
	entry, ok := db.index.Get(key)
	if !ok {
		return nil, ErrNotFound
	}
	record, err := db.readRecord(key, entry)
	if err != nil {
		return nil, err
	}
	return record.Value, nil
}

// RangeQuery returns live records whose timestamps fall within [from, to],
// ordered by timestamp ascending.
func (db *DB) RangeQuery(from, to time.Time) ([]Record, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return nil, os.ErrClosed
	}
	fromUnix, toUnix := from.UnixNano(), to.UnixNano()
	if fromUnix > toUnix {
		return []Record{}, nil
	}

	entries := db.index.Snapshot()
	records := make([]Record, 0)
	for key, entry := range entries {
		if entry.Timestamp < fromUnix || entry.Timestamp > toUnix {
			continue
		}
		record, err := db.readRecord(key, entry)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Timestamp.Equal(records[j].Timestamp) {
			return records[i].Key < records[j].Key
		}
		return records[i].Timestamp.Before(records[j].Timestamp)
	})
	return records, nil
}

// Compact reclaims dead records from every segment except the active one.
// It holds db.mu exclusively for the full operation, serializing compaction
// with Put, Get, Delete, RangeQuery, and Close.
func (db *DB) Compact() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return os.ErrClosed
	}

	paths, err := wal.ListSegments(db.dir)
	if err != nil {
		return err
	}
	active := db.segments.ActiveSegmentNumber()
	oldPaths := make([]string, 0, len(paths))
	oldSegments := make(map[uint32]bool)
	for _, path := range paths {
		segmentID, err := segmentIDFromPath(path)
		if err != nil {
			return err
		}
		if uint64(segmentID) == active {
			continue
		}
		oldPaths = append(oldPaths, path)
		oldSegments[segmentID] = true
	}
	if len(oldPaths) == 0 {
		return nil
	}
	if active == math.MaxUint32 {
		return errors.New("pulselog: segment ID exhausted")
	}

	live := make([]compaction.LiveRecord, 0)
	for key, entry := range db.index.Snapshot() {
		if oldSegments[entry.SegmentID] {
			live = append(live, compaction.LiveRecord{Key: key, Entry: entry})
		}
	}
	sort.Slice(live, func(i, j int) bool {
		if live[i].Entry.SegmentID == live[j].Entry.SegmentID {
			return live[i].Entry.Offset < live[j].Entry.Offset
		}
		return live[i].Entry.SegmentID < live[j].Entry.SegmentID
	})

	newSegment := active + 1
	entries, err := compaction.Merge(db.dir, newSegment, live)
	if err != nil {
		return err
	}
	if err := db.segments.Activate(newSegment); err != nil {
		if path, pathErr := wal.SegmentPath(db.dir, newSegment); pathErr == nil {
			_ = os.Remove(path)
		}
		return err
	}
	db.index.Update(entries)
	var removeErrors []error
	for _, path := range oldPaths {
		if err := os.Remove(path); err != nil {
			removeErrors = append(removeErrors, fmt.Errorf("remove compacted segment %q: %w", path, err))
		}
	}
	return errors.Join(removeErrors...)
}

func segmentIDFromPath(path string) (uint32, error) {
	var segmentID uint64
	name := filepath.Base(path)
	if _, err := fmt.Sscanf(name, "%d.seg", &segmentID); err != nil || segmentID == 0 || segmentID > math.MaxUint32 {
		return 0, fmt.Errorf("pulselog: invalid segment filename %q", name)
	}
	return uint32(segmentID), nil
}

func (db *DB) readRecord(key string, entry index.Entry) (Record, error) {
	path, err := wal.SegmentPath(db.dir, uint64(entry.SegmentID))
	if err != nil {
		return Record{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Record{}, err
	}
	defer file.Close()
	if _, err := file.Seek(entry.Offset, io.SeekStart); err != nil {
		return Record{}, err
	}
	wRecord, err := wal.DecodeRecord(io.LimitReader(file, int64(entry.Length)))
	if err != nil {
		return Record{}, err
	}
	if wRecord.Type != wal.RecordPut || !bytes.Equal(wRecord.Key, []byte(key)) {
		return Record{}, fmt.Errorf("pulselog: index entry for %q does not match WAL record", key)
	}
	var stored storedValue
	if err := json.Unmarshal(wRecord.Value, &stored); err != nil {
		return Record{}, fmt.Errorf("pulselog: decode value metadata for %q: %w", key, err)
	}
	return Record{
		ID:        stored.ID,
		Key:       key,
		Value:     append([]byte(nil), stored.Value...),
		Timestamp: time.Unix(0, wRecord.Timestamp),
	}, nil
}

// Close closes the active WAL segment. It is safe to call Close more than once.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return nil
	}
	db.closed = true
	return db.segments.Close()
}
