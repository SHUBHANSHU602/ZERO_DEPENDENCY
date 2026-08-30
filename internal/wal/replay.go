package wal

import (
	"errors"
	"io"
	"log/slog"
	"os"
)

// Replay reads every WAL segment from oldest to newest and returns all records
// that were written completely. A checksum mismatch or short final record is
// treated as a torn write: Replay removes that record and the remainder of its
// segment, then continues with the next segment.
func Replay(dir string) ([]Record, error) {
	segments, err := ListSegments(dir)
	if err != nil {
		return nil, err
	}

	var records []Record
	for _, path := range segments {
		segmentRecords, err := replaySegment(path)
		if err != nil {
			return nil, err
		}
		records = append(records, segmentRecords...)
	}
	return records, nil
}

func replaySegment(path string) ([]Record, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var records []Record
	for {
		offset, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}

		record, err := DecodeRecord(file)
		if err == nil {
			records = append(records, record)
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
