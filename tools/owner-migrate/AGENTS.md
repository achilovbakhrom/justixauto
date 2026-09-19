# Owner migration tooling

Read approved migration history, persistence-profile and compatibility contracts.
Preserve immutable bundle identity/checksums, serial ordering, owner isolation
and cleanup semantics. Test interrupted/failed migrations against isolated live
PostgreSQL where required. Application defines schema intent; DevOps owns job
execution/environment rollout. Do not apply migrations to a user cluster or
database because a migration runner was requested. No trading-project credentials.
