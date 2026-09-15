"""Read-only exact promotion audit; emits all findings before returning failure."""
from pathlib import Path
import functools
import gzip
import hashlib
import json
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[5]
PROMOTION = "61613b8b45033fce1ed432e7fe6acbb60b90ba0d"
TARGET = sys.argv[1] if len(sys.argv) > 1 else PROMOTION
PROPOSAL = "631388169f3f24c97b9237bcf933de79315b3651"
HASH = "7130ed6d2cf47c609223be2ea6d9a7d6500e515a028c918896224b1b90d7766a"
errors = []
def check(ok, message):
    if not ok:
        errors.append(message)
def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT)
def read(path):
    return (ROOT / path).read_text()
check(git("rev-parse", "HEAD").decode().strip() == TARGET, "checkout SHA differs")
parent = git("rev-parse", PROMOTION + "^").decode().strip()
contract = "docs/justix-auto/state/drafts/contracts/messaging-route-correction.md"
artifact = (ROOT / contract).read_bytes()
check(hashlib.sha256(artifact).hexdigest() == HASH, "canonical contract hash")
check(artifact == git("show", PROPOSAL + ":docs/justix-auto/state/drafts/architect/messaging-route-correction.md"), "canonical proposal byte identity")
approval = read("docs/justix-auto/state/approvals/messaging-route-correction.md")
check(PROPOSAL in approval and HASH in approval and "QA GREEN" in approval, "approval provenance")
report = read("docs/justix-auto/dev/qa/messaging-route-correction.md")
check(PROPOSAL in report and "GREEN" in report and "proposal readiness only" in report, "review provenance/boundary")
old = json.loads(git("show", parent + ":docs/justix-auto/dev/task-index.json"))
new = json.loads(read("docs/justix-auto/dev/task-index.json"))
before = {t["id"]: t for t in old["tasks"]}
after = {t["id"]: t for t in new["tasks"]}
direct = {"T-919", "T-921", "T-924", "T-022"} | {f"T-{n}" for n in range(580, 587)}
check(set(after) - set(before) == {"T-925"} and len(after) == 925, "task set/count")
for key, task in before.items():
    expected = dict(task)
    if key in direct:
        expected["depends_on"] = task["depends_on"] + ["T-925"]
    check(after.get(key) == expected, "unexpected baseline task change " + key)
newtask = after["T-925"]
check(newtask["depends_on"] == ["T-917", "T-011", "T-007", "T-006"], "T925 dependencies")
check(newtask["status"] == "todo" and newtask["effort_hours"] == 4 and newtask["worktree"] is None and newtask["base_sha"] is None, "T925 assignment/effort state")
leaves = ["pkg/eventstore/migrations/000003_messaging_route_compatibility.up.sql", "pkg/eventstore/messaging_route_migration_test.go"]
check(newtask["files_owned"] == leaves, "T925 exact ownership")
check(all(p not in t["files_owned"] for p in leaves for t in before.values()), "new ownership collision")
meta = new["metadata"]
check(meta["task_count"] == len(after) == 925 and meta["effort_hours"] == sum(t["effort_hours"] for t in after.values()) == 3066, "metadata count/effort")
for key, value in old["metadata"].items():
    if key not in {"task_count", "effort_hours", "graph_metrics_scope"}:
        check(meta[key] == value, "baseline metadata drift " + key)
check("916-task baseline" in meta["graph_metrics_scope"] and "without claiming recomputed" in meta["graph_metrics_scope"], "historical graph scope")
check(set(meta["route_compatibility_correction"]["direct_targets"]) == direct, "direct target metadata")
visiting = set()
@functools.lru_cache(None)
def upstream(key):
    if key in visiting:
        raise AssertionError("cycle at " + key)
    visiting.add(key)
    result = set(after[key]["depends_on"])
    for dep in after[key]["depends_on"]:
        result.update(upstream(dep))
    visiting.remove(key)
    return frozenset(result)
for key in after:
    upstream(key)
check({key for key, task in after.items() if "T-925" in task["depends_on"]} == direct, "exact direct gates")
for key in ["T-012", "T-021", "T-023", "T-923", "T-591"]:
    check("T-925" in upstream(key), "missing transitive gate " + key)
for key in ["T-918", "T-920", "T-922"]:
    check("T-925" not in upstream(key), "independent task blocked " + key)
board = read("docs/justix-auto/dev/task-board.md")
rows = [line.split("|")[1:-1] for line in board.splitlines() if line.startswith("| [T-")]
check(len(rows) == 925, "board row count")
for cols in rows:
    key = re.search(r"T-\d+", cols[0])[0]
    task = after[key]
    check(cols[5].strip() == task["status"], "board status " + key)
    deps = re.findall(r"T-\d+", cols[6])
    check(deps == task["depends_on"], "board dependencies " + key)
    text = read("docs/justix-auto/dev/tasks/" + key + ".md")
    check(re.search(r"^- Status: (.+)$", text, re.M)[1] == task["status"], "task status " + key)
    check(re.findall(r"T-\d+", re.search(r"^- Depends on: (.+)$", text, re.M)[1]) == task["depends_on"], "task dependencies " + key)
for key in direct:
    text = read("docs/justix-auto/dev/tasks/" + key + ".md")
    check("revision-3/route-format-1 marker" in text and "fullOwnerQualifiedEventType" in text, "downstream acceptance " + key)
coverage = read("docs/justix-auto/dev/backlog-coverage.md")
for ac in ["B-01.AC1", "B-01.AC2"]:
    line = next(line for line in coverage.splitlines() if line.startswith("| " + ac + " |"))
    check("T-925" in line and line.endswith("T-591 |"), "T925 coverage " + ac)
    if ac.endswith("AC2"):
        check(all(f"T-{n}" in line for n in range(917, 925)), "prior amendment coverage")
backup = ROOT / "docs/justix-auto/state/backups/2026-09-15-route-promotion"
manifest = json.loads((backup / "manifest.json").read_text())
check(len(manifest) == 17, "snapshot count")
for entry in manifest:
    data = gzip.decompress((backup / entry["snapshot"]).read_bytes())
    check(len(data) == entry["bytes"] and hashlib.sha256(data).hexdigest() == entry["sha256"], "snapshot length/hash " + entry["path"])
    check(data == git("show", parent + ":" + entry["path"]), "snapshot differs from parent " + entry["path"])
modified = set(git("diff", "--name-only", "--diff-filter=M", parent, TARGET).decode().splitlines())
check(modified == {e["path"] for e in manifest}, "modified canonical preimage coverage")
changed = git("diff", "--name-only", parent, TARGET).decode().splitlines()
check(all(p.startswith("docs/justix-auto/") for p in changed), "application source changed")
links = 0
for path in changed:
    if not path.endswith(".md"):
        continue
    for link in re.findall(r"\]\(([^)]+)\)", read(path)):
        if "://" in link or link.startswith("#"):
            continue
        links += 1
        check((ROOT / path).parent.joinpath(link.split("#")[0]).resolve().exists(), "missing link " + path + " -> " + link)
print(json.dumps({"target": TARGET, "baseline_parent": parent, "tasks": len(after), "effort_hours": meta["effort_hours"], "direct_gates": len(direct), "snapshots": len(manifest), "local_links_checked": links, "errors": errors, "boundary": "Documentation promotion only; no runtime certification"}, indent=2))
sys.exit(bool(errors))
