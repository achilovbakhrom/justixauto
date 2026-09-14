#!/usr/bin/env bash
set -eu
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd -P)"
GIT_ROOT="$(git -C "$PROJECT_ROOT" rev-parse --show-toplevel 2>/dev/null || true)"
if [ "$GIT_ROOT" != "$PROJECT_ROOT" ]; then
  echo "GIT CHECK FAILED: project needs its own Git repository: $PROJECT_ROOT"
  echo "Detected Git root: ${GIT_ROOT:-none}. Do not use the parent repository."
  echo "User action: cd '$PROJECT_ROOT' && git init -b main && git add . && git commit -m 'Prepare development workspace'"
  exit 4
fi
if ! git -C "$PROJECT_ROOT" rev-parse --verify HEAD >/dev/null 2>&1; then
  echo "GIT CHECK FAILED: create the first commit before development."
  exit 5
fi
echo "GIT CHECK OK: $PROJECT_ROOT"
