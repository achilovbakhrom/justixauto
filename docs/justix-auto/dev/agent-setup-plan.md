# HUB bootstrap execution plan

User authorized implementation after reviewing public setups. Base:
`682696f2fea8e874cc9572749f9faadb2b532b4e`. Integration branch:
`feature/agent-hub-setup`. Remote dev is absent; no protected branch is updated.
These are tooling tasks outside the 949-item product backlog.

| Packet | Branch | Owned paths | Acceptance / evidence |
|---|---|---|---|
| HUB-CONFIG | feature/agent-hub-setup | .codex, scoped AGENTS, canonical workflow/status with snapshots, tools/check-agent-config.py, this plan | two authority hubs; shared capabilities; explicit models; fresh contexts; human dev gate; human-only main; TOML/hash lint and independent review |
| HUB-CTRL | task/agent-packet-controller | tools/agent-flow.mjs, tools/agent-flow.test.mjs, tools/agent-flow/** | bounded manifests; independent plan review; multi-task state; global leases; exact-SHA packet evidence; QA without code changes; final-SHA coverage; CLI regression tests |
| HUB-GIT | task/agent-git-gates | tools/agent-git.mjs, tools/agent-git.test.mjs, tools/agent-git/** | scoped conventional commits; same-name task push; protected-ref denial; no manufactured human approvals; disposable-repository CLI tests |

HUB-CTRL and HUB-GIT run in separate explicit worktrees. The primary writes
HUB-CONFIG in its own worktree. Implementation agents cannot write canonical
state or each other's paths. Because the controller is being built, bootstrap
uses this reviewed ownership map and explicit Git/agent records, then independently
checks exact implementation commits before merging into the feature branch.

Independent QA must inspect each actual diff, run tests and probe negative
paths (stale evidence, protected refs, plan changes, conflicting leases). Final
integration QA validates CLI examples, seven-role config, snapshots and source
scope at the combined candidate commit. This is tooling QA; no running business
application, Kubernetes deployment or model-cost improvement is certified.

The feature branch can be pushed under the user's standing authorization. Final
delivery includes its exact SHA and test evidence. Human approval is the last
gate before creating/updating dev. Main remains unchanged. GitHub rules and
cluster configuration are reported as unconfigured until actually verified.
