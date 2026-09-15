"""Independent read-only canonical promotion checks. Run at worktree root."""
import gzip
import hashlib
import json
from pathlib import Path
import re
import subprocess

HEAD = "3168a653bde21a4432f1fcf62c01c207c138bbe7"
PROPOSAL = "ba1723fdcbbfd9a0bbccdf6736e9346052d80fd7"
ROOT = Path("docs/justix-auto")

def git(*args):
    return subprocess.check_output(["git", *args])

assert git("rev-parse", "HEAD").decode().strip() == HEAD
source = git("show", PROPOSAL + ":docs/justix-auto/state/drafts/architect/messaging-delivery.md")
assert (ROOT / "state/drafts/contracts/messaging-delivery.md").read_bytes() == source
assert (ROOT / "state/drafts/architect/messaging-delivery.md").read_bytes() == source
subprocess.run(["git", "diff", "HEAD^", "HEAD", "--check"], check=True)
changes = [line.split("\t") for line in git("diff", "--name-status", "HEAD^", "HEAD").decode().splitlines()]
assert all(status in ("A", "M") and path.startswith("docs/justix-auto/") for status, path in changes)

# Check every snapshot hash/length and recover each changed canonical preimage.
snapshots = {}
snapshot_count = 0
for run in ("2026-09-15-t036-start", "2026-09-15-messaging-promotion"):
    directory = ROOT / "state/backups" / run
    for entry in json.loads((directory / "manifest.json").read_text()):
        data = gzip.decompress((directory / entry["snapshot"]).read_bytes())
        assert len(data) == entry["bytes"], entry["path"]
        assert hashlib.sha256(data).hexdigest() == entry["sha256"], entry["path"]
        snapshots.setdefault(entry["path"], []).append(data)
        snapshot_count += 1
modified = [path for status, path in changes if status == "M"]
assert set(modified) == set(snapshots)
for path in modified:
    assert snapshots[path][0] == git("show", "HEAD^:" + path), path

index_path = ROOT / "dev/task-index.json"
new = json.loads(index_path.read_text())
old = json.loads(git("show", "HEAD^:" + str(index_path)))
tasks = {task["id"]: task for task in new["tasks"]}
old_tasks = {task["id"]: task for task in old["tasks"]}
assert len(tasks) == len(new["tasks"]) == new["metadata"]["task_count"] == 924
assert len({task["key"] for task in tasks.values()}) == 924
assert set(tasks) - set(old_tasks) == {f"T-{n}" for n in range(917, 925)}
assert new["metadata"]["effort_hours"] == sum(task["effort_hours"] for task in tasks.values()) == 3062
assert "916-task baseline" in new["metadata"]["graph_metrics_scope"]
added_edges = {"T-012": ["T-919", "T-924"], "T-021": ["T-923"], "T-022": ["T-917", "T-924"]}
added_edges.update({f"T-{n}": ["T-919", "T-923", "T-924"] for n in range(580, 587)})
for task_id, before in old_tasks.items():
    after = tasks[task_id]
    changed_fields = {key for key in set(before) | set(after) if before.get(key) != after.get(key)}
    if task_id in added_edges:
        assert changed_fields == {"depends_on"}, (task_id, changed_fields)
        assert after["depends_on"] == before["depends_on"] + added_edges[task_id]
    elif task_id == "T-036":
        assert changed_fields <= {"status", "worktree", "base_sha", "agent", "owner_agent", "execution_note"}, changed_fields
        assert after["status"] == "in-progress"
    else:
        assert not changed_fields, (task_id, changed_fields)

# Exhaustive DAG/unknown-edge checks, independent of the focused helper.
closure = {}
def ancestors(task_id, visiting=frozenset()):
    assert task_id not in visiting, ("cycle", task_id)
    if task_id not in closure:
        deps = tasks[task_id]["depends_on"]
        assert len(deps) == len(set(deps)), task_id
        result = set(deps)
        for dep in deps:
            assert dep in tasks, (task_id, dep)
            result.update(ancestors(dep, visiting | {task_id}))
        closure[task_id] = result
    return closure[task_id]
for task_id in tasks:
    ancestors(task_id)
assert "T-922" in tasks["T-923"]["depends_on"]
assert set(f"T-{n}" for n in range(917, 925)) <= ancestors("T-591")
for n in range(580, 587):
    assert set(f"T-{i}" for i in range(917, 925)) <= ancestors(f"T-{n}")

board = (ROOT / "dev/task-board.md").read_text()
for task_id in set(added_edges) | {f"T-{n}" for n in range(917, 925)}:
    task = tasks[task_id]
    text = (ROOT / f"dev/tasks/{task_id}.md").read_text()
    deps = ", ".join(task["depends_on"])
    assert f"- Depends on: {deps}\n" in text, task_id
    row = next(line for line in board.splitlines() if line.startswith(f"| [{task_id}]"))
    assert f"| {deps} |" in row, task_id
    if int(task_id[2:]) < 917:
        continue
    assert task["status"] == "todo" and task["worktree"] is None and task["base_sha"] is None
    assert task["gates"] == ["ENV-GIT"]
    section = text.split("## Exact file ownership\n", 1)[1].split("Only the named leaves", 1)[0]
    assert re.findall(r"^- `([^`]+)`$", section, re.M) == task["files_owned"], task_id
    for other in tasks.values():
        if other["id"] == task_id:
            continue
        common = set(task["files_owned"]) & set(other["files_owned"])
        assert not common or other["id"] in ancestors(task_id) or task_id in ancestors(other["id"]), (task_id, other["id"], common)

# Intermediate snapshots contain only the separately recorded T-036 allocation.
idx = str(index_path)
mid = json.loads(snapshots[idx][1])
before = json.loads(snapshots[idx][0])
assert mid["metadata"] == before["metadata"]
mid_tasks = {task["id"]: task for task in mid["tasks"]}
assert {task_id for task_id in old_tasks if old_tasks[task_id] != mid_tasks[task_id]} == {"T-036"}
assert mid_tasks["T-036"] == tasks["T-036"]
assert "924 tasks" in board and "original reviewed 916-task baseline" in board
assert "19/924" in (ROOT / "dev/dev-state.md").read_text()
print(json.dumps({"head": HEAD, "contract_sha256": hashlib.sha256(source).hexdigest(), "tasks": len(tasks), "snapshot_entries": snapshot_count, "canonical_preimages": len(modified), "dag": "PASS", "original_task_preservation": "PASS", "new_task_ownership_and_gates": "PASS", "all_eight_reach_owner_wiring_and_B01_acceptance": "PASS"}, indent=2))
