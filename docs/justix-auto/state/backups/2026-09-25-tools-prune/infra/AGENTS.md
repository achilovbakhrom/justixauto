# Infrastructure authority

The devops_orchestrator owns Kubernetes/image/CI/CD decisions. Generic workers,
reviewers and QA execute its bounded packets. New infrastructure dependencies or
operational guarantees need a recorded ADR. Kubernetes is selected; cluster,
registry, namespace and GitOps tool are not inferred from that decision.
Use immutable image references, explicit resources/probes, owner isolation and
least-privilege secret references as required by the approved design. Never print
secret values. Validate manifests offline first. Live operations require named
context, namespace, environment and authorized verbs. Production is human-only.
Human approval gates dev Git integration. No unrestricted deployment operator.
