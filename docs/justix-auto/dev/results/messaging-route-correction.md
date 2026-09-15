# Messaging route correction — architecture result

Status: proposal complete; independent design QA and coordinator approval pending.
Base: `f0d0454e0794dbdc4bddd3cb3b3bb4e331e9a51a`.
Branch: `task/messaging-route-correction`.
Draft: [messaging-route-correction.md](../../state/drafts/architect/messaging-route-correction.md).

Source inspection confirms T-917's schema lookup duplicates the source owner
when it receives T-011's established full-event-type route. Its shortened SQL
fixtures omit that integration boundary. No PostgreSQL reproduction or application
implementation was run during this architecture slice.

Proposed T-925 adds a forward migration and a genuine T-011-through-populated-
cutover regression; installed 000002 and all original bytes/routes remain intact.
A separate immutable correction marker makes readiness explicit without changing
messaging_mode semantics. Exact downstream dependencies and owned leaves are in
the draft; T-918/T-920 admission-only work can proceed independently.

Only the assigned draft and this result changed. No canonical document, board,
application code, dependency, broker, credentials or production state changed.
The containing commit is the exact design review target. Validation: inspected
source anchors and hashes, local Markdown link resolution, and `git diff --check`.
Runtime regression requirements remain proposed acceptance, not reported PASS.
