#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

go test ./...
go test -race ./...
