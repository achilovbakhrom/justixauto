"""Read-only exact-commit and recoverable-preimage verification for T-927 QA."""
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[5]
SHA = "5d8edb0167e654bdb3db8cee26f028c50e8eb1fa"
def git(*args):
    return subprocess.check_output(["git", "-C", str(ROOT), *args])

assert git("rev-parse", "HEAD").decode().strip() == SHA
source = ["pkg/eventstore/migrations/000005_quarantine_evidence.up.sql",
          "pkg/eventstore/quarantine_migration_test.go"]
assert set(git("diff-tree", "--no-commit-id", "--name-only", "-r", SHA).decode().splitlines()) == set(source + ["docs/justix-auto/dev/results/T-927.md"])
artifacts = source + ["pkg/eventstore/schema.sql"] + [
    "pkg/eventstore/migrations/" + name for name in [
        "000002_messaging_delivery.up.sql", "000003_messaging_route_compatibility.up.sql",
        "000004_projection_checkpoint.up.sql"]]
digests = {}
for relative in artifacts:
    data = (ROOT / relative).read_bytes()
    assert data == git("show", SHA + ":" + relative)
    if relative not in source:
        assert data == git("show", SHA + "^:" + relative)
    digests[relative] = hashlib.sha256(data).hexdigest()

log = Path("/private/tmp/justixauto-qa-t927-full-race.log").read_text()
evidence_root = Path(re.search(r"Recoverable synthetic migration evidence: (\S+)", log)[1])
preimages = []
for sidecar in sorted(evidence_root.glob("*.sha256")):
    path = Path(str(sidecar)[:-7])
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    assert sidecar.read_text().strip() == digest
    preimages.append({"path": str(path), "sha256": digest})
assert len(preimages) == 36, len(preimages)
print(json.dumps({"reviewed_commit": SHA, "application_files_unchanged": True,
                  "artifact_sha256": digests, "recomputed_preimages": preimages}, indent=2))
