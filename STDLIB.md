# Standard-library substitutions

PulseLog has no module dependencies. These standard-library facilities replace
packages commonly added to storage and CLI projects:

1. `hash/crc32` — computes and verifies WAL record checksums without a hashing dependency.
2. `encoding/binary` — packs and unpacks the fixed-width little-endian record header.
3. `os.File.Sync()` — supplies fsync-based durability without a storage engine or durability wrapper.
4. `flag` — parses global and subcommand CLI flags without Cobra, urfave/cli, or similar frameworks.
5. `log/slog` — emits structured torn-write recovery warnings without a logging package.
6. `sync.RWMutex` and `sync.Mutex` — protect the index, database lifecycle, and segment writers without synchronization helpers.
7. **Package Killer:** replaces `github.com/google/uuid` (a Go staple for unique IDs) with the Go 1.27 standard library `uuid` package (`uuid.NewV7()`), used directly with no wrapper.
8. `path/filepath` — constructs and inspects portable segment paths without filesystem utility packages.
9. `encoding/json` — wraps values with UUID metadata without a third-party serializer.
10. `os.CreateTemp` plus `os.Rename` — stages and atomically commits compacted segments without a transactional-files package.
11. `io.ReadFull` and `io.LimitReader` — enforce exact record boundaries and bounded reads without binary-I/O helpers.
12. `errors.Is`, `errors.As`, and `errors.Join` — implement sentinel matching, typed corruption detection, and multi-error cleanup reporting.
13. `sort.Slice` — orders replay metadata, compaction input, and range results without collection libraries.
14. `testing` — provides the complete test suite without assertion or test-runner dependencies.
15. In-memory `io.Reader` and `io.Writer` values exercise CLI parsing directly without a CLI testing framework.
