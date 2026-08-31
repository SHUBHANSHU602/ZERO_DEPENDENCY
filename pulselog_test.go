package pulselog

import (
	"bytes"
	"errors"
	"testing"
	"time"
	"uuid"
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
