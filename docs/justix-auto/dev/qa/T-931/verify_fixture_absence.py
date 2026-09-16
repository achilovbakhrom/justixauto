import json,pathlib,subprocess
directory=pathlib.Path(__file__).resolve().parent
data=json.loads((directory/'evidence-check.json').read_text())
checks=[]
for ref in sorted(set(data['author_fixture_refs']+data['qa_fixture_refs'])):
    p=subprocess.run(['docker','inspect','--format','{{.Id}}',ref],capture_output=True,text=True,timeout=10)
    assert p.returncode!=0 and 'no such object' in p.stderr.lower(),(ref,p.returncode,p.stdout,p.stderr)
    checks.append({'reference':ref,'absent':True,'exit_code':p.returncode})
print(json.dumps({'result':'PASS','author_references':len(data['author_fixture_refs']),'qa_references':len(data['qa_fixture_refs']),'checks':checks},indent=2))
