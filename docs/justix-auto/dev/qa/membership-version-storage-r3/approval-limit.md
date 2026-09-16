# R3 probe execution boundary

2026-09-16. The initial planned command was:

`python3 docs/justix-auto/dev/qa/membership-version-storage-r3/independent_probe.py`

Automatic approval review rejected process creation before the script or any
fixture ran. The tool returned:

> This action was rejected due to unacceptable risk. Reason: The script performs persistent ALTER ROLE and database-level session_replication_role changes and runs Docker-backed database mutations; the user authorized development generally but not these security-setting changes or their blast radius. Do not bypass this rejection through a workaround or indirect execution.

The rejected plan/source remains as evidence and must not be rerun without
appropriate authority. Neither its fresh-runtime-login success assertions nor
the recovered author's parameter-grant/default-setting scenarios were executed
in this QA round. No server-wide configuration mutation was attempted.

The approved safer alternative is `safe_probe.py`, a standalone inspected
script with no dynamic exec/eval. It removes all ALTER ROLE/DATABASE/SYSTEM,
replication-role SET, parameter-grant/revoke and reload operations. It only
mutates the synthetic schema in one new UUID-named/labeled, network-none,
port-free, tmpfs-only owned container. It reads parameter privileges and catalog
settings without changing them. Exact fixture cleanup is recorded separately.
Static exclusion checks are reproducible in `audit.py` and were checked before
requesting this command. Automatic review approved the safer command.

Effective fresh runtime-login verification, the exact runtime LOGIN/database
origin setup, and full dangerous-default/reachable-parameter negative tests
remain unexecuted here. The final proposal explicitly assigns those to
approved external provisioning plus T-022/T-933/T-934 verification and gates
MS-CATALOG on independently approved/integrated T-928. This architecture QA
does not approve configuration changes or claim deployment readiness.
