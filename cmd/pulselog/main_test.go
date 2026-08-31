package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunQuickstart(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--dir", dir, "put", "vitals-042"},
		strings.NewReader("{\"hr\":118}\n"), &stdout, &stderr); err != nil {
		t.Fatalf("put error = %v; stderr = %q", err, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := run([]string{"--dir", dir, "get", "vitals-042"},
		strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("get error = %v; stderr = %q", err, stderr.String())
	}
	if got, want := stdout.String(), "{\"hr\":118}\n"; got != want {
		t.Fatalf("get stdout = %q, want %q", got, want)
	}

	stdout.Reset()
	stderr.Reset()
	if err := run([]string{"--dir", dir, "query", "--from", "2020-01-01T00:00:00Z", "--to", "2030-01-01T00:00:00Z"},
		strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("query error = %v; stderr = %q", err, stderr.String())
	}
	if got, want := stdout.String(), "vitals-042\t{\"hr\":118}\n"; got != want {
		t.Fatalf("query stdout = %q, want %q", got, want)
	}

	if err := run([]string{"--dir", dir, "delete", "vitals-042"},
		strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("delete error = %v", err)
	}
	if err := run([]string{"--dir", dir, "get", "vitals-042"},
		strings.NewReader(""), &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("get deleted key error = %v, want clear not-found error", err)
	}
	if err := run([]string{"--dir", dir, "compact"},
		strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("compact error = %v", err)
	}
}

func TestRunShowsUsageForUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"unknown"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatal("run() error = nil, want unknown-command error")
	}
	if !strings.Contains(stderr.String(), "Usage: pulselog") {
		t.Fatalf("stderr = %q, want usage message", stderr.String())
	}
}
