#!/usr/bin/env python3
"""Independent document/example checks, not an auth implementation test."""
import copy
import hashlib
import json
from pathlib import Path
import re
import subprocess
import uuid

ROOT = Path(__file__).resolve().parents[5]
SHA = "fe3ffbfbe37d05261296a0ef5ba91cbec8edcead"
BASE = "c135e4e33190ad4e30c4f573f133e25fd5614f21"
PROPOSAL = "docs/justix-auto/state/drafts/contracts/auth.md"
checks = []

def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT, text=True)

def check(name, condition):
    assert condition, name
    checks.append(name)

check("exact HEAD", git("rev-parse", "HEAD").strip() == SHA)
changed = git("diff", "--name-only", BASE, SHA).splitlines()
check("only owned proposal and result changed", sorted(changed) == sorted([
    PROPOSAL, "docs/justix-auto/dev/results/T-058.md"]))
check("commit whitespace", git("diff", "--check", BASE, SHA) == "")
doc = git("show", f"{SHA}:{PROPOSAL}")
check("proposal worktree bytes match commit", (ROOT / PROPOSAL).read_text() == doc)
blocks = re.findall(r"```json\n(.*?)\n```", doc, re.S)
check("two JSON blocks", len(blocks) == 2)
cases, session = [json.loads(b) for b in blocks]
check("25 distinct fixture IDs", len(cases) == len({c["id"] for c in cases}) == 25)

def ident(value):
    try:
        return isinstance(value, str) and str(uuid.UUID(value)) == value and uuid.UUID(value).int != 0
    except (ValueError, TypeError, AttributeError):
        return False

def revision(value, positive=False):
    return (isinstance(value, str) and re.fullmatch(r"0|[1-9][0-9]*", value) is not None
            and int(value) <= 9223372036854775807 and (not positive or int(value) > 0))

def event(value):
    return (isinstance(value, dict) and set(value) == {"userId", "securityRevision", "change"}
            and ident(value["userId"]) and revision(value["securityRevision"], True)
            and value["change"] in {"credential-created", "credential-replaced", "mfa-enrolled",
                                     "sessions-revoked", "bootstrap-enrollment-started"})

request_fields = {
    "LoginRequest": {"login", "password"}, "VerifyRequest": {"challengeId", "code"},
    "EmptyRequest": set(), "RevokeAllRequest": {"reason"}, "RecoveryRequest": {"login"},
    "RecoveryCompleteRequest": {"token", "newPassword", "confirmation"},
}
for case in cases:
    name, value = case["id"], case["input"]
    if case["schema"] == "SecurityChangedData":
        check(name, event(value) == case["expected"]["valid"])
        continue
    check(name + " known alias", case["schema"] in request_fields)
    structurally_valid = (set(value) == request_fields[case["schema"]]
                          and all(isinstance(v, str) for v in value.values()))
    check(name + " structural expectation", structurally_valid == (case["expected"].get("status") != 400))

valid = next(c["input"] for c in cases if c["id"] == "auth.event.valid")
for bad in [0, 1, None, "", "0", "01", "+1", "-0", "1.0", "1e3", "1\n", "9223372036854775808"]:
    value = dict(valid, securityRevision=bad)
    check("event rejects revision " + repr(bad), not event(value))
for key in ["password", "token", "credentialHash", "csrf", "login", "reason", "actorId", "nested"]:
    check("event rejects extra " + key, not event(dict(valid, **{key: "fixture-secret"})))
for bad in ["00000000-0000-0000-0000-000000000000", "not-an-id", None]:
    check("event rejects user ID " + repr(bad), not event(dict(valid, userId=bad)))
check("event rejects unknown transition", not event(dict(valid, change="admin-granted")))
check("event accepts largest revision", event(dict(valid, securityRevision="9223372036854775807")))

def session_consistency(value):
    try:
        d = value["data"]
        c = d["context"]
        scope = c["branchScope"]
        return (set(value) == {"data", "revision", "asOf"}
                and set(d) == {"user", "roles", "permissions", "mfa", "context", "accessibleCompanies", "setup"}
                and d["user"]["status"] == "active" and ident(d["user"]["id"])
                and revision(value["revision"]) and value["revision"] == c["revision"]
                and (c["companyId"] is None or ident(c["companyId"]))
                and ((scope["mode"] == "ALL" and scope["branchIds"] == [])
                     or (scope["mode"] == "SELECTED" and c["companyId"] is not None
                         and len(scope["branchIds"]) > 0
                         and len(scope["branchIds"]) == len(set(scope["branchIds"]))
                         and all(ident(i) for i in scope["branchIds"])))
                and (d["mfa"]["enrolled"] or "authenticatedAt" not in d["mfa"]))
    except (KeyError, TypeError):
        return False

check("positive SessionRead consistency", session_consistency(session))
mutations = [
    ("mismatched revision", lambda s: s.update(revision="1")),
    ("numeric revision", lambda s: s.update(revision=0)),
    ("token response", lambda s: s.update(token="fixture-token")),
    ("pending full session", lambda s: s["data"]["user"].update(status="pending")),
    ("MFA timestamp without factor", lambda s: s["data"]["mfa"].update(authenticatedAt="2026-09-15T00:00:00Z")),
    ("null company selected", lambda s: s["data"]["context"]["branchScope"].update(mode="SELECTED")),
    ("ALL contains branch", lambda s: s["data"]["context"]["branchScope"].update(branchIds=[valid["userId"]])),
]
for label, mutate in mutations:
    value = copy.deepcopy(session)
    mutate(value)
    check("session rejects " + label, not session_consistency(value))

links = re.findall(r"\[[^\]]+\]\(([^)]+)\)", doc)
for link in links:
    target = (ROOT / PROPOSAL).parent / link.split("#")[0]
    check("local link " + link, target.is_file())
check("five explicit refinements", re.findall(r"^\| (A[1-5]) \|", doc, re.M) == ["A1", "A2", "A3", "A4", "A5"])
for label, pattern in {
    "unapproved security proposal": "is **unapproved**",
    "Origin and CSRF both mandatory": "Origin **and** matching `X-CSRF-Token`",
    "one factor counter across challenges": "unique per factor+counter across",
    "unknown outcome": "AUTH_OUTCOME_UNKNOWN",
    "disabled recovery does not look up": "no account lookup/delivery",
    "owner migration gate": "UOW.Check accepts clean owner ledger version 1",
    "recipient admission gate": "absent configuration cannot mean broadcast",
    "nonsecret provider hash": "normalized **nonsecret** input",
    "proposal does not execute tests": "not executed backend tests",
    "no generic credential receipt": "No\ngeneric receipt stores them",
}.items():
    check(label, pattern in doc)

codec = git("show", f"{SHA}:pkg/events/envelope.go")
for field in ["eventId", "eventType", "schemaVersion", "aggregateType", "aggregateId",
              "aggregateVersion", "integrationSequence", "companyId", "occurredAt", "actor",
              "correlationId", "causationId", "operationId", "data"]:
    check("existing envelope field " + field, 'json:"' + field + '"' in codec)
check("no envelope owner field", 'json:"owner"' not in codec)
check("identity null-company codec", "input.CompanyID == nil && owner != OwnerIdentity" in codec)

print(json.dumps({"outcome": "PASS", "reviewedSHA": SHA,
                  "proposalSHA256": hashlib.sha256(doc.encode()).hexdigest(),
                  "checks": len(checks), "results": checks,
                  "limits": "Document/example validation only; no runtime, cryptographic, browser or production guarantee."}, indent=2))
