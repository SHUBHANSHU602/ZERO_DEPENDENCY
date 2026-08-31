# Reproducible build verification

The CLI was built twice from the same commit on Windows with the Go build cache
cleared before each build. Both builds used:

```text
go version go1.27.0 windows/amd64
```

Commands executed from the repository root:

```powershell
$env:GOCACHE = Join-Path $env:TEMP 'pulselog-repro-cache'

go clean -cache
go build -trimpath -buildvcs=false -o "$env:TEMP\pulselog-repro-1.exe" ./cmd/pulselog
Get-FileHash -Algorithm SHA256 "$env:TEMP\pulselog-repro-1.exe"

go clean -cache
go build -trimpath -buildvcs=false -o "$env:TEMP\pulselog-repro-2.exe" ./cmd/pulselog
Get-FileHash -Algorithm SHA256 "$env:TEMP\pulselog-repro-2.exe"
```

Results:

| Build | SHA-256 |
| --- | --- |
| `pulselog-repro-1.exe` | `4B081A96149F5A692E0D7CB6BC8E1DB545EAB011599765549E0AC47D616E96AE` |
| `pulselog-repro-2.exe` | `4B081A96149F5A692E0D7CB6BC8E1DB545EAB011599765549E0AC47D616E96AE` |

The hashes are identical. The repository's `build.bat` applies the same
`-trimpath` and `-buildvcs=false` flags and writes `go version` output to
`<binary>.go-version.txt` beside each requested output binary. Disabling VCS
stamping prevents the repository revision or dirty-worktree flag from making an
otherwise identical binary differ.
