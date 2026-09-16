import json, pathlib, subprocess
directory=pathlib.Path(__file__).resolve().parent
evidence=json.loads((directory/'evidence-check.json').read_text())
fixtures=evidence['retained_runs']['developer_final']['fixture_ids_reported_in_log']
assert len(fixtures)==6
checks=[]
for identity,name in fixtures:
    for ref in (identity,name):
        p=subprocess.run(['docker','inspect','--format','{{.Id}}',ref],capture_output=True,text=True,timeout=10)
        assert p.returncode!=0 and 'no such object' in (p.stdout+p.stderr).lower(),(ref,p.returncode,p.stdout,p.stderr)
        checks.append({'identifier':ref,'exit_code':p.returncode,'absent':True})
print(json.dumps({'result':'PASS','mode':'read-only inspection; no fixture created or removed by r3 QA','checks':checks},indent=2))
