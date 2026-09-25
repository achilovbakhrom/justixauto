# Agent hubs and packet execution

Status: feature-branch handoff; see [exact-commit evidence](agent-setup-results.md).
Human approval is still required for dev creation/integration.
Decision authority: the user's September 2026 conversation approving this design.
This supersedes old agent identities, inherited-model defaults and automatic
main integration in historical task documents or installed development skills.

Local setup checks: `python3.14 tools/check-agent-config.py` (Python >= 3.11)
and `node --test tools/agent-*.test.mjs` (Node >= 22; use the project-pinned
toolchain when available). These checks never touch a live cluster or Git remote.

## Authority and capabilities

The primary Codex session is the application/delivery orchestrator (Astra/max).
It owns product decisions within approved scope, scheduling, canonical records
and final acceptance. `devops_orchestrator` (Astra/xhigh) owns infrastructure
architecture, Kubernetes, CI/CD and operational readiness. Neither changes a
shared runtime contract alone. Unresolved product rules still require the human.

Both hubs use the same capability pool:

| Capability | Model / effort | Artifact or action |
|---|---|---|
| task_slicer | gpt-6-astra / high | Small packets, dependency graph, acceptance coverage |
| worker | gpt-5.6-terra / medium | One bounded implementation, no Git writes |
| reviewer | gpt-5.6-sol / high | Independent plan or exact-diff review, read-only |
| qa | gpt-5.6-sol / high | Small executable verification packet and evidence |
| qa_aggregator | gpt-5.6-sol / high | Coverage audit and final-SHA integration checks |
| version_control | gpt-5.6-luna / medium | Branches in the main checkout, scoped commits, feature pushes |

Role TOML files define model defaults. The current tool catalog may still expose
old roles until a new session loads configuration. During bootstrap use a generic
agent with an explicit model, fresh context and the new role instructions. Do not
claim a new custom role has been discovered merely because its TOML parses.
For risk escalation create a fresh explicit-model job with the same packet and
findings; ensure the selected role config does not override the intended model.

One global budget permits at most three active delegates. A DevOps hub occupies
one slot. Both hubs reserve remaining slots with the primary; never each run an
independent three-worker pool. One implementation writer in the main checkout, one browser
QA and one holder of a named live fixture. Review/QA use immutable checkpoints;
do not let an implementation worker edit the source under an active QA process.
The latest user instruction forbids separate development worktrees. Execute code
tasks sequentially on their branches in the main project folder; read-only planning
may run in parallel when it does not depend on changing source. Never switch the
shared branch while another agent uses its source. Preserve old worktrees as history.

## Persistent truth and context

Approved requirements/contracts and explicit user decisions are authoritative.
The orchestrators are decision authorities, not a substitute for durable records.
The canonical backlog remains the existing 949 tasks. HUB setup is a separate
tooling change; execution packets do not add permanent product task IDs.

The slicer writes artifacts in its assigned execution directory. It records
canonical task identity, source hashes, base SHA, authority hub, requirement IDs,
packet dependencies, owned paths, scoped rules and decision gates. Each packet
has one coherent behavior, acceptance criteria, required checks and an explicit
context limit. The controller validates its supported JSON format; see
`tools/agent-flow/README.md` for the exact commands and schema.

Read only the current packet and its referenced rules. Handoffs contain paths,
identifiers and short findings, never full transcripts. Start independent workers,
reviewers and QA with fresh contexts. Supply scoped AGENTS.md paths explicitly:
automatic discovery depends on working directory, so root-launched workers must
not assume every nested file was automatically loaded.

Keep worker reports and raw evidence on disk. Read-only reviewer/aggregator
responses are persisted verbatim by the hub with actor identity and run ID.
Summaries link to raw evidence; missing usage is unknown, not zero. Checkpoints
contain current decisions, next ready packet, active jobs, findings and blockers.
Resume from those checkpoints and Git, not from an accumulated chat summary.

## Execution protocol

1. Reconcile canonical task, dependency status and actual Git history. Record any
   disagreement; do not invent a completion state. Check own repository root.
2. Slicer decomposes implementation and verification, including cross-packet
   interfaces. Reviewer independently checks the plan and requirement coverage.
   Owning hub accepts the reviewed manifest digest before dispatch.
3. Version control creates a task branch from dev in the main checkout. Pin source contracts
   and base SHA. Register the plan with the controller; reserve writer/resources.
4. Worker edits its owned paths and runs checks. Version control creates a scoped
   commit. Submission records the complete before..after range, not HEAD~1.
5. Independent reviewer inspects the packet diff and necessary adjacent code.
   QA runs its bounded behavioral checks at that immutable SHA. Both report
   findings/exclusions. A worker's self-report does not replace independent QA.
6. Owning hub accepts only current GREEN review and QA. On failure, preserve
   reports and produce a corrective packet. After two unsuccessful repairs,
   diagnose/reslice or explicitly escalate the model; no blind retry loop.
