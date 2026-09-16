import json, pathlib, re, subprocess
directory = pathlib.Path(__file__).resolve().parent
text = (directory / 'live-and-independent.log').read_text()
records = set(re.findall(r'validated fixture removed and absent: ([0-9a-f]{64}) (justixauto-t027-[0-9a-f-]{36})', text))
names = set(re.findall(r'fixture absent after failed allocation: (justixauto-t027-[0-9a-f-]{36})', text))
assert len(records) == 5, records
names -= {name for _, name in records}
assert len(names) == 1, names
results = []
for identifier in sorted({x for pair in records for x in pair} | names):
    proc = subprocess.run(['docker', 'inspect', '--format', '{{.Id}}', identifier], text=True, capture_output=True, timeout=10)
    assert proc.returncode != 0 and 'no such object' in proc.stderr.lower(), (identifier,proc.returncode,proc.stdout,proc.stderr)
    results.append({'identifier':identifier,'exit_code':proc.returncode,'stderr':proc.stderr.strip(),'absent':True})
print(json.dumps({'allocated_fixture_count':len(records),'refused_allocation_count':len(names),'checks':results,'result':'PASS'},indent=2))
