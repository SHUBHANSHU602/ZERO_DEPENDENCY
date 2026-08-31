package pulselog

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"testing/synctest"
	"time"
	"uuid"

	"pulselog/internal/wal"
)

func TestPutThenGet(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	want := []byte("value")
	if err := db.Put("key", want); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	got, err := db.Get("key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Get() = %q, want %q", got, want)
	}
}

func TestPutOverwritesExistingKey(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	if err := db.Put("key", []byte("first")); err != nil {
		t.Fatalf("Put(first value) error = %v", err)
	}
	want := []byte("second")
	if err := db.Put("key", want); err != nil {
		t.Fatalf("Put(second value) error = %v", err)
	}
	got, err := db.Get("key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Get() = %q, want %q", got, want)
	}
}

func TestGetMissingKey(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	_, err = db.Get("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestOpenRecoversExistingData(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Put("first", []byte("one")); err != nil {
		t.Fatalf("Put(first) error = %v", err)
	}
	if err := db.Put("second", []byte("two")); err != nil {
		t.Fatalf("Put(second) error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() after restart error = %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("Close() after restart error = %v", err)
		}
	})
	for key, want := range map[string][]byte{"first": []byte("one"), "second": []byte("two")} {
		got, err := reopened.Get(key)
		if err != nil {
			t.Fatalf("Get(%q) error = %v", key, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("Get(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestOpenRecoversLatestValueAfterOverwrite(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Put("key", []byte("first")); err != nil {
		t.Fatalf("Put(first value) error = %v", err)
	}
	want := []byte("second")
	if err := db.Put("key", want); err != nil {
		t.Fatalf("Put(second value) error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() after restart error = %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("Close() after restart error = %v", err)
		}
	})
	got, err := reopened.Get("key")
	if err != nil {
		t.Fatalf("Get() after restart error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Get() after restart = %q, want %q", got, want)
	}
}

func TestDeleteRemovesKey(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	if err := db.Put("key", []byte("value")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := db.Delete("key"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := db.Get("key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestDeleteSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Put("key", []byte("value")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := db.Delete("key"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() after restart error = %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("Close() after restart error = %v", err)
		}
	})
	if _, err := reopened.Get("key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after restart error = %v, want ErrNotFound", err)
	}
}

func TestRangeQueryFiltersSortsAndExcludesDeletedKeys(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	for _, key := range []string{"before", "first", "deleted", "second", "after"} {
		if err := db.Put(key, []byte(key+"-value")); err != nil {
			t.Fatalf("Put(%q) error = %v", key, err)
		}
	}
	all, err := db.RangeQuery(time.Unix(0, 0), time.Unix(0, 1<<62))
	if err != nil {
		t.Fatalf("RangeQuery(all) error = %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("RangeQuery(all) returned %d records, want 5", len(all))
	}
	if err := db.Delete("deleted"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	got, err := db.RangeQuery(all[1].Timestamp, all[3].Timestamp)
	if err != nil {
		t.Fatalf("RangeQuery() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("RangeQuery() returned %d records, want 2", len(got))
	}
	for i, wantKey := range []string{"first", "second"} {
		if got[i].Key != wantKey {
			t.Errorf("RangeQuery()[%d].Key = %q, want %q", i, got[i].Key, wantKey)
		}
		if !bytes.Equal(got[i].Value, []byte(wantKey+"-value")) {
			t.Errorf("RangeQuery()[%d].Value = %q", i, got[i].Value)
		}
	}
	if got[0].Timestamp.After(got[1].Timestamp) {
		t.Error("RangeQuery() results are not ordered by timestamp")
	}
}

func TestPutGeneratesUniqueUUIDs(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	for _, key := range []string{"first", "second", "third"} {
		if err := db.Put(key, []byte("value")); err != nil {
			t.Fatalf("Put(%q) error = %v", key, err)
		}
	}
	records, err := db.RangeQuery(time.Unix(0, 0), time.Unix(0, 1<<62))
	if err != nil {
		t.Fatalf("RangeQuery() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("RangeQuery() returned %d records, want 3", len(records))
	}
	seen := make(map[uuid.UUID]bool, len(records))
	for _, record := range records {
		if record.ID == uuid.Nil() {
			t.Fatalf("record %q has an empty UUID", record.Key)
		}
		if _, err := uuid.Parse(record.ID.String()); err != nil {
			t.Fatalf("record %q UUID is invalid: %v", record.Key, err)
		}
		if seen[record.ID] {
			t.Fatalf("duplicate UUID %s", record.ID)
		}
		seen[record.ID] = true
	}
}

func TestCompactReclaimsDeadDataAndSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	db := openCompactionFixture(t, dir)

	if err := db.Compact(); err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	assertCompactedValues(t, db)
	assertSegmentNames(t, dir, []string{"000002.seg", "000003.seg"})
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() after compaction error = %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("Close() after compaction error = %v", err)
		}
	})
	assertCompactedValues(t, reopened)
}

