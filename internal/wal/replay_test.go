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
		{Type: RecordDelete, Timestamp: 3, Key: []byte("alpha"), Value: []byte{}},
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

func TestReplayTruncatesHeaderWithoutPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, segmentFilename(firstSegmentNumber))
	valid := Record{
		Type: RecordPut, Timestamp: 1, Key: []byte("alpha"), Value: []byte("one"),
	}
	validEncoded, err := EncodeRecord(valid)
	if err != nil {
		t.Fatalf("EncodeRecord(valid record) error = %v", err)
	}
	tornEncoded, err := EncodeRecord(Record{
		Type: RecordPut, Timestamp: 2, Key: []byte("beta"), Value: []byte("missing"),
	})
	if err != nil {
		t.Fatalf("EncodeRecord(torn record) error = %v", err)
	}

	segment := append(append([]byte(nil), validEncoded...), tornEncoded[:recordHeaderSize]...)
	if err := os.WriteFile(path, segment, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := Replay(dir)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	want := []Record{valid}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Replay() records = %#v, want %#v", got, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got, want := info.Size(), int64(len(validEncoded)); got != want {
		t.Fatalf("segment size after Replay() = %d, want %d", got, want)
	}
}

func TestReplayAcrossMultipleSegments(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, segmentFilename(1))
	secondPath := filepath.Join(dir, segmentFilename(2))
	firstRecords := []Record{
		{Type: RecordPut, Timestamp: 1, Key: []byte("alpha"), Value: []byte("one")},
		{Type: RecordPut, Timestamp: 2, Key: []byte("beta"), Value: []byte("two")},
	}
	secondRecords := []Record{
		{Type: RecordDelete, Timestamp: 3, Key: []byte("alpha"), Value: []byte{}},
		{Type: RecordPut, Timestamp: 4, Key: []byte("gamma"), Value: []byte("three")},
	}

	var firstSegment bytes.Buffer
	for _, record := range firstRecords {
		encoded, err := EncodeRecord(record)
		if err != nil {
			t.Fatalf("EncodeRecord(first segment) error = %v", err)
		}
		firstSegment.Write(encoded)
	}
	firstSize := int64(firstSegment.Len())
	if err := os.WriteFile(firstPath, firstSegment.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile(first segment) error = %v", err)
	}

	var secondSegment bytes.Buffer
	for _, record := range secondRecords {
		encoded, err := EncodeRecord(record)
		if err != nil {
			t.Fatalf("EncodeRecord(second segment) error = %v", err)
		}
		secondSegment.Write(encoded)
	}
	secondValidSize := int64(secondSegment.Len())
	tornEncoded, err := EncodeRecord(Record{
		Type: RecordPut, Timestamp: 5, Key: []byte("delta"), Value: []byte("missing"),
	})
	if err != nil {
		t.Fatalf("EncodeRecord(torn record) error = %v", err)
	}
	secondSegment.Write(tornEncoded[:recordHeaderSize])
	if err := os.WriteFile(secondPath, secondSegment.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile(second segment) error = %v", err)
	}

	got, err := Replay(dir)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	want := append(append([]Record(nil), firstRecords...), secondRecords...)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Replay() records = %#v, want %#v", got, want)
	}

	firstInfo, err := os.Stat(firstPath)
	if err != nil {
		t.Fatalf("Stat(first segment) error = %v", err)
	}
	if firstInfo.Size() != firstSize {
		t.Fatalf("first segment size after Replay() = %d, want %d", firstInfo.Size(), firstSize)
	}
	secondInfo, err := os.Stat(secondPath)
	if err != nil {
		t.Fatalf("Stat(second segment) error = %v", err)
	}
	if secondInfo.Size() != secondValidSize {
		t.Fatalf("second segment size after Replay() = %d, want %d", secondInfo.Size(), secondValidSize)
	}
}
