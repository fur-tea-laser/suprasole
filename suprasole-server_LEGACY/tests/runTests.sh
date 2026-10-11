#!/bin/sh
set -eu

cd "$(dirname "$0")/.."

overlay="$(mktemp)"
directory_PtyExec="$(mktemp -d)"
trap 'rm -f "$overlay"; rm -rf "$directory_PtyExec"' EXIT

{
	printf '{"Replace":{'
	separator=''
	for file in tests/*_test.go; do
		printf '%s"source/%s":"%s"' "$separator" "$(basename "$file")" "$file"
		separator=','
	done
	printf '}}'
} > "$overlay"

go build -ldflags='-E main.Entry_PtyExec' -o "$directory_PtyExec/suprasole-pty-exec" ./source/PtyExec
export SUPRASOLE_PTY_EXEC="$directory_PtyExec/suprasole-pty-exec"

go vet ./source/PtyExec
go vet -tags tests_overlay -overlay "$overlay" ./source ./tests/contract
go test -tags tests_overlay -overlay "$overlay" "$@" ./source ./tests/contract
