@AGENTS.md

## Claude Code adaptation

`AGENTS.md` (above) and the scoped `AGENTS.md` files are the single source of
truth; each scoped directory has a `CLAUDE.md` that only imports its `AGENTS.md`.
The `.claude/` setup mirrors `.codex/` — keep the two in sync when roles change.

- Edits: use the Edit/Write tools where `AGENTS.md` says `apply_patch`.
- Roles: the Codex roles exist as project subagents in `.claude/agents/`
  (`devops_orchestrator`, `task_slicer`, `worker`, `reviewer`, `qa`,
  `qa_aggregator`, `version_control`). Dispatch them with the Agent tool by
  exactly those names. Do not use the global `crew-*` agents in this project.
- Models: Codex → Claude mapping is gpt-6-astra → `opus`,
  gpt-5.6-sol → `sonnet` (high effort), gpt-5.6-terra → `sonnet` (medium),
  gpt-5.6-luna → `haiku`. Each role file pins its model and effort; pass an
  explicit `model` override only for deliberate risk escalation.
- Nested spawning: Claude subagents cannot spawn subagents. `devops_orchestrator`
  therefore always returns exact dispatch packets, and the primary session
  dispatches them on its behalf within the global three-delegate budget.
- Never use `isolation: "worktree"` on Agent calls — separate development
  worktrees are forbidden in this project.
