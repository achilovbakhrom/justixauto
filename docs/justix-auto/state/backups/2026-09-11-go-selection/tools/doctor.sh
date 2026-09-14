#!/usr/bin/env bash
set -u
cd "$(dirname "$0")/.." || exit 1
FAILED=0
for PROGRAM in git go node npm docker codex; do
  if command -v "$PROGRAM" >/dev/null 2>&1; then echo "FOUND: $PROGRAM"; else echo "MISSING: $PROGRAM"; FAILED=1; fi
done
go version || FAILED=1
node --version || FAILED=1
npm --version || FAILED=1
docker compose version || FAILED=1
docker info --format 'Docker daemon: {{.ServerVersion}}' || FAILED=1
codex --version || FAILED=1
bash tools/check-git.sh || FAILED=1
echo "No infrastructure was started. Go/React application dependencies are not scaffolded yet."
exit "$FAILED"
