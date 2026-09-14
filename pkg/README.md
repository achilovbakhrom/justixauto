# Shared Go infrastructure — reserved

Candidate infrastructure matches Gaze: event store, aggregate/replay base,
event bus, database, logging/tracing, config and shutdown. No trading domain,
secrets, binaries or untested snapshot/dispatch implementation has been copied.
Reliability changes need an explicit ADR and tests before reuse.
