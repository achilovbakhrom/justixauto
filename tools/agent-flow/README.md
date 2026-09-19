# agent-flow

`agent-flow` is a zero-dependency Node controller for bounded task packets. It
stores one multi-task ledger in `<git-common-dir>/justix-agent-state`; linked
worktrees therefore share its writer lock, active-packet capacity (three), and
`browser`/`postgres` resource reservations.

Every task command names `--task`. `status` deliberately returns a compact
summary; `audit` is the explicit full ledger/history view.

The CLI resolves the shared Git common directory through its current working
directory and deliberately requires that checkout to be clean. Run `status`,
`audit`, plan actions and hub actions from a clean hub checkout when an
implementation worktree has uncommitted changes.

## Manifest v2 (schema excerpt)

```json
{
  "schema": 2,
  "task_id": "HUB-CTRL",
  "planner_actor": "planner.a",
  "hub_actor": "hub.owner",
  "hub": "infrastructure",
  "base_sha": "<40-character base commit>",
  "contracts": [{"path": "contracts/hub.md", "sha256": "<sha256>"}],
  "requirements": ["REQ-ONE"],
  "packets": [{
    "id": "PKT-IMPL", "kind": "implementation", "depends_on": [],
    "owned_paths": ["tools/agent-flow.mjs"],
    "brief": {"path": "briefs/impl.md", "sha256": "<sha256>"},
    "acceptance": ["REQ-ONE"], "resources": []
  }]
}
```

The excerpt shows one packet only; a runnable workflow which names `PKT-QA` and
`PKT-INTEGRATION` must declare those packets and their dependency edges. `kind`
is `implementation`, `qa`, or `integration`. Verification packets have
no owned paths and run `verify` at their claim-pinned clean `HEAD`; they do not
create a code submission. Every requirement needs coverage. Contracts and
briefs are rehashed on each claim and must be safe non-symlink, relative paths
outside `.git`.

## Runnable workflow

Run these from a clean registered Git worktree after creating a valid plan:

```sh
node tools/agent-flow.mjs init /absolute/path/HUB-CTRL.json
node tools/agent-flow.mjs register-worktree --worktree "$PWD"

# `init` prints the digest used here. The plan reviewer cannot be planner_actor;
# only hub_actor can record accept-hub.
node tools/agent-flow.mjs review-plan --task HUB-CTRL --digest <manifest-digest> \
  --actor reviewer.plan --verdict GREEN --evidence 'schema and contracts checked'
node tools/agent-flow.mjs accept-hub --task HUB-CTRL --actor hub.owner \
  --evidence 'explicit bounded-hub decision'

node tools/agent-flow.mjs claim implementation --task HUB-CTRL --run run-1 \
  --actor worker.a --worktree "$PWD"
# Make only owned changes, commit them, and leave the worktree clean.
node tools/agent-flow.mjs submit PKT-IMPL --task HUB-CTRL --run run-1 \
  --actor worker.a --worktree "$PWD"
node tools/agent-flow.mjs review PKT-IMPL --task HUB-CTRL --actor reviewer.code \
  --verdict GREEN --evidence 'owned diff and code review: PASS' --worktree "$PWD"
node tools/agent-flow.mjs qa PKT-IMPL --task HUB-CTRL --actor qa.code \
  --verdict GREEN --evidence 'independent checks: PASS' --worktree "$PWD"
node tools/agent-flow.mjs accept PKT-IMPL --task HUB-CTRL --actor hub.owner \
  --evidence 'accept exact reviewed SHA' --worktree "$PWD"

# A dependent QA packet needs no code commit: it verifies its pinned HEAD.
node tools/agent-flow.mjs claim qa --task HUB-CTRL --run run-qa \
  --actor qa.packet --worktree "$PWD"
node tools/agent-flow.mjs verify PKT-QA --task HUB-CTRL --actor qa.packet \
  --verdict GREEN --evidence 'QA command: PASS' --worktree "$PWD"
node tools/agent-flow.mjs accept PKT-QA --task HUB-CTRL --actor hub.owner \
  --evidence 'accept QA packet' --worktree "$PWD"
```

An integration packet follows the same `claim integration` → `verify` →
`accept` sequence. `aggregate` then verifies that the current clean final HEAD
descends from the manifest base and every implementation submission, and that
all integration verification/acceptance receipts are for that exact current
HEAD. A post-acceptance commit invalidates aggregation. Explicitly reopen the
accepted integration packet, then claim/verify/accept it again at the new HEAD:

```sh
node tools/agent-flow.mjs recover PKT-INTEGRATION --task HUB-CTRL \
  --reason 'final code changed; integration must be rechecked'
```

```sh
node tools/agent-flow.mjs aggregate --task HUB-CTRL --actor vc.agent \
  --evidence 'coverage and ancestry verified' --worktree "$PWD"
```

The repository test is also a complete executable fixture: it creates a
temporary Git repository plus a multi-task plan, exercises real CLI commands,
and verifies implementation, QA-only, integration and aggregation behavior.

```sh
node --test tools/agent-flow.test.mjs
```

## Boundaries

The tool uses atomic replacement and a local exclusive writer directory, exact
Git SHAs/ancestry, clean canonical worktrees, and an unkeyed integrity seal to
catch accidental or unsealed state edits. Actor IDs and evidence are trusted
coordinator claims: this tool does not authenticate identities, sign receipts,
prove commands were honestly executed, approve a human decision, create a
merge, or update `dev`/`main`.
