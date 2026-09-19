# Event-store invariants

Contiguous revisions, expected-version append and full replay must agree. Test
duplicate IDs, competing writers, rollback and malformed/gapped history. Commit
failure is failure; no publish before durable outbox commit. Do not add snapshots
or mutate historical events without an approved contract change. Use real isolated
PostgreSQL QA when storage semantics cannot be established by pure tests.
