"""Exact-commit proposal diagnostics; does not execute or certify a migration."""
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[5]
SHA = "631388169f3f24c97b9237bcf933de79315b3651"
assert subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip() == SHA
hashes = {
    "pkg/eventstore/migrations/000002_messaging_delivery.up.sql": "1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2",
    "pkg/outbox/insert.go": "e9e6d7e37f7adc0e6f7e15539a06f8215f15cb4da720703ab0ead14996c98a03",
    "docs/justix-auto/state/drafts/contracts/messaging-delivery.md": "97e70c29183feded9e58f702ba2c995dc67b8ad4e6896c5f3e877b49db94c7d7",
}
for path, expected in hashes.items():
    assert hashlib.sha256((ROOT / path).read_bytes()).hexdigest() == expected, path
changes = subprocess.check_output(["git", "diff-tree", "--no-commit-id", "--name-only", "-r", SHA], cwd=ROOT, text=True).splitlines()
assert sorted(changes) == sorted([
    "docs/justix-auto/dev/results/messaging-route-correction.md",
    "docs/justix-auto/state/drafts/architect/messaging-route-correction.md",
])
producer = (ROOT / "pkg/outbox/insert.go").read_text()
sql = (ROOT / "pkg/eventstore/migrations/000002_messaging_delivery.up.sql").read_text()
assert 'string(e.Owner()) + "." + string(target) + "." + e.EventType()' in producer
assert 'len(route(envelope, target)) > 255' in producer
assert "m.source_owner||substring(NEW.routing_key FROM length(m.source_owner)+length(NEW.destination)+2)" in sql
assert "m.source_owner||substring(m.routing_key FROM length(m.source_owner)+length(split_part(m.routing_key,'.',2))+2)" in sql
owners = "identity inventory commerce retail financing insurance documents".split()
topology = json.loads((ROOT / "infra/local/rabbitmq/definitions.json").read_text())
topic = {p["user"]: p for p in topology["topic_permissions"] if p["exchange"] == "justix.integration.v1"}

def valid(source, target, route):
    if source not in owners or target not in owners or len(route.encode()) > 255:
        return False
    return re.fullmatch(re.escape(source + "." + target + "." + source) + r"(\.[a-z][a-z0-9-]*)+\.v[1-9][0-9]*", route) is not None

for source in owners:
    for target in owners:
        event = source + ".fixture.changed.v1"
        route = source + "." + target + "." + event
        # PostgreSQL substring offsets are one-based; these are ASCII inputs.
        old = source + route[len(source) + len(target) + 1:]
        corrected = route[len(source) + len(target) + 2:]
        assert old == source + "." + event and old != event
        assert corrected == event and valid(source, target, route)
        assert re.fullmatch(topic["svc_" + source]["write"], route)
        assert re.fullmatch(topic["svc_" + target]["read"], route)
        assert any(b["routing_key"] == "*." + target + ".#" for b in topology["bindings"])
        assert not valid(source, target, source + "." + target + ".fixture.changed.v1")
print("PASS 49 source/target pairs: established full route, old duplicated lookup, corrected exact suffix, current topic ACL/binding compatibility, shortened rejection")
bad = [
    "inventory.retail.commerce.fixture.v1", "inventory.commerce.inventory.fixture.v1",
    "inventory.retail.inventory.v1", "inventory.retail.inventory.fixture.v0",
    "inventory.retail.inventory.fixture.v01", "inventory.retail.inventory.fixture.V1",
    "inventory.retail.inventory.*.v1", "inventory.retail.inventory.fixture.#",
    "inventory.retail.inventory..fixture.v1", "inventory.retail.inventory.Fixture.v1",
    "inventory.retail.inventory." + "a" * 240 + ".v1",
]
assert all(not valid("inventory", "retail", route) for route in bad)
assert valid("inventory", "retail", "inventory.retail.inventory.inventory.fixture.v1")
assert not valid("unknown", "retail", "unknown.retail.unknown.fixture.v1")
assert not valid("inventory", "unknown", "inventory.unknown.inventory.fixture.v1")
print("PASS 13 malformed/unknown cases and legitimate repeated owner word in nested event name")

tasks = {t["id"]: t for t in json.loads((ROOT / "docs/justix-auto/dev/task-index.json").read_text())["tasks"]}
assert "T-925" not in tasks
owned = ["pkg/eventstore/migrations/000003_messaging_route_compatibility.up.sql", "pkg/eventstore/messaging_route_migration_test.go"]
assert all(path not in t["files_owned"] for path in owned for t in tasks.values())
graph = {key: set(t["depends_on"]) for key, t in tasks.items()}
graph["T-925"] = {"T-917", "T-011", "T-007", "T-006"}
direct = ["T-919", "T-921", "T-924", "T-022"] + [f"T-{n}" for n in range(580, 587)]
for key in direct:
    graph[key].add("T-925")
done, visiting = set(), set()
def visit(key):
    assert key not in visiting, key
    if key in done:
        return
    visiting.add(key)
    for dep in graph[key]:
        visit(dep)
    visiting.remove(key)
    done.add(key)
for key in graph:
    visit(key)
def ancestors(key):
    result = set(graph[key])
    for dep in graph[key]:
        result.update(ancestors(dep))
    return result
for key in ["T-012", "T-021", "T-023", "T-923", "T-591"] + direct:
    assert "T-925" in ancestors(key), key
for key in ["T-918", "T-920", "T-922"]:
    assert "T-925" not in ancestors(key), key
print("PASS proposed 925-node graph: acyclic, both leaves unowned, all 11 direct consumers and 5 transitive targets gated; 3 independent tasks remain eligible")
print("PASS exact proposal commit and all 3 pinned source hashes; no source changes in proposal commit")
print("LIMIT: arithmetic/regex and dependency diagnostics only; no PostgreSQL migration, AMQP delivery, payload validation, or business authorization proof")
