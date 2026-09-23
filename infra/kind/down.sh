#!/usr/bin/env bash
# Deletes the local kind cluster "justixauto" and everything in it.
set -euo pipefail
cd "$(dirname "$0")/../.."
KIND=$(command -v kind || echo var/bin/kind)
"$KIND" delete cluster --name justixauto
