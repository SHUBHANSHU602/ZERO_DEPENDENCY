# PulseLog

PulseLog is a zero-dependency, crash-safe embedded key-value store written with
the Go 1.27 standard library.

## Quickstart

```console
$ go build -trimpath -o pulselog ./cmd/pulselog
$ echo '{"hr":118}' | ./pulselog --dir ./demo-data put vitals-042
$ ./pulselog --dir ./demo-data get vitals-042
{"hr":118}
$ ./pulselog --dir ./demo-data query --from 2020-01-01T00:00:00Z --to 2030-01-01T00:00:00Z
vitals-042	{"hr":118}
$ ./pulselog --dir ./demo-data delete vitals-042
$ ./pulselog --dir ./demo-data get vitals-042
pulselog: key "vitals-042" not found
$ echo $?
1
$ ./pulselog --dir ./demo-data compact
```

`put`, `delete`, and `compact` are silent on success. Values may be supplied as
the second `put` argument or read from standard input as shown above.

## Crash recovery

On a Unix-like system, start a `put` with a large stream on standard input and
send the CLI process `kill -9` while the record is being written. Start PulseLog
again against the same directory. It replays numbered WAL segments
oldest-to-newest. A partial payload or checksum mismatch is treated as a torn
write: the segment is truncated exactly to the last valid record boundary and
startup continues. Recovery emits a structured `log/slog` warning like:

```text
WARN truncating WAL segment after torn write segment=demo-data/000001.seg offset=77 error="unexpected EOF"
```

On Windows, forcibly terminating the process from Task Manager or with
`taskkill /F` provides the equivalent abrupt-stop experiment.

## Architecture

### Write-ahead log

Every mutation is an append-only binary WAL record containing payload length,
CRC32 checksum, operation type, nanosecond timestamp, key length, key, and
value. Segments roll at a size threshold. Startup replay validates records and
repairs a torn tail without scanning past corruption.

### In-memory index

A `map[string]Entry` guarded by `sync.RWMutex` points each live key at its most
recent segment, byte offset, encoded length, and timestamp. Reads seek directly
to that record. Replay rebuilds the index in write order; tombstones remove keys
so deleted values cannot reappear.

### Compaction

Manual compaction copies only index-referenced records from inactive segments
into one temporary segment, syncs it, atomically renames it, updates the index,
and only then removes obsolete segments. Overwritten records and tombstoned
keys are absent from the live index and therefore disappear naturally.

## Durability guarantee

PulseLog does not acknowledge a successful `Put` or `Delete` until the WAL
append has completed and `os.File.Sync` has returned successfully. A process
crash can lose an unacknowledged partial record, but replay removes that torn
tail while preserving every earlier valid record.

## Limitations

- Development and automated testing have been performed on Windows; broader
  operating-system testing is still needed.
- Values are JSON-wrapped with UUID metadata. Because `encoding/json` encodes
  byte slices as base64, binary values incur roughly 33% size overhead plus the
  metadata fields.
- Compaction is manual through `DB.Compact` or `pulselog compact`; there is no
  background scheduler or automatic threshold.
- There is no MANIFEST file. The active segment is derived by scanning for the
  highest-numbered segment, avoiding another piece of crash-sensitive state;
  see the [SegmentManager rationale](internal/wal/segment.go).
- The index is rebuilt in memory at startup, so startup time grows with WAL
  size and the complete live key set must fit in memory.

## Build and test

Go 1.27 is required because PulseLog uses the standard-library `uuid` package.

```console
$ go version
go version go1.27.0 windows/amd64
$ go build ./...
$ go test ./... -v
```

For a reproducible Windows CLI build:

```console
> build.bat pulselog.exe
```

On Linux or macOS, use the equivalent POSIX script:

```console
$ ./build.sh pulselog
```

Both scripts use `-trimpath`, disable environment-specific VCS stamping, and
record the toolchain version beside the binary.
See [REPRODUCIBLE_BUILD.md](REPRODUCIBLE_BUILD.md) for the two-build verification.
