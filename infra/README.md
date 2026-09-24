# Local infrastructure

`local/compose.yaml` runs the single PostgreSQL 18.6 database used by the
modular monolith ([ADR-14](../docs/justix-auto/adr-14-classic-modular-monolith.md)),
bound to loopback port 55432 with a named volume. Credentials come from the
git-ignored `.env` (see `.env.example`); never commit real secrets.

```sh
docker compose --env-file .env -f infra/local/compose.yaml up -d     # start
docker compose --env-file .env -f infra/local/compose.yaml stop      # stop, keep data
```

## Local Kubernetes (kind)

[ADR-15](../docs/justix-auto/adr-15-kubernetes-multi-replica-observability.md):
the app runs as 2 replicas from the Helm chart `deploy/helm/justixauto`
(`values-dev.yaml`), with PostgreSQL, MinIO (S3) and an observability stack
(OpenTelemetry Collector → Loki, Tempo, Prometheus; Grafana).

Needs Docker, `kubectl`, `helm` and `kind` (or `GOBIN=$PWD/var/bin bash tools/go.sh install sigs.k8s.io/kind@v0.30.0`).

```sh
bash infra/kind/up.sh     # create/update cluster "justixauto" and deploy everything
bash infra/kind/down.sh   # delete the cluster
```

- App: http://localhost:18090; two-factor is off (`values-dev.yaml`).
- Grafana: http://localhost:13000, user `admin`, password:
  `kubectl --context kind-justixauto -n observability get secret grafana-admin -o jsonpath='{.data.admin-password}' | base64 -d`.
  Dashboard *JustixAuto → JustixAuto API*; in Explore, Loki log lines link to Tempo traces by `trace_id`.
- Every command names `--context kind-justixauto`; the script restores your previous current context.
- Secrets are generated once inside the cluster and never printed.

Production: `values-prod.yaml` is a template (registry, image digest, S3 bucket,
IRSA role, ingress host). Rollout to shared or production clusters is human-only
(`AGENTS.md`).
