#!/usr/bin/env bash
# Runs the ADR-13 pinned compiler without downloads or global PATH edits.
set -eu
export GOTOOLCHAIN=local
JUSTIX_GO_VERSION=go1.27.1
JUSTIX_PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
compatible() { [ -x "$1" ] && [ "$("$1" env GOVERSION 2>/dev/null)" = "$JUSTIX_GO_VERSION" ]; }
if [ -n "${JUSTIX_GO_BIN:-}" ]; then
  compatible "$JUSTIX_GO_BIN" || { echo "JUSTIX_GO_BIN must point to the installed $JUSTIX_GO_VERSION compiler." >&2; exit 1; }
  exec "$JUSTIX_GO_BIN" "$@"
fi
# Linked worktrees share the main checkout's ignored toolchain installation.
JUSTIX_COMMON_DIR="$(git -C "$JUSTIX_PROJECT_ROOT" rev-parse --path-format=absolute --git-common-dir 2>/dev/null || true)"
for CANDIDATE in \
  "$JUSTIX_PROJECT_ROOT/docs/justix-auto/dev/local/toolchains/go-1.27.1/bin/go" \
  "${JUSTIX_COMMON_DIR%/.git}/docs/justix-auto/dev/local/toolchains/go-1.27.1/bin/go" \
  "$(command -v go || true)" /opt/homebrew/bin/go; do
  if compatible "$CANDIDATE"; then exec "$CANDIDATE" "$@"; fi
done
echo "Go 1.27.1 is required. Install the approved toolchain locally or set JUSTIX_GO_BIN to its executable path. Automatic toolchain downloads are disabled." >&2
exit 1