7. Aggregator checks coverage and proposes final-SHA integration QA packets.
   QA executes them in fresh contexts. Earlier results remain historical;
   code changes require renewed affected checks. Final acceptance covers the
   candidate plus its tested dev base, not just individually passing diffs.
8. Version control pushes the allowed task branch and prepares a human-readable
   dev candidate. The human approves the exact candidate. Only the approved
   GitHub integration mechanism updates dev. Agents cannot promote main.

Work packets should be small enough to reason about, not mechanically one line
each. Batch closely related repetitive edits to amortize dispatch/review cost.
Measure total input/output tokens, repair attempts and elapsed time per accepted
packet, including planning/review/QA. Do not promise savings from model price alone.

## Infrastructure decisions and execution

DevOps owns image standards, Kubernetes topology, environment isolation, resource
limits, probes, ingress, networking, secret references, observability, migration
execution and rollback. Application owns domain semantics and schema content.
Record both authorities for changes to runtime interfaces or delivery guarantees.

Kubernetes is selected by the user. Cluster provider/context, dev/prod namespaces,
registry and bootstrap credentials remain unconfigured until explicitly supplied
or discovered in an authorized environment. Do not invent a context or secret.
Argo CD is an evaluated GitOps candidate, not a selected/installed dependency.
Use an ADR before introducing it. If adopted, GitOps performs routine reconciliation
after approved Git promotion; a deployment LLM is not needed for each rollout.

Infrastructure workers can edit/test manifests offline. Live mutations need a
packet naming environment/context/namespace, allowed operations and authorization.
Infrastructure authority is not blanket access to clusters. Production promotion
and production operations are human-controlled.

## Git and human gate

Allowed agent branch prefixes: task/, feature/, fix/, infra/. Branches are
independent work units based on dev; commits carry task/packet intent. Push only
same-name origin refs at a verified SHA. No force, wildcard staging, automatic
conflict resolution, history rewriting or cleanup of unverified worktrees.
Do not use the legacy `agent-git create-worktree` or `agent-git commit` commands:
both create worktrees. Use explicit-path Git commits in the main checkout, checking
the staged diff before committing and the actual commit diff afterward. The
helper's inspect, push and prepare-dev commands do not require extra worktrees.
Controller `--worktree` fields are legacy names for the main checkout path; they
do not authorize additional checkouts. Do not infer workflow permission from a
legacy helper's available commands or disposable test fixtures.

Every dev merge/push requires human approval. Prefer protected GitHub PRs with
required checks, a human reviewer, dismissal of stale approvals and no agent
bypass identity. Approval binds the source SHA and tested target base. If either
changes, revalidate affected checks and request renewed approval. A local JSON
file saying `human_approved` is not authentication and cannot authorize a merge.

The local Git adapter intentionally refuses writes to dev/main; it prepares
candidates. Human-reviewed GitHub integration is an external gate. Until branch
protection and its permissions are verified, the human performs dev integration.
Prompt instructions and local scripts do not confine an agent that also has
unrestricted Git credentials. Repository rules and credentials must enforce this.

The human alone promotes dev to main/production. Agents may prepare a release
summary and evidence. No agent may merge/push main or create a production release.

Bootstrap exception: remote and local dev were absent at inspection. The HUB
branches start at main 682696f2fea8e874cc9572749f9faadb2b532b4e; this provenance is
explicit. Creating/pushing dev requires human approval at the end of bootstrap.
Existing product task worktrees/history are retained, not rebased en masse.

## Controller boundaries and adoption

`tools/agent-flow.mjs` is a local state/validation CLI, not an unattended agent
daemon. The Codex hubs dispatch agents. State is shared via the repository's Git
common directory, excluded from commits; durable exported reports belong in the
assigned execution artifacts. Preserve that local state when moving repositories.
Only the controller mutates its state. Global lock and revision checks prevent
accidental concurrent updates; they are not a hostile-process security boundary.
Actor IDs/reports are attributed orchestration inputs, not signed attestations.
The hub must preserve actual independent runs and raw test evidence.

Do not install multiple orchestration frameworks over the existing backlog.
Augani is a close candidate, but its external-CLI/account routing and runtime
policies need audit before adoption. This first implementation uses a small,
dependency-free controller and native Codex roles to make project gates testable.

References inspected:
- https://github.com/Augani/agent-orchestrator — bounded contexts and checkpoints
- https://github.com/gsd-build/get-shit-done/blob/main/agents/gsd-plan-checker.md — plan review
- https://github.com/obra/superpowers/blob/main/skills/subagent-driven-development/SKILL.md — scoped review/repair
- https://github.com/openai/symphony/blob/main/SPEC.md — scheduling/reconciliation
- https://developers.openai.com/codex/subagents — role configuration
- https://github.com/argoproj/argo-cd/blob/master/docs/user-guide/auto_sync.md — GitOps candidate
