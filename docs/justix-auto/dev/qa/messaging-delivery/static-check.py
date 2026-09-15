"""Run from the proposal worktree root; documentation checks only."""
import hashlib
import json
from pathlib import Path
import re
import subprocess

expected_head = "ba1723fdcbbfd9a0bbccdf6736e9346052d80fd7"
expected = {
    "docs/justix-auto/dev/results/messaging-delivery.md": "f6057fbfdd37388682e53546892c751461f6e6d33adefbe76f3342a88803c8bd",
    "docs/justix-auto/state/drafts/architect/messaging-delivery.md": "97e70c29183feded9e58f702ba2c995dc67b8ad4e6896c5f3e877b49db94c7d7",
}

def git(*args):
    return subprocess.check_output(["git", *args], text=True).strip()

assert git("rev-parse", "HEAD") == expected_head
assert sorted(git("diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD").splitlines()) == sorted(expected)
subprocess.run(["git", "diff", "HEAD^", "HEAD", "--check"], check=True)
links = 0
for name, digest in expected.items():
    path = Path(name)
    assert hashlib.sha256(path.read_bytes()).hexdigest() == digest, name
    for target in re.findall(r"\]\(([^)]+)\)", path.read_text()):
        if target.startswith(("http://", "https://", "#")):
            continue
        assert (path.parent / target.split("#")[0]).is_file(), (name, target)
        links += 1
assert links == 8
print(json.dumps({"head": expected_head, "scope": "PASS", "diff_check": "PASS", "relative_links": links, "sha256": expected}, indent=2))
