#!/usr/bin/env bash
# Local Kubernetes for JustixAuto: kind cluster "justixauto" with PostgreSQL,
# MinIO, OpenTelemetry Collector, Loki, Tempo, Prometheus, Grafana and the app
# (2 replicas) from deploy/helm/justixauto with values-dev.yaml.
# Every kubectl/helm call names the kind context explicitly; it never touches
# the current kubectl context. Secrets are generated here and never printed.
set -euo pipefail
cd "$(dirname "$0")/../.."
CTX=kind-justixauto
KIND=$(command -v kind || echo var/bin/kind)
K="kubectl --context $CTX"
H="helm --kube-context $CTX"
G=https://grafana.github.io/helm-charts

# kind switches the current kubectl context on create; put the previous one back.
PREVIOUS_CONTEXT=$(kubectl config current-context 2>/dev/null || true)
restore_context() { [ -n "$PREVIOUS_CONTEXT" ] && kubectl config use-context "$PREVIOUS_CONTEXT" >/dev/null 2>&1 || true; }
trap restore_context EXIT
if ! "$KIND" get clusters | grep -qx justixauto; then
  "$KIND" create cluster --config infra/kind/kind-config.yaml
  restore_context
fi
$K get nodes >/dev/null

# Tag = commit + content hash of the built image: every change gets a new,
# immutable tag, so Helm always rolls the pods out when the code changed.
COMMIT=$(git rev-parse --short HEAD)
docker build -q -t justixauto:build --build-arg VERSION="$COMMIT" . >/dev/null
TAG="$COMMIT-$(docker image inspect -f '{{.Id}}' justixauto:build | cut -d: -f2 | cut -c1-12)"
docker tag justixauto:build "justixauto:$TAG"
echo "== image justixauto:$TAG"
"$KIND" load docker-image --name justixauto "justixauto:$TAG" minio/minio:latest >/dev/null

for ns in justixauto justixauto-deps observability; do $K create namespace $ns --dry-run=client -o yaml | $K apply -f - >/dev/null; done

secret() { # namespace name key=value... — created once, kept on later runs
  local ns=$1 name=$2; shift 2
  $K -n "$ns" get secret "$name" >/dev/null 2>&1 && return
  local args=(); for kv in "$@"; do args+=(--from-literal="$kv"); done
  $K -n "$ns" create secret generic "$name" "${args[@]}" >/dev/null
}
rand() { openssl rand -hex 16; }
if ! $K -n justixauto-deps get secret deps-secrets >/dev/null 2>&1; then
  PG=$(rand); S3=$(rand)
  secret justixauto-deps deps-secrets POSTGRES_PASSWORD="$PG" MINIO_ROOT_PASSWORD="$S3"
  secret justixauto justixauto-secrets \
    DATABASE_URL="postgres://justixauto:$PG@postgres.justixauto-deps:5432/justixauto?sslmode=disable" \
    AWS_ACCESS_KEY_ID=justixauto AWS_SECRET_ACCESS_KEY="$S3"
fi
secret observability grafana-admin admin-user=admin admin-password="$(rand)"

echo "== PostgreSQL and MinIO"
$K apply -f infra/kind/deps.yaml >/dev/null
$K -n justixauto-deps rollout status statefulset/postgres --timeout=180s >/dev/null
$K -n justixauto-deps rollout status deployment/minio --timeout=180s >/dev/null

echo "== observability"
$K apply -f infra/kind/observability/dashboard.yaml >/dev/null
$H upgrade --install loki loki --repo $G --version 7.3.0 -n observability -f infra/kind/observability/loki.yaml --wait --timeout 10m >/dev/null
$H upgrade --install tempo tempo --repo $G --version 1.24.4 -n observability -f infra/kind/observability/tempo.yaml --wait --timeout 10m >/dev/null
$H upgrade --install prometheus prometheus --repo https://prometheus-community.github.io/helm-charts --version 29.31.1 \
  -n observability -f infra/kind/observability/prometheus.yaml --wait --timeout 10m >/dev/null
$H upgrade --install otel-collector opentelemetry-collector --repo https://open-telemetry.github.io/opentelemetry-helm-charts --version 0.173.1 \
  -n observability -f infra/kind/observability/otel-collector.yaml --wait --timeout 10m >/dev/null
$H upgrade --install grafana grafana --repo $G --version 10.5.15 -n observability -f infra/kind/observability/grafana.yaml --wait --timeout 10m >/dev/null

echo "== JustixAuto (migrations run as a pre-install/pre-upgrade job)"
$H upgrade --install justixauto deploy/helm/justixauto -n justixauto -f deploy/helm/justixauto/values-dev.yaml \
  --set image.tag="$TAG" --wait --timeout 10m >/dev/null

$K -n justixauto get pods -l app.kubernetes.io/component=api -o wide
cat <<MSG

App:     http://localhost:18090
Grafana: http://localhost:13000  (user admin; password:
  kubectl --context $CTX -n observability get secret grafana-admin -o jsonpath='{.data.admin-password}' | base64 -d)
MSG
