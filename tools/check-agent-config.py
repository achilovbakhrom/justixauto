#!/usr/bin/env python3
"""Read-only lint for the project agent roster and canonical pre-edit snapshots."""
import hashlib
import json
from pathlib import Path
import sys
try:
    import tomllib
except ModuleNotFoundError:
    sys.exit("Python >= 3.11 is required; this workspace provides python3.14")


def check(root: Path) -> list[str]:
    errors: list[str] = []
    expected = {
        "devops_orchestrator": ("gpt-6-astra", "xhigh", "workspace-write"),
        "task_slicer": ("gpt-6-astra", "high", "workspace-write"),
        "worker": ("gpt-5.6-terra", "medium", "workspace-write"),
        "reviewer": ("gpt-5.6-sol", "high", "read-only"),
        "qa": ("gpt-5.6-sol", "high", "workspace-write"),
        "qa_aggregator": ("gpt-5.6-sol", "high", "read-only"),
        "version_control": ("gpt-5.6-luna", "medium", "workspace-write"),
    }
    config = tomllib.loads((root / ".codex/config.toml").read_text())
    if config.get("model") != "gpt-6-astra":
        errors.append("primary coordinator model differs from approved routing")
    agents = config.get("agents", {})
    if agents.get("max_concurrent_threads_per_session") != 3:
        errors.append("delegate cap must be three")
    if agents.get("default_subagent_model") != "gpt-5.6-terra":
        errors.append("untyped delegates must not inherit the coordinator model")
    seen: set[str] = set()
    for file in sorted((root / ".codex/agents").glob("*.toml")):
        data = tomllib.loads(file.read_text())
        name = data.get("name")
        if name in seen or name not in expected or file.stem != name:
            errors.append(f"unexpected, duplicate or mismatched role: {file.name}")
            continue
        seen.add(name)
        actual = tuple(data.get(key) for key in
                       ("model", "model_reasoning_effort", "sandbox_mode"))
        if actual != expected[name]:
            errors.append(f"{name}: model/effort/sandbox differs from routing")
        if not data.get("description") or not data.get("developer_instructions"):
            errors.append(f"{name}: missing role contract")
    if seen != set(expected):
        errors.append(f"missing roles: {sorted(set(expected) - seen)}")
    for scope in ("internal", "web", "infra", "tools", "docs/justix-auto/dev"):
        if not (root / scope / "AGENTS.md").is_file():
            errors.append(f"missing scoped rules: {scope}")
    snapshots = root / "docs/justix-auto/state/backups/2026-09-19-agent-hubs"
    manifest = json.loads((snapshots / "manifest.json").read_text())
    for entry in manifest["files"]:
        target = (snapshots / entry["snapshot"]).resolve()
        if not target.is_relative_to(snapshots.resolve()):
            errors.append("snapshot escapes backup directory")
        elif hashlib.sha256(target.read_bytes()).hexdigest() != entry["sha256"]:
            errors.append(f"snapshot changed: {entry['source']}")
    return errors


if __name__ == "__main__":
    try:
        failures = check(Path(__file__).resolve().parent.parent)
    except (OSError, ValueError, KeyError) as exc:
        failures = [str(exc)]
    for failure in failures:
        print(f"FAIL: {failure}", file=sys.stderr)
    if failures:
        sys.exit(1)
    print("PASS: seven delegate roles, routing, scoped rules and backup hashes")
