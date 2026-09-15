# Messaging delivery amendment — independent QA

**GREEN — documentation proposal readiness only.**

Reviewed commit: `ba1723fdcbbfd9a0bbccdf6736e9346052d80fd7`.
Date: 2026-09-15. Reviewer: `/root/qa_messaging_delivery`.
Worktree: `.worktrees/messaging-delivery-contract`.
HEAD was this exact SHA before and after review. No implementation, migration,
topology, deployment, business admission or runtime readiness is approved by QA.
Coordinator approval and promotion remain required.

## Independent checks and evidence

Read the assigned proposal and result, AGENTS, current development state and
messaging-readiness note, architecture §§5/9 and ADR references, T-006/T-008/
T-009/T-013 contracts, the approved T-746 registration contract, product/open
decision constraints, mock map and both pinned Gaze audit summaries. Compared
the proposed boundaries with actual `pkg/eventstore/schema.sql`,
`pkg/eventstore/transaction.go`, `pkg/eventstore/append.go`,
`infra/local/rabbitmq/definitions.json`, and the newer main
`pkg/inbox/consume.go`. Main reference HEAD observed during this check was
`606d5317836676fe6f59ab11a2e7909759b42538`; it is not the proposal QA target.

Executed from the assigned worktree:

- `git rev-parse HEAD`, `git status --short`: exact target; initially clean.
- `git show --stat --oneline HEAD` and `git diff-tree --no-commit-id --name-only -r HEAD`:
  only the assigned architect draft and developer result changed.
- `git diff HEAD^ HEAD --check`: exit 0.
- Independent Python relative-link, changed-path and SHA-256 checks: all eight
  relative Markdown links resolve; exact two-leaf ownership passes. Repeatable
  script and observed hashes are in [static-check.py](messaging-delivery/static-check.py).
- Manual scenario analysis below: no contradictory safety or ownership rule
  found in the selected model. These are design checks, not executable failure tests.

