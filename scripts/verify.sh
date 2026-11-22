#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p .cache/acceptance
receipt_dir=$(mktemp -d "$PWD/.cache/acceptance/run.XXXXXX")
export WENSHUSHU_RECEIPTS="$receipt_dir"
printf 'running\n' > "$receipt_dir/status.txt"
trap 'result=$?; if [ "$result" -ne 0 ]; then printf "failed\n" > "$receipt_dir/status.txt"; fi' EXIT
mkdir -p "$receipt_dir/tmp"
export TMPDIR="$receipt_dir/tmp"
git rev-parse HEAD > "$receipt_dir/source-commit.txt"
git diff --binary HEAD > "$receipt_dir/source.patch"
find . -type f \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' -o -name 'spec.toml' \) -not -path './.cache/*' -not -path './.git/*' -exec shasum -a 256 {} \; > "$receipt_dir/source-sha256.txt"
if go test -race ./... -count=1 -v > "$receipt_dir/tests.log" 2>&1; then
  :
else
  printf 'failed\n' > "$receipt_dir/status.txt"
  cat "$receipt_dir/tests.log"
  printf 'First failure retained: %s\n' "$receipt_dir"
  exit 1
fi
go build -trimpath -o "$receipt_dir/unic-plugin-wenshushu" ./cmd/unic-plugin-wenshushu
shasum -a 256 "$receipt_dir/unic-plugin-wenshushu" > "$receipt_dir/binary-sha256.txt"
printf 'Replay: GOWORK=off WENSHUSHU_LIVE=%s sh scripts/verify.sh\n' "${WENSHUSHU_LIVE:-0}" > "$receipt_dir/replay.txt"
printf 'passed\n' > "$receipt_dir/status.txt"
printf 'Receipt: %s\n' "$receipt_dir"
