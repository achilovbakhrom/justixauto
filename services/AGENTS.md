# Go owner services

Read the assigned packet and approved owner contracts. Run `bash tools/go.sh`.
Keep domain/app/port/adapter dependencies and explicit composition roots. The
seven service owners have separate mutable data and database credentials.
Application code consumes ports; never import another owner's adapters/domain
or query another owner's business tables. Replay is authoritative; distinguish
missing aggregates from storage failure. Use approved fixed-precision money.
Commands, expected revision, receipts, projections and subscribers follow approved
contracts. Test invariants, rollback, concurrency and authorization relevant to
the packet. Narrow tests to changed owner/packages and affected consumers.
Workers never commit or deploy. Return results to the owning hub.
