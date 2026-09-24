#!/usr/bin/env bash
# lefthook pre-commit: regenerates the OpenAPI spec when Go code is committed
# and stops the commit if the spec changed, so it can be reviewed and staged.
set -euo pipefail
spec=internal/pkg/apidocs/swagger.json
git diff --cached --name-only --diff-filter=ACMRD | grep -qE '^(cmd|internal)/.*\.go$' || exit 0
# The generator reads the working tree; unstaged Go edits would leak into the spec.
if ! git diff --quiet -- 'cmd/*.go' 'internal/*.go'; then
  echo "pre-commit: stash or stage your unstaged Go changes so the OpenAPI spec matches the commit." >&2
  exit 1
fi
make --no-print-directory openapi >/dev/null
if ! git diff --quiet -- "$spec" || ! git diff --quiet -- 'cmd/*.go' 'internal/*.go'; then
  echo "pre-commit: OpenAPI annotations changed the spec (or swag fmt reformatted them)." >&2
  echo "Review with 'git diff', then: git add $spec <changed files> && git commit" >&2
  exit 1
fi
