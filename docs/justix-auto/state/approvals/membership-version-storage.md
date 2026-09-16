# Durable membership storage technical adoption

2026-09-16. Coordinator adopts exact proposal `bb908b4425003cbf41375b46b07fb88d2a3be817` after
[independent r3 proposal GREEN](../../dev/qa/membership-version-storage-r3.md).
The [canonical contract](../drafts/contracts/membership-version-storage.md) equals
the reviewed draft, SHA-256 `c468216706f0b40e01dc5c5e2bf07060cac60cecd429eac54ec5616bc9886330`. Historical proposed wording is
preserved; this approval supplies the technical implementation decision.

| Alias | Task | Maximum hours |
|---|---|---|
| MS-CATALOG | T-945 | 4 |
| MS-SELECTION | T-946 | 4 |
| MS-IDENTITY | T-947 | 4 |
| MS-STATE | T-948 | 4 |

Four tasks add16hours:948nodes/3154total effort hours. Eight new leaves are
disjoint; existing ownership remains. Six added edges: T-920→T-948, T-921→T-948,
T-016→T-948, T-022→T-946/T-948 and T-934→T-946. T-591 reaches all four through
existing upstream dependencies and remains the B-01 closer. Reassess each4h
limit and T-920's remaining3h before assignment; reslice before scope expansion.

The contract separates frozen snapshots from future enrollment links. Immediate
bidirectional guards, server xid8 and retained head locks survive early ALL/named
constraints and concurrent head updates. Only two custom old-table trigger
attachments plus declared FK attachments are added; old SQL/columns/grants/
trigger definitions remain unchanged. T-934 verifies exact lineage/objects.
No new dependency family, shared mutable owner table or production policy.

Independent proposal review executed76 reduced observations, not full application
acceptance. Automatic review rejected configuration-changing test setup; the safe
probe omitted all such mutations. Its GREEN does not authorize those tests or
prove effective runtime startup. T-928 remains verification-blocked pending
explicit authorization and independent live checks. T-945 waits for integration;
fresh exact runtime LOGIN/database origin setup, quiescence and actual handle
checks remain outer prerequisites. No configuration mutation/activation is
authorized by this adoption. Pure independent identity work can proceed only
after canonical promotion QA.

Canonical promotion QA must verify exact copies, all944previous task records
except the six dependency additions, coverage/closure, disjoint ownership,
acyclic graph, local links and recoverable preimages before assignment.
Snapshots: `../backups/2026-09-16-membership-promotion/`. All40earlier membership QA artifacts remain
unchanged. T-920 remains blocked until its implementation prerequisites integrate.

## Canonical promotion gate — GREEN

[Independent audit](../../dev/qa/membership-promotion.md) passed the exact
`b50490b48c5f31a542b0305827c1e345c2214cf6` promotion. All documented mapping,
ownership, graph, coverage/closure, exact-copy and preimage checks passed.
The bounded tasks may now be assigned when their dependencies permit. This
does not clear T-928's configuration-test permission or any implementation gate.
