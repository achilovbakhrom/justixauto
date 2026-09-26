#!/usr/bin/env bash
# Kept for task records and agent rules; the check itself is `devtool check-git`.
# Built instead of `go run` so the exit codes 4 and 5 reach the caller.
# Rebuilt only when a source file is newer than the binary (R0 review, 2026-09-25).
set -eu
cd "$(dirname "$0")/.."
bin=var/bin/devtool
if [ ! -x "$bin" ] || [ -n "$(find tools/devtool -name '*.go' -newer "$bin" -print -quit)" ]; then
  bash tools/go.sh build -buildvcs=false -o "$bin" ./tools/devtool
fi
exec "$bin" check-git
