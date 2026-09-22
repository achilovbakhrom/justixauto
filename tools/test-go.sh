#!/usr/bin/env bash
# Runs all Go tests, including database and S3 tests, against throwaway
# PostgreSQL and MinIO containers that are removed afterwards. Extra
# arguments go to `go test`. Set NO_S3=1 to use a temporary directory instead.
set -euo pipefail
cd "$(dirname "$0")/.."
pg_image="docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
minio_image="docker.io/minio/minio:latest"
name="justixauto-test-$$"
password="$(openssl rand -hex 16)"
cleanup() { docker rm -f "$name-pg" "$name-s3" >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker run -d --rm --name "$name-pg" -p 127.0.0.1::5432 -e POSTGRES_PASSWORD="$password" -e POSTGRES_DB=justixauto_test "$pg_image" >/dev/null
if [ "${NO_S3:-}" != 1 ]; then
  docker run -d --rm --name "$name-s3" -p 127.0.0.1::9000 -e MINIO_ROOT_USER=justixtest -e MINIO_ROOT_PASSWORD="$password" \
    "$minio_image" server /data >/dev/null
  export TEST_S3_ENDPOINT="http://127.0.0.1:$(docker port "$name-s3" 9000/tcp | head -1 | awk -F: '{print $NF}')"
  export AWS_ACCESS_KEY_ID=justixtest AWS_SECRET_ACCESS_KEY="$password" AWS_REGION=us-east-1
fi
for _ in $(seq 60); do
  docker exec "$name-pg" pg_isready -U postgres -d justixauto_test -h 127.0.0.1 >/dev/null 2>&1 && break
  sleep 1
done
if [ -n "${TEST_S3_ENDPOINT:-}" ]; then
  for _ in $(seq 60); do curl -sf "$TEST_S3_ENDPOINT/minio/health/ready" >/dev/null && break; sleep 1; done
fi
port="$(docker port "$name-pg" 5432/tcp | head -1 | awk -F: '{print $NF}')"
export TEST_DATABASE_URL="postgres://postgres:${password}@127.0.0.1:${port}/justixauto_test?sslmode=disable"
# -p 1: packages share one database.
bash tools/go.sh test -race -count=1 -p 1 "${@:-./...}"
