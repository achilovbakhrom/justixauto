# ADR-03/05/13 owner migration compatibility adoption

2026-09-15. Coordinator technical adoption after [independent proposal QA GREEN](../../dev/qa/owner-migration-compatibility.md)
at exact `c3734a5c3f2863be8fe752338c11740b9bdf31a4`. The [canonical contract](../drafts/contracts/owner-migration-compatibility.md)
and architect copy preserve SHA-256 `87ca5a5b7b5398726a53a9f4ec52b4f63c665e08ebaf69f57003401fa9ccbd76`. Their proposed header is historical
provenance; this approval adopts P1-P6 for bounded implementation.

Adopt exact trusted immutable owner artifact/feature evidence and read-only
compatible profiles; preserved original legacy constructors; explicit verified
fresh-v1 versus attested-existing-v1 baseline; separate custody lineage/ACLs;
and actual migrate engine plus project pgx database.Driver/verified source.
This explicitly refines T-001 upstream pgx/file-source guidance under ADR-13.
No new dependency pin is selected and no lib/pq is authorized. Coordinator inserts
the already approved migrate module/transitives before the affected exact-SHA QA.

| Proposal alias | Task |
|---|---|
| OM-HISTORY | T-929 |
| OM-PROFILE | T-930 |
| OM-IDENTITY | T-931 |
| OM-DRIVER | T-932 |
| OM-RUNNER | T-933 |
| OM-CUSTODY | T-934 |

Six four-hour bounds add 24 task-effort hours: 934 tasks/3,102 hours total.
Eight explicit edges amend T-059/060/022/580. T-059 alone gains its adjacent auth
manifest; T-931 serially succeeds T-024 UOW ownership, and T-934 serially extends
T-930 common checker leaves. Thirteen newly assigned file leaves are unowned in
the prior graph. B-01.AC1 still closes only at T-591; historical graph width/
schedule metrics are not recomputed completion forecasts.

Recorded P5 later-owner follow-ups remain unassigned. OWNER-COMPATIBILITY on
T-581–T-586 and the board's schema-manifest rule make that existing requirement
explicit; no such root is authorized to bypass its legacy UOW. These gates may
clear only through concrete reviewed ownership/dependency follow-ups. No new
business rule, service/data ownership or policy gate is inferred.

QA's feasibility limits are mandatory: bound all of Lock including preflight;
clean fresh-v1 checks its original mechanics marker without nonexistent history,
and that exception is inaccessible to existing/feature upgrades. After baseline,
receipt and clean head commit together; missing Run, Force, ambiguous outcomes,
retained dirty state and lower-version gaps cannot yield automatic success.
Keep the original SQL BEGIN/COMMIT and byte hashes. No global administrative
tamper-proofness, uninterrupted cross-session lock or automatic repair claim.

Canonical promotion verification is required before assigning the six tasks.
They then require real pinned PostgreSQL/engine fault tests and independent exact
commit implementation QA. Baseline attestations, real release profiles, later
owner rollout, production settings, auth transport, source admissions and business
activation remain separate. No migration, service or deployment ran here.
Recoverable preimages: `../backups/2026-09-15-owner-migration-promotion/`.
