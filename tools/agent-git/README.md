# agent-git

`node tools/agent-git.mjs` is the intentionally limited Git helper for a task
worker. It only runs at the exact root of its own repository/worktree; it never
discovers an enclosing repository.

Generic workers do not run Git. A designated version-control agent uses this
helper to create its branch/worktree and perform only the bounded operations
below; direct Git access by an agent is consequently a workflow violation, not
something this local CLI can technically prevent.

Supported commands:

```sh
node tools/agent-git.mjs create-worktree --repo /repo --branch task/HUB-GIT --expected-dev 0123456789abcdef0123456789abcdef01234567 --path /tmp/hub-git
node tools/agent-git.mjs commit --expected-head 0123456789abcdef0123456789abcdef01234567 --task HUB-GIT --message 'feat(HUB-GIT): add gate' --path tools/agent-git.mjs
node tools/agent-git.mjs push --expected-head 0123456789abcdef0123456789abcdef01234567
node tools/agent-git.mjs prepare-dev --expected-head 0123456789abcdef0123456789abcdef01234567
```

Only `feature/`, `task/`, `fix/`, and `infra/` branches may be created,
committed, pushed, or prepared for review. `dev`, `main`, and `master` are
always denied. `create-worktree` bases strictly on local `dev`; for a first
setup where `dev` does not exist, `--bootstrap-base <40-character SHA>` is
required and is used only as the new task branch's base. It does **not** create
or move `dev`. Where `origin` is configured, the expected local `dev` SHA must
also match `origin/dev`; a bootstrap is denied if `origin/dev` exists, and an
origin lookup failure fails closed. A repository without `origin` is explicitly
local-only and has no remote freshness assertion.

Commits require an expected current HEAD plus one or more exact relative file
paths. They use Git's literal pathspec mode and reject symlinks (including a
dangling symlink in the index), `.git`, traversal, magic/glob paths,
directories, and any already-staged out-of-scope file. They require a conventional
one-line message containing the supplied task or packet ID. Push uses the
validated immutable source SHA to `refs/heads/<current>`, never force, and validates the
local expected SHA and remote resulting SHA. `prepare-dev` is read-only and
prints the source branch/SHA, current remote `dev` SHA, and reviewable commits.
It deliberately has no approval flag.

This CLI is not a security boundary and cannot manufacture human authority.
Development integration must happen through a human-capable external gate:
human GitHub PR approval of the exact source head plus protected branches.
Agents must not directly write, merge, or push `dev`/`main`; promotion to
`main` remains a human-only action. The tool makes no branch-protection changes
and performs no deployment.

Run its dependency-free integration suite with:

```sh
node --test tools/agent-git.test.mjs
```
