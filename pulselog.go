// Package pulselog provides a zero-dependency, crash-safe embedded key-value store.
package pulselog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"pulselog/internal/index"
	"pulselog/internal/wal"
)

// ErrNotFound is returned when a key does not exist in the database.
var ErrNotFound = errors.New("pulselog: key not found")

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
		idx.Set(key, index.Entry{SegmentID: uint32(replayed.SegmentID), Offset: replayed.Offset, Length: replayed.Length})
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
	record := wal.Record{Type: wal.RecordPut, Timestamp: time.Now().UnixNano(), Key: []byte(key), Value: value}
	encoded, err := wal.EncodeRecord(record)
	if err != nil {
		return err
	}
	segmentID, offset, err := db.segments.Append(record)
	if err != nil {
		return err
	}
	if segmentID > math.MaxUint32 {
		return fmt.Errorf("pulselog: segment ID %d exceeds index capacity", segmentID)
	}
	db.index.Set(key, index.Entry{SegmentID: uint32(segmentID), Offset: offset, Length: uint32(len(encoded))})
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
	path, err := wal.SegmentPath(db.dir, uint64(entry.SegmentID))
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if _, err := file.Seek(entry.Offset, io.SeekStart); err != nil {
		return nil, err
	}
	record, err := wal.DecodeRecord(io.LimitReader(file, int64(entry.Length)))
	if err != nil {
		return nil, err
	}
	if record.Type != wal.RecordPut || !bytes.Equal(record.Key, []byte(key)) {
		return nil, fmt.Errorf("pulselog: index entry for %q does not match WAL record", key)
	}
	return append([]byte(nil), record.Value...), nil
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
