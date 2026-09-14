# JustixAuto architecture/planning intake — 2026-09-13

Source root: `/Users/bakhromachilov/startups/justixauto/docs/justix-auto`.
Target repo: `/Users/bakhromachilov/startups/justixauto`.
User explicitly asks to continue architecture and small-task documentation.
Only docs are authorized now; no application implementation or Git init.
Git check fails (own root absent; inherited `/Users/bakhromachilov/startups`).
This does not block drafting architecture/backlog/tasks. Architecture and release
scope have not been approved: task files must remain `proposed`, never `ready`.

Confirmed: Go microservices, CQRS/Event Sourcing in Gaze's hexagonal shape;
React; four existing HTML mock apps; no Figma or retired dealer/distributor apps.
Read business-logic.md, open-decisions.md, reference/gaze-reference.md and mock-map.md.
Do not inherit legacy aggregate screens.md from spec-team as current UI truth.

Working ownership proposal for slice consistency (review, do not claim approved):
identity (users/roles/memberships/companies/branches), inventory (canonical VIN,
warehouse capacity/placement/reservation), commerce (partnerships/B2B offers,
RFQ/quotes/orders/shipments), retail (CRM/listings/sales/own installments),
financing (programs/applications/conditions/document requests), insurance
(underwriting applications only), documents (blob metadata/version/scan; NOT
business approval). Edge routing is infrastructure, not another domain owner.
Invoice/payment-evidence facts stay with originating commerce/retail domain;
no independent money-moving billing service in this release proposal.

Only inventory grants/releases/finalizes reservations, shared by wholesale and
retail. Cross-service processing uses operation IDs + durable process managers,
not distributed SQL transactions or independently recreated VIN records.
Gaze publication gaps must be documented. Outbox/confirm-aware relay/inbox is a
PROPOSED reliability hardening ADR requiring approval, not claimed existing Gaze.
No bank funding, real policy issuance, ownership/legal/GAI rules invented.

Return concrete entities/commands/events/edge contracts for your slice and small
task candidates (2–4 engineering hours, one coherent command/adapter/test unit).
Dependencies on OD-* affect only that operation; separate policy-blocked tasks
from safe foundations. Missing mock states block FE parity tasks, not all backend.
Use module-local contracts; service root/framework/version selection is synthesis.
Workers write only under this draft folder, never target canonical files. No agents
spawned by workers. Final reports <=6 lines, file-backed. Coordinator synthesizes.
