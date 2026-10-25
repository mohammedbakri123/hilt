#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
	echo "gofmt required for:" >&2
	echo "$unformatted" >&2
	exit 1
fi

go vet ./...
go test ./...
go test -race ./...
make build
