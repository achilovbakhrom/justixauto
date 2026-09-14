# Local infrastructure readiness

Reference technologies: PostgreSQL/GORM event store and read models, RabbitMQ,
Redis where required, OpenTelemetry/Jaeger, Docker Compose. Refer to the exact
versions observed in `docs/justix-auto/reference/gaze-reference.md`.

No Gaze Compose file, environment file, credentials, private network or database
connection was copied. No containers, migrations or service ports were started
by this handoff. Node serves only documentation on loopback port 4180.

After independent Git setup and architecture approval, the first infrastructure
task must pin supported images/tool versions; allocate collision-free loopback
ports; generate local-only credentials; provide `.env.example`; define isolated
volumes/networks, health checks, migration commands and a non-destructive stop
command. Do not copy the reference's unpinned Jaeger or development gRPC version
without review. Outbox/inbox guarantees and snapshot functionality are pending
ADRs, not silently added as if Gaze already provided them.
