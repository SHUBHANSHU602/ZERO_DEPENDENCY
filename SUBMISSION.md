# PulseLog submission

## Track

**D — Data & Storage**

## One-line pitch

PulseLog is a crash-safe embedded key-value store with a hand-built binary WAL,
checksummed recovery, direct-read index, segment rotation, and compaction, using
only the Go 1.27 standard library.

## What makes it useful

Small local tools often need durable state but should not require a database
server or a third-party embedded-database driver. PulseLog ships as one binary,
stores data in one directory, persists acknowledged writes across restarts, and
repairs a write interrupted by a process crash.

## What was built from scratch

- A versioned binary WAL record format with explicit bounds and CRC32 checksums.
- Synchronous durable append and size-based segment rotation.
- Ordered replay that rebuilds state and truncates an incomplete tail.
- A concurrent in-memory index pointing at exact segment offsets.
- Tombstones, timestamp range queries, and crash-conscious compaction.
- A standard-library-only CLI and test suite.

## Verification commands

```sh
./build.sh pulselog
go test ./... -v
go vet ./...
go list -m all
```

`go list -m all` must print only `pulselog`. See `deps-proof.txt`, `STDLIB.md`,
and `REPRODUCIBLE_BUILD.md` for the dependency and deterministic-build receipts.

## Honest boundaries

PulseLog coordinates goroutines inside one process. It does not implement
multi-process locking, replication, transactions across keys, SQL, encryption,
or background compaction. The complete live index must fit in memory, and
startup time grows with WAL history.

## Suggested video structure

- **0:00–0:30:** Problem, Track D, empty `go.mod`.
- **0:30–1:30:** Build, put/get, and persistence across CLI invocations.
- **1:30–2:30:** Overwrite/delete/range query and compaction.
- **2:30–3:30:** Kill during append, reopen, and show torn-tail recovery.
- **3:30–4:20:** WAL format, index, and compaction architecture.
- **4:20–5:00:** Tests, dependency proof, reproducible hashes, and limitations.
