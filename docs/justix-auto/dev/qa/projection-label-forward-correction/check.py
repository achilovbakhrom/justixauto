#!/usr/bin/env python3
"""Exact proposal scope and immutable-source checks."""
import hashlib
import json
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parents[5]
def git(*args):
    return subprocess.check_output(['git', '-C', str(root), *args], text=True).strip()
assert git('rev-parse', 'HEAD') == 'f3fc029de3da3dbadb0cab980ef6bd33c86723b3'
proposal = 'docs/justix-auto/state/drafts/architect/projection-label-forward-correction.md'
assert git('diff', 'HEAD^', 'HEAD', '--name-only') == proposal
assert not git('diff', '--name-only')
expected = {
    proposal: '704d2c65eedbe1e98f36bc0270d6ce366ad7cd38cccc2fe94b25e2a3ffa9f196',
    'pkg/eventstore/migrations/000002_messaging_delivery.up.sql': '1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2',
    'pkg/eventstore/migrations/000003_messaging_route_compatibility.up.sql': 'c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec',
    'pkg/eventstore/migrations/000004_projection_checkpoint.up.sql': '41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c',
}
for path, digest in expected.items():
    assert hashlib.sha256((root / path).read_bytes()).hexdigest() == digest, path
    print('PASS exact SHA-256', path, digest)
tasks = {t['id']: t for t in json.loads((root / 'docs/justix-auto/dev/task-index.json').read_text())['tasks']}
assert len(tasks) == 934
assert tasks['T-928']['files_owned'] == ['pkg/eventstore/migrations/000006_projection_generation.up.sql', 'pkg/eventstore/projection_generation_migration_test.go']
assert tasks['T-928']['depends_on'] == ['T-927']
assert 'T-928' in tasks['T-016']['depends_on']
assert 'T-928' in tasks['T-934']['depends_on']
assert all(tasks[t]['status'] == 'todo' for t in ('T-928', 'T-016', 'T-934'))
assert not (root / 'pkg/eventstore/migrations/000006_projection_generation.up.sql').exists()
print('PASS future T-928 exclusive leaves, existing T-016/T-934 dependency paths, 934-task count and unassigned implementation status')
finding = (root / 'docs/justix-auto/dev/projection-label-readiness.md').read_text()
assert 'overstated' in finding and 'do not establish' in finding
old = (root / 'pkg/eventstore/migrations/000004_projection_checkpoint.up.sql').read_text()
assert 'generation text NOT NULL CHECK(length(btrim(generation))>0)' in old
assert 'a.consumer_kind,a.generation' in old and 'NEW.consumer_kind,NEW.generation' in old
assert 'IS DISTINCT FROM' in old
assert 'GRANT SELECT ON eventstore.projection_checkpoint_compatibility' in old
print('PASS historical coverage correction retained; installed CHECK, admission equality and marker privilege source inspected')
print('GREEN static proposal scope. No application source changed; canonical adoption and future implementation remain separate.')
