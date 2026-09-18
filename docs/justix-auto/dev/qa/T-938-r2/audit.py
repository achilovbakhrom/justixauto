import hashlib
import json
import pathlib
import subprocess

root = pathlib.Path.cwd()
evidence = root / "docs/justix-auto/dev/qa/T-938-r2"
target = "931ce802d736950e1d89f4b0cf231bebe6f6dc2f"
parent = "c45792e41fcbfa9a5316fb8b094e10821779a425"
base = "809d539a45ce4dae9a423e7f0d26ef17d478ad63"


def git(*arguments: str) -> bytes:
    return subprocess.check_output(["git", *arguments], cwd=root)


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


assert git("rev-parse", "HEAD").decode().strip() == target
changed_from_base = git("diff", "--name-only", base, target).decode().splitlines()
changed_in_fix = git("diff", "--name-only", parent, target).decode().splitlines()
expected = {
    "docs/justix-auto/dev/results/T-938.md",
    "tests/contracts/generation_reproducibility_test.go",
    "tools/generate-contracts.mjs",
}
assert set(changed_from_base) == expected
assert set(changed_in_fix) == expected

source = {}
for name in sorted(expected):
    content = (root / name).read_bytes()
    assert content == git("show", f"{target}:{name}"), name
    source[name] = sha256(content)

assert source["tools/generate-contracts.mjs"] == "de72fda38c34401fcbedd9c88bdd86d5cdc21598ced15751aa38eab4614691aa"
assert source["tests/contracts/generation_reproducibility_test.go"] == "0012be760605fa88304423488aa39a4720ae92e3cfe65c67dd27ff9e86085e5f"

unchanged = {}
for name in [
    "go.mod",
    "go.sum",
    "package.json",
    "package-lock.json",
    "tools/contracts-generator.config.json",
]:
    content = (root / name).read_bytes()
    assert content == git("show", f"{base}:{name}") == git("show", f"{target}:{name}"), name
    unchanged[name] = sha256(content)

original_checksums = {}
for line in (root / "docs/justix-auto/dev/qa/T-938/SHA256SUMS").read_text().splitlines():
    digest, name = line.split("  ", 1)
    content = (root / name).read_bytes()
    assert sha256(content) == digest, name
    original_checksums[name] = digest

result = {
    "target": target,
    "parent": parent,
    "base": base,
    "changed_from_base": changed_from_base,
    "changed_in_fix": changed_in_fix,
    "source": source,
    "unchanged": unchanged,
    "original_qa_artifacts_verified": len(original_checksums),
    "tracked_worktree_changes": git("diff", "--name-only", "HEAD").decode().splitlines(),
}
assert not result["tracked_worktree_changes"]
(evidence / "audit.json").write_text(json.dumps(result, indent=2) + "\n")
print(json.dumps(result, indent=2))
