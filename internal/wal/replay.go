package wal

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Replay reads every WAL segment from oldest to newest and returns all records
// that were written completely. A checksum mismatch or short final record is
// treated as a torn write: Replay removes that record and the remainder of its
// segment, then continues with the next segment.
func Replay(dir string) ([]Record, error) {
	replayed, err := ReplayWithMetadata(dir)
	if err != nil {
		return nil, err
	}
	records := make([]Record, len(replayed))
	for i := range replayed {
		records[i] = replayed[i].Record
	}
	return records, nil
}

// ReplayedRecord describes a decoded record and its location in the WAL.
type ReplayedRecord struct {
	Record    Record
	SegmentID uint64
	Offset    int64
	Length    uint32
}

// ReplayWithMetadata performs recovery and returns each record's location so
// callers can rebuild an index without scanning the WAL a second time.
func ReplayWithMetadata(dir string) ([]ReplayedRecord, error) {
	segments, err := ListSegments(dir)
	if err != nil {
		return nil, err
	}

	var records []ReplayedRecord
	for _, path := range segments {
		segmentID, err := segmentNumber(filepath.Base(path))
		if err != nil {
			return nil, err
		}
		segmentRecords, err := replaySegment(path, segmentID)
		if err != nil {
			return nil, err
		}
		records = append(records, segmentRecords...)
	}
	return records, nil
}

func replaySegment(path string, segmentID uint64) ([]ReplayedRecord, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var records []ReplayedRecord
	for {
		offset, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}

		record, err := DecodeRecord(file)
		if err == nil {
			end, err := file.Seek(0, io.SeekCurrent)
			if err != nil {
				return nil, err
			}
			records = append(records, ReplayedRecord{
				Record:    record,
				SegmentID: segmentID,
				Offset:    offset,
				Length:    uint32(end - offset),
			})
			continue
		}
		if errors.Is(err, io.EOF) {
			return records, nil
		}
		if !isTornWrite(err) {
			return nil, err
		}

		slog.Warn("truncating WAL segment after torn write",
			"segment", path,
			"offset", offset,
			"error", err,
		)
		if err := file.Truncate(offset); err != nil {
			return nil, err
		}
		return records, nil
	}
}

func isTornWrite(err error) bool {
	var checksumErr *ChecksumError
	return errors.As(err, &checksumErr) || errors.Is(err, io.ErrUnexpectedEOF)
}
