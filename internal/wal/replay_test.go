package wal

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReplayTruncatesTornWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, segmentFilename(firstSegmentNumber))
	records := []Record{
		{Type: RecordPut, Timestamp: 1, Key: []byte("alpha"), Value: []byte("one")},
		{Type: RecordPut, Timestamp: 2, Key: []byte("beta"), Value: []byte("two")},
		{Type: RecordDelete, Timestamp: 3, Key: []byte("alpha")},
	}

	var segment bytes.Buffer
	for _, record := range records {
		encoded, err := EncodeRecord(record)
		if err != nil {
			t.Fatalf("EncodeRecord() error = %v", err)
		}
		segment.Write(encoded)
	}
	validSize := int64(segment.Len())

	torn, err := EncodeRecord(Record{
		Type: RecordPut, Timestamp: 4, Key: []byte("gamma"), Value: []byte("incomplete"),
	})
	if err != nil {
		t.Fatalf("EncodeRecord(torn record) error = %v", err)
	}
	segment.Write(torn[:len(torn)-3])
	if err := os.WriteFile(path, segment.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := Replay(dir)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if !reflect.DeepEqual(got, records) {
		t.Fatalf("Replay() records = %#v, want %#v", got, records)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Size() != validSize {
		t.Fatalf("segment size after Replay() = %d, want %d", info.Size(), validSize)
	}
}
