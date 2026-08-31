#!/bin/sh
set -eu

output=${1:-pulselog}
go version > "${output}.go-version.txt"
go version
go build -trimpath -buildvcs=false -o "$output" ./cmd/pulselog
