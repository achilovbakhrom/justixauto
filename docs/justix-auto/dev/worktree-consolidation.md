# Single-checkout consolidation — 2026-09-19

User requested one combined branch and no development worktrees.
Continue in `/Users/bakhromachilov/startups/justixauto` on
`feature/worktree-consolidation`.

## Included history

The branch starts from `feature/agent-hub-setup` at `ff9b2bb`, including all new
agent definitions and the latest branch-only rules. Nine remaining task tips
were merged without conflicts. All 81 previously registered additional worktree
HEADs are ancestors of the combined branch; earlier revisions are not discarded.
Existing task branches and protected main remain unchanged; dev was not created.

The additional histories cover auth transport, membership storage, messaging
route correction, owner migration compatibility, projection storage handoff,
T-002, T-920, T-928 and T-933. This is preservation/consolidation, not approval of
draft policies or a declaration that unfinished tasks passed QA.

## File preservation and local archive

[Inventory](../state/worktree-consolidation/2026-09-19/inventory.json) records each
original path, branch, HEAD, archive location and every untracked-file SHA-256.
There were no uncommitted tracked-source edits in the 81 additional checkouts.
Of 676 untracked QA files, 671 were byte-identical to files already present in
the main checkout. Five divergent versions are retained under the inventory's
`variants/` directory without overwriting the current versions.

Whole old directories, including ignored files, are retained locally under:

```text
.git/worktree-archive/2026-09-19/trees/<original-name>
```

The archive also contains the original worktree metadata and inventory. It is
local recovery storage, not part of Git commits or remote pushes. Archival uses
same-filesystem directory renames, verified by device/inode identity and QA file
hashes. Only after preservation checks are stale Git registrations pruned. No
worktree source directory is recursively deleted. Do not run development in the
archived directories: their old `.git` pointer files are historical and inactive.
Recover a specific file by copying it into the main checkout on an appropriate
branch; committed versions remain accessible through Git history.

The completion receipt `archive-verification.json` next to the inventory records
the observed final registration count and preservation checks after archiving.

## Verification and remaining work

- Config lint: `python3.14 -B tools/check-agent-config.py` — PASS.
- Git whitespace checks — PASS for working source and handoff docs. Preserved
  historical QA variants are excluded: T-034's original trailing blank line is
  deliberately retained so its bytes and SHA-256 do not change.
- Targeted T-933 unit tests did not execute: the pinned Go installation lacks
  standard-library packages `crypto/pbkdf2`, `encoding/xml` and `os/user`.
  The subsequent eventstore compile check was not run after that setup failure.
- No PostgreSQL fixtures, live migrations, Kubernetes actions or production
  operations were performed. T-928 live QA and T-933 re-QA remain outstanding.
- This migration makes no product-task status complete and does not silently
  resolve the historical T-938 task-board/index disagreement.
- Agent tooling tests that create temporary Git worktrees were not rerun as
  part of this branch-only migration. Earlier exact-commit receipts remain
  historical evidence, not certification of this new combined branch.
- No remote push, dev integration or main promotion is authorized by this
  consolidation. Existing human approval gates continue to apply.
