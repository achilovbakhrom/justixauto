# Canonical development state

Only the primary coordinator edits canonical task-board, task-index, dev-state
or workflow documents. Before edits record SHA-256 and a recoverable snapshot.
Workers/slicers write only assigned execution artifacts; read-only reviewers
return evidence which the coordinator persists verbatim. Never mark integrated
on review alone: human-approved dev integration must actually exist in Git.
Historical main integrations remain historical facts. Do not regenerate the
949-task backlog to introduce execution packets. Conflicting status views require
reconciliation against task commits and independent QA before updating status.
