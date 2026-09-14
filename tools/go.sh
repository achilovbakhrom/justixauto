#!/usr/bin/env bash
# Select an installed compiler meeting the reference baseline without global PATH edits.
set -eu
compatible() {
  [ -x "$1" ] || return 1
  local VERSION
  VERSION="$("$1" env GOVERSION 2>/dev/null)" || return 1
  [[ "$VERSION" =~ ^go([0-9]+)\.([0-9]+) ]] || return 1
  [ "${BASH_REMATCH[1]}" -gt 1 ] || { [ "${BASH_REMATCH[1]}" -eq 1 ] && [ "${BASH_REMATCH[2]}" -ge 23 ]; }
}
if [ -n "${JUSTIX_GO_BIN:-}" ]; then
  compatible "$JUSTIX_GO_BIN" || { echo 'JUSTIX_GO_BIN must point to an installed Go >= 1.23 compiler.' >&2; exit 1; }
  exec "$JUSTIX_GO_BIN" "$@"
fi
for CANDIDATE in "$(command -v go || true)" /opt/homebrew/bin/go; do
  if compatible "$CANDIDATE"; then exec "$CANDIDATE" "$@"; fi
done
echo 'Go >= 1.23 is required. Install it or set JUSTIX_GO_BIN to its absolute executable path.' >&2
exit 1
