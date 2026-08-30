package wal

import (
	"bytes"
	"testing"
)

func TestRecordRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		record Record
	}{
		{
			name: "put",
			record: Record{
				Type:      RecordPut,
				Timestamp: 1_725_000_000_123_456_789,
				Key:       []byte("user:42"),
				Value:     []byte("active"),
			},
		},
		{
			name: "delete",
			record: Record{
				Type:      RecordDelete,
				Timestamp: 1_725_000_001_987_654_321,
				Key:       []byte("user:42"),
			},
		},
		{
			name: "empty value",
			record: Record{
				Type:      RecordPut,
				Timestamp: 123,
				Key:       []byte("empty"),
				Value:     []byte{},
			},
		},
		{
			name: "large value",
			record: Record{
				Type:      RecordPut,
				Timestamp: 456,
				Key:       []byte("large"),
				Value:     bytes.Repeat([]byte{0x5a}, 1<<20),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncodeRecord(tt.record)
			if err != nil {
				t.Fatalf("EncodeRecord() error = %v", err)
			}

			got, err := DecodeRecord(bytes.NewReader(encoded))
			if err != nil {
				t.Fatalf("DecodeRecord() error = %v", err)
			}
			if got.Type != tt.record.Type {
				t.Errorf("Type = %d, want %d", got.Type, tt.record.Type)
			}
			if got.Timestamp != tt.record.Timestamp {
				t.Errorf("Timestamp = %d, want %d", got.Timestamp, tt.record.Timestamp)
			}
			if !bytes.Equal(got.Key, tt.record.Key) {
				t.Errorf("Key = %q, want %q", got.Key, tt.record.Key)
			}
			if !bytes.Equal(got.Value, tt.record.Value) {
				t.Errorf("Value length = %d, want %d", len(got.Value), len(tt.record.Value))
			}
		})
	}
}

func TestDecodeRecordChecksumMismatch(t *testing.T) {
	encoded, err := EncodeRecord(Record{
		Type:      RecordPut,
		Timestamp: 789,
		Key:       []byte("key"),
		Value:     []byte("value"),
	})
	if err != nil {
		t.Fatalf("EncodeRecord() error = %v", err)
	}
	encoded[len(encoded)-1] ^= 0xff

	_, err = DecodeRecord(bytes.NewReader(encoded))
	checksumErr, ok := err.(*ChecksumError)
	if !ok {
		t.Fatalf("DecodeRecord() error = %T, want *ChecksumError", err)
	}
	if checksumErr.Expected == checksumErr.Actual {
		t.Fatal("ChecksumError reports matching checksums")
	}
}
