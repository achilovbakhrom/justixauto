# HUB setup handoff

Branch: `feature/agent-hub-setup`. Bootstrap base:
`682696f2fea8e874cc9572749f9faadb2b532b4e`. No dev/main update or cluster action
is part of this handoff. Product task counts and existing worktrees are retained.

## Exact packet evidence

The coordinator reviewed source changes; independent agents checked frozen
commits and executed disposable-repository tests. These are bootstrap receipts,
not fabricated entries in the controller being built.

| Packet | Verified commit | Independent result |
|---|---|---|
| HUB-CONFIG | `f02309efbe946342e7d95c6fcaa72482aebf6913` | GREEN; role routing, two hubs, global three-delegate policy, protected-branch wording, TOML lint and five pre-edit hashes checked |
| HUB-CTRL | `32f36044f270ac70bc4b170fe6a03954f398b60f` | GREEN; two real-CLI workflow tests and independent recursive recovery/invalidation probe |
| HUB-GIT | `0724234a7d62bc7e0f715a91bf32848fa1411a67` | GREEN; 12 tests plus independent rejected-ref preservation, Unicode/newline paths, protected refs and immutable-push probes |

Rejected iterations remain in Git. Review caught and corrected ambiguous dev
approval wording, premature packet reservation release, repair-range loss,
stale active descendant verification, hook-added out-of-scope commits, missing
remote-dev freshness and rejected-ref source preservation. Reports include
negative checks; passing author tests alone did not authorize integration.

## Combined candidate check

After integrating only GREEN packet commits, run from the clean feature worktree:

```sh
python3.14 -B tools/check-agent-config.py
node --test tools/agent-flow.test.mjs tools/agent-git.test.mjs
git diff --check 682696f2fea8e874cc9572749f9faadb2b532b4e..HEAD
```

Use the workspace-pinned Node 24.21.0 when available. The final combined commit
must receive its own independent integration check before push. A receipt made
before that commit exists does not claim to certify that future SHA. The final
delivery supplies the checked/pushed SHA and observed integration outcome.

## Activation and remaining gates

- Load a fresh Codex session in the chosen feature/dev worktree to discover the
  new role files. The current main checkout still contains the old configuration.
- Human approval is required to create/update dev. Human alone promotes main.
  Task/feature/fix/infra pushes retain standing authorization.
- GitHub branch protection and agent credentials have not been configured or
  certified. Until enforced server-side, human performs protected integration.
- Kubernetes is the selected deployment target, not an installed deployment.
  Cluster context, namespaces, provider, registry and credentials remain unset.
- The controller is a local validation/state helper, not a daemon or authentication
  boundary. Actor identities and evidence are trusted orchestration inputs.
- Use small real tasks to measure total tokens/latency/repairs before claiming
  cost savings. No model-cost improvement or application readiness is certified.
- T-938's board/index discrepancy remains documented for separate reconciliation;
  it was not silently corrected or used to schedule product implementation.