func TestCompactWithOnlyActiveSegmentIsNoOp(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	if err := db.Put("key", []byte("value")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := db.Compact(); err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	assertSegmentNames(t, dir, []string{"000001.seg"})
	got, err := db.Get("key")
	if err != nil || !bytes.Equal(got, []byte("value")) {
		t.Fatalf("Get() after no-op compaction = %q, %v", got, err)
	}
}

func TestCompactSerializesWithPutAndGet(t *testing.T) {
	db := openCompactionFixture(t, t.TempDir())
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	var wg sync.WaitGroup
	errs := make(chan error, 3)
	wg.Add(3)
	go func() {
		defer wg.Done()
		errs <- db.Compact()
	}()
	go func() {
		defer wg.Done()
		errs <- db.Put("concurrent", []byte("write"))
	}()
	go func() {
		defer wg.Done()
		_, err := db.Get("live")
		errs <- err
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent operation error = %v", err)
		}
	}
	for key, want := range map[string][]byte{"live": []byte("kept"), "concurrent": []byte("write")} {
		got, err := db.Get(key)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("Get(%q) after concurrent compaction = %q, %v", key, got, err)
		}
	}
}

func openCompactionFixture(t *testing.T, dir string) *DB {
	t.Helper()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	for key, value := range map[string][]byte{
		"live":    []byte("kept"),
		"deleted": []byte("remove me"),
	} {
		if err := db.Put(key, value); err != nil {
			t.Fatalf("Put(%q) error = %v", key, err)
		}
	}
	if err := db.Put("overwritten", bytes.Repeat([]byte("x"), 3<<20)); err != nil {
		t.Fatalf("Put(overwritten old value) error = %v", err)
	}
	if err := db.Put("overwritten", []byte("latest")); err != nil {
		t.Fatalf("Put(overwritten latest value) error = %v", err)
	}
	if err := db.Delete("deleted"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	return db
}

func assertCompactedValues(t *testing.T, db *DB) {
	t.Helper()
	for key, want := range map[string][]byte{"live": []byte("kept"), "overwritten": []byte("latest")} {
		got, err := db.Get(key)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("Get(%q) after compaction = %q, %v", key, got, err)
		}
	}
	if _, err := db.Get("deleted"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(deleted) after compaction error = %v, want ErrNotFound", err)
	}
}

func assertSegmentNames(t *testing.T, dir string, want []string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	var got []string
	for _, entry := range entries {
		if !entry.IsDir() {
			got = append(got, entry.Name())
		}
	}
	if len(got) != len(want) {
		t.Fatalf("segment files = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment files = %v, want %v", got, want)
		}
	}
}

func TestConcurrentPuts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db, err := Open(t.TempDir())
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		t.Cleanup(func() {
			if err := db.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		})

		const goroutines = 20
		want := make(map[string][]byte, goroutines)
		putErrors := make([]error, goroutines)
		var wg sync.WaitGroup
		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			key := fmt.Sprintf("key-%02d", i)
			value := []byte(fmt.Sprintf("value-%02d", i))
			want[key] = value
			go func(i int, key string, value []byte) {
				defer wg.Done()
				putErrors[i] = db.Put(key, value)
			}(i, key, value)
		}
		wg.Wait()

		for i, err := range putErrors {
			if err != nil {
				t.Errorf("Put goroutine %d error = %v", i, err)
			}
		}
		for key, wantValue := range want {
			got, err := db.Get(key)
			if err != nil {
				t.Errorf("Get(%q) error = %v", key, err)
				continue
			}
			if !bytes.Equal(got, wantValue) {
				t.Errorf("Get(%q) = %q, want %q", key, got, wantValue)
			}
		}
		if got := len(db.index.Snapshot()); got != goroutines {
			t.Errorf("index entry count = %d, want %d", got, goroutines)
		}
	})
}

// This is the automated, repeatable equivalent of the demo video's live
// kill -9: both prove that restart preserves valid records and removes a torn
// final write.
func TestOpenRecoversFromSimulatedCrash(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	want := map[string][]byte{
		"alpha": []byte("one"),
		"beta":  []byte("two"),
		"gamma": []byte("three"),
	}
	for key, value := range want {
		if err := db.Put(key, value); err != nil {
			t.Fatalf("Put(%q) error = %v", key, err)
		}
	}
	segmentPath, err := wal.SegmentPath(dir, db.segments.ActiveSegmentNumber())
	if err != nil {
		t.Fatalf("SegmentPath() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	validInfo, err := os.Stat(segmentPath)
	if err != nil {
		t.Fatalf("Stat(valid segment) error = %v", err)
	}

	torn, err := wal.EncodeRecord(wal.Record{
		Type: wal.RecordPut, Timestamp: time.Now().UnixNano(), Key: []byte("torn"), Value: []byte("missing"),
	})
	if err != nil {
		t.Fatalf("EncodeRecord(torn record) error = %v", err)
	}
	const recordHeaderSize = 19
	file, err := os.OpenFile(segmentPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatalf("OpenFile(segment) error = %v", err)
	}
	if _, err := file.Write(torn[:recordHeaderSize]); err != nil {
		_ = file.Close()
		t.Fatalf("Write(torn header) error = %v", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		t.Fatalf("Sync(torn header) error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close(torn segment) error = %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() after simulated crash error = %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("Close() after recovery error = %v", err)
		}
	})
	for key, wantValue := range want {
		got, err := reopened.Get(key)
		if err != nil {
			t.Errorf("Get(%q) after recovery error = %v", key, err)
			continue
		}
		if !bytes.Equal(got, wantValue) {
			t.Errorf("Get(%q) after recovery = %q, want %q", key, got, wantValue)
		}
	}
	recoveredInfo, err := os.Stat(segmentPath)
	if err != nil {
		t.Fatalf("Stat(recovered segment) error = %v", err)
	}
	if got, wantSize := recoveredInfo.Size(), validInfo.Size(); got != wantSize {
		t.Fatalf("recovered segment size = %d, want %d", got, wantSize)
	}
}
