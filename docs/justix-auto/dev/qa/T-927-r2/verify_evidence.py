"""Read-only exact-commit, original-QA and recoverable-preimage checks."""
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[5]
SHA = "3e1a5fe2d18f49f68738b954a3e0f0f489ad8c33"
ORIGINAL = "5d8edb0167e654bdb3db8cee26f028c50e8eb1fa"
IMPORT = "e648a64"
def git(*args):
    return subprocess.check_output(["git", "-C", str(ROOT), *args])
def digest(data):
    return hashlib.sha256(data).hexdigest()

assert git("rev-parse", "HEAD").decode().strip() == SHA
changed = git("diff", "--name-only", ORIGINAL, SHA).decode().splitlines()
assert set(changed) == {"docs/justix-auto/dev/results/T-927.md", "pkg/eventstore/quarantine_migration_test.go"}
artifacts = ["pkg/eventstore/schema.sql", "pkg/eventstore/quarantine_migration_test.go"] + [
    "pkg/eventstore/migrations/" + name for name in [
        "000002_messaging_delivery.up.sql", "000003_messaging_route_compatibility.up.sql",
        "000004_projection_checkpoint.up.sql", "000005_quarantine_evidence.up.sql"]]
digests = {}
for relative in artifacts:
    data = (ROOT / relative).read_bytes()
    assert data == git("show", SHA + ":" + relative)
    if relative.endswith(".sql"):
        assert data == git("show", ORIGINAL + ":" + relative)
    digests[relative] = digest(data)
preserved = {}
for relative in git("ls-tree", "-r", "--name-only", IMPORT, "docs/justix-auto/dev/qa/T-927.md", "docs/justix-auto/dev/qa/T-927/").decode().splitlines():
    data = (ROOT / relative).read_bytes()
    assert data == git("show", IMPORT + ":" + relative)
    preserved[relative] = digest(data)
assert len(preserved) == 10, len(preserved)

log = Path("/private/tmp/justixauto-qa-t927-r2-full-race.log").read_text()
assert "ok  \tjustixauto/pkg/eventstore\t" in log and not re.search(r"^FAIL", log, re.M)
roots = sorted(set(re.findall(r"Recoverable synthetic migration evidence: (\S+)", log)))
preimages = []
for directory in roots:
    for sidecar in sorted(Path(directory).glob("*.sha256")):
        path = Path(str(sidecar)[:-7])
        actual = digest(path.read_bytes())
        assert sidecar.read_text().strip() == actual
        preimages.append({"path": str(path), "sha256": actual})
assert len(preimages) == 36, len(preimages)
print(json.dumps({"reviewed_commit": SHA, "fix_paths": changed,
                  "artifact_sha256": digests, "preserved_original_qa": preserved,
                  "fixture_evidence_directories": roots,
                  "recomputed_preimages": preimages}, indent=2))
