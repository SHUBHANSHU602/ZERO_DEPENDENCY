// Package wal provides the write-ahead log for pulselog.
package wal

import (
	"encoding/binary"
	"hash/crc32"
	"io"
)

const (
	// RecordPut identifies a record that stores a value for a key.
	RecordPut byte = 0x01
	// RecordDelete identifies a record that removes a key.
	RecordDelete byte = 0x02

	recordHeaderSize = 4 + 4 + 1 + 8 + 2
	maxPayloadLength = 16 << 20
	maxUint16        = 1<<16 - 1
	maxUint32        = 1<<32 - 1
)

// Record is a single mutation stored in the write-ahead log.
type Record struct {
	Type      byte
	Timestamp int64
	Key       []byte
	Value     []byte
}

// ChecksumError reports corruption in an otherwise complete record. It lets
// callers distinguish a torn or corrupted write from an error reading the WAL.
type ChecksumError struct {
	Expected uint32
	Actual   uint32
}

// Error implements error.
func (*ChecksumError) Error() string {
	return "wal: record checksum mismatch"
}

// InvalidRecordError reports a record whose fields cannot be represented by
// the on-disk format.
type InvalidRecordError string

// Error implements error.
func (err InvalidRecordError) Error() string {
	return "wal: invalid record: " + string(err)
}

// EncodeRecord encodes rec in the WAL's binary on-disk format.
func EncodeRecord(rec Record) ([]byte, error) {
	if err := validateType(rec.Type); err != nil {
		return nil, err
	}
	if len(rec.Key) > maxUint16 {
		return nil, InvalidRecordError("key exceeds uint16 length")
	}
	if uint64(len(rec.Key))+uint64(len(rec.Value)) > maxUint32 {
		return nil, InvalidRecordError("payload exceeds uint32 length")
	}

	payloadLength := len(rec.Key) + len(rec.Value)
	encoded := make([]byte, recordHeaderSize+payloadLength)
	binary.LittleEndian.PutUint32(encoded[0:4], uint32(payloadLength))
	encoded[8] = rec.Type
	binary.LittleEndian.PutUint64(encoded[9:17], uint64(rec.Timestamp))
	binary.LittleEndian.PutUint16(encoded[17:19], uint16(len(rec.Key)))
	copy(encoded[recordHeaderSize:], rec.Key)
	copy(encoded[recordHeaderSize+len(rec.Key):], rec.Value)

	checksum := recordChecksum(rec.Type, rec.Timestamp, encoded[recordHeaderSize:])
	binary.LittleEndian.PutUint32(encoded[4:8], checksum)
	return encoded, nil
}

// DecodeRecord reads and verifies one record from r.
func DecodeRecord(r io.Reader) (Record, error) {
	var header [recordHeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Record{}, err
	}

	payloadLength := binary.LittleEndian.Uint32(header[0:4])
	if payloadLength > maxPayloadLength {
		return Record{}, InvalidRecordError("payload length implausibly large")
	}
	expectedChecksum := binary.LittleEndian.Uint32(header[4:8])
	recordType := header[8]
	if err := validateType(recordType); err != nil {
		return Record{}, err
	}
	timestamp := int64(binary.LittleEndian.Uint64(header[9:17]))
	keyLength := binary.LittleEndian.Uint16(header[17:19])
	if uint32(keyLength) > payloadLength {
		return Record{}, InvalidRecordError("key length exceeds payload length")
	}

	payload := make([]byte, int(payloadLength))
	if _, err := io.ReadFull(r, payload); err != nil {
		// A complete header proves that a record was started. If none of its
		// payload reached disk, ReadFull reports EOF rather than UnexpectedEOF;
		// normalize both cases so replay recognizes the torn write.
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return Record{}, err
	}
	actualChecksum := recordChecksum(recordType, timestamp, payload)
	if actualChecksum != expectedChecksum {
		return Record{}, &ChecksumError{
			Expected: expectedChecksum,
			Actual:   actualChecksum,
		}
	}

	keyEnd := int(keyLength)
	return Record{
		Type:      recordType,
		Timestamp: timestamp,
		Key:       payload[:keyEnd],
		Value:     payload[keyEnd:],
	}, nil
}

func validateType(recordType byte) error {
	if recordType != RecordPut && recordType != RecordDelete {
		return InvalidRecordError("unknown record type")
	}
	return nil
}

func recordChecksum(recordType byte, timestamp int64, payload []byte) uint32 {
	var metadata [9]byte
	metadata[0] = recordType
	binary.LittleEndian.PutUint64(metadata[1:], uint64(timestamp))

	checksum := crc32.Update(0, crc32.IEEETable, metadata[:])
	return crc32.Update(checksum, crc32.IEEETable, payload)
}
