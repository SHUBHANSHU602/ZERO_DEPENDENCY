package pulselog

import (
	"bytes"
	"errors"
	"testing"
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
