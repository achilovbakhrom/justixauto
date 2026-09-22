#!/usr/bin/env bash
# Runs all Go tests, including database tests, against a throwaway PostgreSQL
# container that is removed afterwards. Extra arguments go to `go test`.
set -euo pipefail
cd "$(dirname "$0")/.."
image="docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
name="justixauto-test-$$"
password="$(openssl rand -hex 16)"
docker run -d --rm --name "$name" -p 127.0.0.1::5432 -e POSTGRES_PASSWORD="$password" -e POSTGRES_DB=justixauto_test "$image" >/dev/null
trap 'docker rm -f "$name" >/dev/null 2>&1 || true' EXIT
for _ in $(seq 60); do
  docker exec "$name" pg_isready -U postgres -d justixauto_test -h 127.0.0.1 >/dev/null 2>&1 && break
  sleep 1
done
port="$(docker port "$name" 5432/tcp | head -1 | awk -F: '{print $NF}')"
export TEST_DATABASE_URL="postgres://postgres:${password}@127.0.0.1:${port}/justixauto_test?sslmode=disable"
# -p 1: packages share one database.
bash tools/go.sh test -race -count=1 -p 1 "${@:-./...}"
