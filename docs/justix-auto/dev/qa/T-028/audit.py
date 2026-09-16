import base64,difflib,gzip,hashlib,json,pathlib,re,subprocess
root=pathlib.Path.cwd();qa=root/'docs/justix-auto/dev/qa/T-028'
target='0f4736a21db0d56fa17ca83934a43596a5352a35';base='6f30bbbf9144fc3d03a49fbac57bb9dd9a43afdc'
def sha(b):return hashlib.sha256(b).hexdigest()
def git(*args):return subprocess.check_output(['git',*args],cwd=root)
assert git('rev-parse','HEAD').decode().strip()==target
resultpath='docs/justix-auto/dev/results/T-028.md';text=(root/resultpath).read_text()
assert (root/resultpath).read_bytes()==git('show',target+':'+resultpath)
identities=json.loads(re.search(r'## Source and unchanged input identities.*?```json\n(.*?)\n```',text,re.S).group(1))
for group,items in identities.items():
    for name,digest in items.items():
        b=(root/name).read_bytes();assert sha(b)==digest and b==git('show',target+':'+name),name
        if group=='unchanged_inputs':assert b==git('show',base+':'+name),name
changed=git('diff','--name-only',base,target).decode().splitlines()
assert set(changed)==set(identities['source'])|{resultpath},changed
for name in ['services/financing/port/unit_of_work.go','services/financing/port/dependencies.go']:
    s=(root/name).read_text();assert re.findall(r'import "([^"]+)"',s)==['context'],name

archive64=re.search(r'## Recoverable raw evidence.*?```text\n(.*?)\n```',text,re.S).group(1)
archive=gzip.decompress(base64.b64decode(archive64));assert sha(archive)=='4d22fba18da379bbc28bbb3a27f7f3d697f004179d021c508aec1926c5ab3f2f'
entries=json.loads(archive);assert len(entries)==6
inventory=[];authorrefs=set()
for name,item in entries.items():
    b=base64.b64decode(item['bytes_base64']);assert sha(b)==item['sha256'],name
    actual=pathlib.Path('/private/tmp')/name
    assert actual.read_bytes()==b,name
    inventory.append(dict(name=name,sha256=sha(b),bytes=len(b)))
    for a,c in re.findall(r'validated[^\n]*?absent: ([0-9a-f]{64}) (justixauto-t028-[\w-]+)',b.decode()):authorrefs.update([a,c])
    authorrefs.update(re.findall(r'fixture absent after failed allocation: (justixauto-t028-[\w-]+)',b.decode()))
assert len(authorrefs)==14,authorrefs
(qa/'author-archive.json.gz.b64').write_text(archive64+'\n')

diff=[]
for name in ['port/unit_of_work.go','port/dependencies.go','migrations/0001_mechanics.up.sql','adapter/postgres/unit_of_work.go','adapter/postgres/unit_of_work_test.go']:
    retail=(root/('services/retail/'+name)).read_text()
    normalized=retail.replace('Retail','Financing').replace('retail','financing').replace('T-027','T-028').replace('T027','T028').replace('t027','t028')
    financing=(root/('services/financing/'+name)).read_text()
    diff.extend(difflib.unified_diff(normalized.splitlines(True),financing.splitlines(True),fromfile='adapted Retail '+name,tofile='Financing '+name))
(qa/'retail-pattern.diff').write_text(''.join(diff))
test=(root/'services/financing/adapter/postgres/unit_of_work_test.go').read_text()
assert '[]string{"identity", "inventory", "commerce", "retail", "financing", "insurance", "documents"}' in test
assert '[]string{"identity", "inventory", "commerce", "retail", "insurance", "documents"}' in test
freshrefs=set()
for name in ['scope.log','independent.log','migration.log']:
    s=(qa/name).read_text()
    for a,b in re.findall(r'validated[^\n]*?absent: ([0-9a-f]{64}) (justixauto-t028-[\w-]+)',s):freshrefs.update([a,b])
    freshrefs.update(re.findall(r'fixture absent after failed allocation: (justixauto-t028-[\w-]+)',s))
    assert (qa/(name[:-4]+'-exit.txt')).read_text().strip()=='0'
absence=[]
for ref in sorted(authorrefs|freshrefs):
    r=subprocess.run(['docker','inspect','--format','{{.Id}}',ref],capture_output=True,text=True,timeout=10)
    assert r.returncode!=0 and 'no such object' in (r.stdout+r.stderr).lower(),ref
    absence.append(dict(reference=ref,exit=r.returncode,absent=True))
populated=json.loads((qa/'qa-populated-retained.json').read_text())
assert populated['ledger']==[{'dirty':False,'version':1}]
assert len(populated['events'])==len(populated['receipts'])==len(populated['operations'])==1
assert populated['events'][0]['owner_service']=='financing'
assert populated['events'][0]['company_id']==populated['receipts'][0]['company_id']==populated['operations'][0]['company_id']
report=dict(target=target,base=base,changed=changed,source_and_inputs=identities,verified_author_entries=inventory,archive_sha256=sha(archive),typed_port_imports='context only',owner_bootstrap_count=7,foreign_database_denial_count=6,author_fixture_reference_count=len(authorrefs),fresh_fixture_reference_count=len(freshrefs),independent_absence_checks=absence,populated_retained_effects='one financing event, scoped receipt, operation; common company; clean v1',tracked_changes=git('diff','--name-only','HEAD').decode().splitlines())
assert not report['tracked_changes']
(qa/'audit.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k not in ['independent_absence_checks','source_and_inputs','verified_author_entries']},indent=2))
