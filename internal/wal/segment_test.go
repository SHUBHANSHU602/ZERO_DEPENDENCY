package wal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSegmentWriterAppendTracksSize(t *testing.T) {
	dir := t.TempDir()
	writer, err := NewSegmentWriter(dir, 1, 1<<20)
	if err != nil {
		t.Fatalf("NewSegmentWriter() error = %v", err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	records := []Record{
		{Type: RecordPut, Timestamp: 1, Key: []byte("alpha"), Value: []byte("one")},
		{Type: RecordPut, Timestamp: 2, Key: []byte("beta"), Value: []byte("two")},
		{Type: RecordDelete, Timestamp: 3, Key: []byte("alpha")},
	}

	var wantSize int64
	for _, record := range records {
		encoded, err := EncodeRecord(record)
		if err != nil {
			t.Fatalf("EncodeRecord() error = %v", err)
		}
		offset, err := writer.Append(record)
		if err != nil {
			t.Fatalf("Append() error = %v", err)
		}
		if offset != wantSize {
			t.Errorf("Append() offset = %d, want %d", offset, wantSize)
		}
		wantSize += int64(len(encoded))
		if got := writer.Size(); got != wantSize {
			t.Errorf("Size() = %d, want %d", got, wantSize)
		}
	}

	info, err := os.Stat(filepath.Join(dir, "000001.seg"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Size(); got != wantSize {
		t.Errorf("file size = %d, want %d", got, wantSize)
	}
	if writer.ShouldRoll() {
		t.Error("ShouldRoll() = true below threshold")
	}
}

func TestSegmentManagerRollsAtThreshold(t *testing.T) {
	dir := t.TempDir()
	record := Record{
		Type:      RecordPut,
		Timestamp: 10,
		Key:       []byte("key"),
		Value:     []byte("value"),
	}
	encoded, err := EncodeRecord(record)
	if err != nil {
		t.Fatalf("EncodeRecord() error = %v", err)
	}
	threshold := int64(len(encoded) * 2)

	manager, err := NewSegmentManager(dir, threshold)
	if err != nil {
		t.Fatalf("NewSegmentManager() error = %v", err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	for i := 0; i < 2; i++ {
		segment, offset, err := manager.Append(record)
		if err != nil {
			t.Fatalf("Append() error = %v", err)
		}
		if segment != 1 {
			t.Errorf("Append() segment = %d, want 1", segment)
		}
		wantOffset := int64(i * len(encoded))
		if offset != wantOffset {
			t.Errorf("Append() offset = %d, want %d", offset, wantOffset)
		}
	}

	if got := manager.ActiveSegmentNumber(); got != 2 {
		t.Errorf("ActiveSegmentNumber() = %d, want 2", got)
	}
	segments, err := ListSegments(dir)
	if err != nil {
		t.Fatalf("ListSegments() error = %v", err)
	}
	wantNames := []string{"000001.seg", "000002.seg"}
	if len(segments) != len(wantNames) {
		t.Fatalf("ListSegments() returned %d segments, want %d", len(segments), len(wantNames))
	}
	for i, path := range segments {
		if got := filepath.Base(path); got != wantNames[i] {
			t.Errorf("segment %d name = %q, want %q", i, got, wantNames[i])
		}
	}

	firstInfo, err := os.Stat(segments[0])
	if err != nil {
		t.Fatalf("Stat(first segment) error = %v", err)
	}
	if got, want := firstInfo.Size(), threshold; got != want {
		t.Errorf("first segment size = %d, want %d", got, want)
	}
	secondInfo, err := os.Stat(segments[1])
	if err != nil {
		t.Fatalf("Stat(second segment) error = %v", err)
	}
	if got := secondInfo.Size(); got != 0 {
		t.Errorf("second segment size = %d, want 0", got)
	}
}