Independently checked the official RabbitMQ documentation, currently labeled
4.3. A mandatory unroutable publish can receive a positive confirm after its
return; publisher confirms and consumer acknowledgments cover different legs.
The proposal's return/confirm correlation and custody distinction match those
semantics. [RabbitMQ confirms](https://www.rabbitmq.com/docs/confirms).
Resource permissions and topic routing permissions are separate; missing topic
permissions do not imply deny-all, and existing connections can cache access
decisions. MD-5 must explicitly install deny-all for empty approved route sets
and verify the post-cutover connections. The migration's channel drain/close
requirement supports that boundary. [RabbitMQ access control](https://www.rabbitmq.com/docs/access-control).
Version-prefixed `/docs/4.3/` URLs did not open through the web tool; the actual
unversioned links used by the proposal opened successfully and showed 4.3.

No Go, PostgreSQL, RabbitMQ, migration or browser suite was run: the reviewed
commit changes only a design proposal. Historical implementation PASS results
were inspected as scope/context, not presented as fresh execution evidence.

## Scenario reasoning

| Scenario | Assessment against the proposal |
|---|---|
| One event targets A and B; only A routes/confirms | One immutable parent and two delivery identities retain identical bytes and event ID. Only A's child can become sent. B remains recoverable; a parent-wide mutable sent flag cannot hide B (§§3/5/7). |
| Required type is irrelevant to one subscriber | Every admitted owner receives every integration position. Known permitted irrelevant types checkpoint as no-ops; unapproved new schema emission blocks rather than being filtered. Internal domain revisions do not create integration gaps (§3). |
| Application supplies an incomplete or expanded destination list | It cannot submit raw recipients through the proposed writer. Transaction-bound admission resolves the exact set, and deferred completeness compares every child and admission reference. Missing configuration differs from an explicitly permitted empty plan (§§3–5). |
| Projection executor is down while process executor works | Only intake competes on AMQP. It commits the complete inertly declared membership before custody ACK; independent jobs remain for the absent executor. One consumer's inbox or checkpoint cannot complete another's job (§6). |
| Commit reply is lost before custody ACK | Unknown is not success. Original delivery remains unacknowledged or channel closes; authoritative read/duplicate intake compares immutable bytes and original enrollment. It does not select today's consumers for an old duplicate (§6). |
| Lease expires during a local effect | Final lease-conditioned completion must succeed inside the same locked transaction as inbox/effect/checkpoint; otherwise all roll back. Late relay completion updates no row, so another claimant retries identical bytes. External calls are excluded from this protection (§7). |
| New local consumer starts at h=10; custody already contains 11 and 13 | Observed 13 is not an authorized contiguous checkpoint. Fenced admission scans authorized retained custody above 10, creates separate immutable enrollment/jobs, and recovers missing 12 through the admitted source port. Old obligations stay intact. Activation is incomplete without required bootstrap/recovery (§6). |
| Source adds a recipient while append runs | Admission and append share a stream fence and compatible sorted lock order. Boundary h separates old/new recipient obligations; snapshot/checkpoint authorization is explicit. The new recipient cannot infer access to earlier personal history or process effects (§3). |
| Consumer removed, renamed or immediately revoked | Drain preserves obligations through the boundary. Missing binary and new catalog do not delete jobs. Immediate revocation holds work pending a separately approved disposition; it makes no recall promise for delivered bytes (§§3/6). |
| Same event ID arrives with changed bytes, or quarantine is unavailable | Immutable conflict is evidence, not overwrite or successful dedup. Failed durable quarantine cannot ACK the original. Handler failure retains its own incomplete job and checkpoint (§§5/6). |
| Valid broker route carries a forbidden company/stream | Broker routing is only a technical ceiling. Source and receiver admission remain mandatory; registration is no grant. Owner-local tables and role credentials provide no cross-owner SQL or queue-read authority (§§3/5/7). |
| Existing outbox has sent, unsent and ambiguous rows | Additive fenced migration preserves original bytes, observed single destination and prior evidence. No inference of missing recipients, no old lease reuse or mixed writer mode, no destructive down conversion (§8). |
| Legacy direct-ACK consumer already has an inbox row | Its prior effect must not repeat. Newly admitted consumers need separate bootstrap/catch-up. Existing inbox evidence can support the original consumer's completed dispatch job only through the explicitly required composable interface below (§§8/9). |

## Confirmed prerequisite for coordinator assignment

**MD-4 needs the narrow T-013 interface amendment; the condition in §9 is now
resolved as necessary.** Actual `Consumer.Consume` constructs and owns its
transaction runner (`consume.go:54–65,97`), skips `apply` for duplicate inbox
rows (`103–118`), and invokes its ACK after its own commit (`123`). Calling it
inside another transaction does not establish the proposed job/effect atomicity.
A no-op ACK callback does not solve that issue. Legacy duplicate recovery also
cannot rely on `apply` to mark the recovered job complete.

The proposal already explicitly gates this missing port and preserves direct-ACK
compatibility, so this is not a contradictory architecture or a proposal BOUNCE.
Before MD-4 becomes ready, the coordinator must assign a separate bounded task
owning `pkg/inbox/consume.go` and `pkg/inbox/consume_test.go` (plus its result/QA)
and record it as a definite dependency. Its design must expose transaction-local
dedup/effect behavior to dispatch, permit completion reconciliation for an
already-committed matching inbox without replaying effects, retain mandatory
schema/scope validation and exact-byte checks, and preserve the current direct
consumer's post-commit ACK behavior. Test lost commits, existing-inbox migration,
stale leases and rollback of job/inbox/effect/checkpoint together. Re-run exact
commit QA; historical T-013 GREEN remains evidence only for its original API.

## Preserved implementation and approval gates

Section 9 gives separate leaves for schema/admission (MD-1), insertion (MD-2),
intake/membership (MD-3), dispatch (MD-4), topology (MD-5), relay, sequence,
quarantine, rebuild, integration proof and owner/Compose wiring. MD-1 may need
reslicing before allocation to meet the effort cap. No concurrent edit of
T-011's files is authorized by this proposal. The coordinator must add actual
IDs/dependencies and promote the ADR-03/05 and T-746 ACK amendment with backups;
the current board does not gain readiness merely from this report.

Deferred SQL completeness, immutable content, inbox-linked completion and
inherited/default privileges need real migration tests. Lease clocks/fencing,
return/confirm correlation, populated legacy migration and crash recovery need
the pinned live evidence required in §10. Structural constraints cannot prove
business authorization or arbitrary callback correctness. The proposal states
these distinctions rather than claiming them implemented.

Concrete source/recipient grants, process catch-up, personal snapshot access,
immediate-revocation disposition and affected OD-10 policy remain unapproved.
No dependency pin, financial rule, seven-owner boundary, or exactly-once broker
promise is introduced. No blocking finding remains for this bounded proposal.
