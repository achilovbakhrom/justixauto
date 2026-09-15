import hashlib, json, pathlib, re, subprocess

root = pathlib.Path(__file__).resolve().parents[5]
assert root.name == 'T-027', root
target = 'db5c28c6ea7fe709d874f4e2a083d891a965877b'
base = 'a2c61e8fadd0b8a5f4d7563aa67c1383c30b134e'
def git(*args):
    return subprocess.check_output(['git', '-C', str(root), *args])
def digest(data): return hashlib.sha256(data).hexdigest()
assert git('rev-parse', 'HEAD').decode().strip() == target
result_path = 'docs/justix-auto/dev/results/T-027.md'
result = (root / result_path).read_text()
sources = re.findall(r'\| `(services/retail/[^`]+)` \| `([0-9a-f]{64})` \|', result)
assert len(sources) == 5
checks = {}
for path, expected in sources:
    raw = (root / path).read_bytes()
    assert raw == git('show', target + ':' + path)
    assert digest(raw) == expected, path
    checks[path] = expected
changed = set(git('diff', '--name-only', base, target).decode().splitlines())
assert changed == {p for p, _ in sources} | {result_path}, changed
assert (root/result_path).read_bytes() == git('show', target+':'+result_path)
logs = re.findall(r'### (justixauto-t027-[^\n]+)\n\nSHA-256 `([0-9a-f]{64})`\n\n```text\n(.*?)```',result,re.S)
assert len(logs) == 4
restored = 0
for name, expected, content in logs:
    if name.startswith('justixauto-t027-live-'):
        content, n = re.subn(r'(?m)^\n(?= {16}(?:Usage:  docker create|Run \'docker create --help\'))',' '*16+'\n',content)
        assert n == 2, (name,n)
        restored += n
    assert digest(content.encode()) == expected, name
    checks['reconstructed/'+name] = expected
assert restored == 4
for path in ['pkg/eventstore/schema.sql','pkg/eventstore/transaction.go','pkg/eventstore/replay.go','pkg/eventstore/upcast.go','pkg/commands/ledger.go','pkg/commands/operation.go','pkg/eventstore/migrations/000002_messaging_delivery.up.sql']:
    raw=(root/path).read_bytes()
    assert raw == git('show',base+':'+path) == git('show',target+':'+path)
    checks[path]=digest(raw)
imports=git('grep','-n','import',target,'--','services/retail/port').decode()
assert imports.count('import "context"') == 2
assert len(imports.splitlines()) == 2
print(json.dumps({'reviewed_commit':target,'source_hashes_and_reconstructed_logs':checks,'exact_changed_files':sorted(changed),'restored_log_lines':restored,'port_imports':imports.splitlines(),'result':'PASS'},indent=2))
