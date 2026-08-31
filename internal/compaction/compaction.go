// Package compaction implements durable WAL segment merging.
package compaction

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"pulselog/internal/index"
	"pulselog/internal/wal"
)

// LiveRecord identifies a live WAL record that should be copied forward.
type LiveRecord struct {
	Key   string
	Entry index.Entry
}

// Merge copies live records into one durable segment. It does not modify the
// source segments or the caller's index.
func Merge(dir string, segmentID uint64, live []LiveRecord) (map[string]index.Entry, error) {
	target, err := wal.SegmentPath(dir, segmentID)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(target); err == nil {
		return nil, fmt.Errorf("compaction: target segment already exists: %s", target)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	temporary, err := os.CreateTemp(dir, ".compact-*")
	if err != nil {
		return nil, err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()

	entries := make(map[string]index.Entry, len(live))
	var offset int64
	for _, item := range live {
		record, err := readRecord(dir, item)
		if err != nil {
			return nil, err
		}
		encoded, err := wal.EncodeRecord(record)
		if err != nil {
			return nil, err
		}
		if err := writeAll(temporary, encoded); err != nil {
			return nil, err
		}
		entries[item.Key] = index.Entry{
			SegmentID: uint32(segmentID),
			Offset:    offset,
			Length:    uint32(len(encoded)),
			Timestamp: record.Timestamp,
		}
		offset += int64(len(encoded))
	}
	if err := temporary.Sync(); err != nil {
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return nil, err
	}
	committed = true
	return entries, nil
}

func readRecord(dir string, item LiveRecord) (wal.Record, error) {
	path, err := wal.SegmentPath(dir, uint64(item.Entry.SegmentID))
	if err != nil {
		return wal.Record{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return wal.Record{}, err
	}
	defer file.Close()
	if _, err := file.Seek(item.Entry.Offset, io.SeekStart); err != nil {
		return wal.Record{}, err
	}
	record, err := wal.DecodeRecord(io.LimitReader(file, int64(item.Entry.Length)))
	if err != nil {
		return wal.Record{}, err
	}
	if record.Type != wal.RecordPut || !bytes.Equal(record.Key, []byte(item.Key)) {
		return wal.Record{}, fmt.Errorf("compaction: index entry for %q does not match WAL record", item.Key)
	}
	return record, nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		data = data[n:]
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
