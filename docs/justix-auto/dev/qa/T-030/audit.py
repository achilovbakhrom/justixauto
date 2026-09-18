import base64,hashlib,io,json,os,pathlib,re,subprocess,tarfile
r=pathlib.Path.cwd();q=r/'docs/justix-auto/dev/qa/T-030'
target='6e96b488f8bbbde8f8f91f05cd7cd5c7d1b10e67';base='384c9ea59ea72fb45aeeab503c6a515ff80d7b20'
def sha(b):return hashlib.sha256(b).hexdigest()
def git(*args):return subprocess.check_output(['git',*args],cwd=r)
assert git('rev-parse','HEAD').decode().strip()==target
resultpath='docs/justix-auto/dev/results/T-030.md';text=(r/resultpath).read_text()
assert (r/resultpath).read_bytes()==git('show',target+':'+resultpath)
encoded=re.search(r'```base64\n(.*?)\n```',text,re.S).group(1)
raw=base64.b64decode(encoded);assert sha(raw)=='ccfbb7c37422642627be1922e3e25ba791d97ec423e1242541016bbc5a3144b8'
assert raw==pathlib.Path('/private/tmp/justixauto-t030-evidence.tar.gz').read_bytes()
entries={};inventory=[]
with tarfile.open(fileobj=io.BytesIO(raw),mode='r:gz') as archive:
    members=archive.getmembers();assert len(members)==6
    for m in members:
        assert m.isfile() and '/' not in m.name and m.name.startswith('justixauto-t030-'),m.name
        b=archive.extractfile(m).read();assert b==(pathlib.Path('/private/tmp')/m.name).read_bytes(),m.name
        expected=re.search(re.escape('`'+m.name+'`: `')+r'([0-9a-f]{64})`',text).group(1)
        assert sha(b)==expected,m.name
        entries[m.name]=b;inventory.append(dict(name=m.name,sha256=sha(b),bytes=len(b),final_newline=not b or b.endswith(b'\n')))
(q/'author-evidence.tar.gz.b64').write_text(encoded+'\n')
author_source=json.loads(entries['justixauto-t030-source-audit.json'])
sources=author_source['new_source_sha256'];assert len(sources)==5
for name,digest in sources.items():
    b=(r/name).read_bytes();assert sha(b)==digest and b==git('show',target+':'+name),name
changed=git('diff','--name-only',base,target).decode().splitlines();assert set(changed)==set(sources)|{resultpath}
base_identities=[]
for record in git('ls-tree','-r','-z',base).split(b'\0'):
    if not record:continue
    info,path=record.split(b'\t',1);mode,kind,oid=info.split();assert kind==b'blob'
    file=r/os.fsdecode(path);body=os.fsencode(os.readlink(file)) if mode==b'120000' else file.read_bytes()
    actual=hashlib.sha1(b'blob '+str(len(body)).encode()+b'\0'+body).hexdigest()
    assert actual==oid.decode(),str(file)
    base_identities.append(actual+'  '+os.fsdecode(path)+'\n')
assert len(base_identities)==3818
(q/'base-identities.sha1').write_text(''.join(base_identities))
for name in ['services/documents/port/unit_of_work.go','services/documents/port/dependencies.go']:
    assert re.findall(r'import "([^"]+)"',(r/name).read_text())==['context']
test=(r/'services/documents/adapter/postgres/unit_of_work_test.go').read_text()
assert '[]string{"identity", "inventory", "commerce", "retail", "financing", "insurance", "documents"}' in test
assert '[]string{"identity", "inventory", "commerce", "retail", "financing", "insurance"}' in test
authorrefs=set();freshrefs=set()
def addrefs(text,refs):
    for a,b in re.findall(r'validated[^\n]*?absent: ([0-9a-f]{64}) (justixauto-t030-[\w-]+)',text):refs.update([a,b])
    refs.update(re.findall(r'fixture absent after failed allocation: (justixauto-t030-[\w-]+)',text))
for name,b in entries.items():
    if name.endswith('.log'):addrefs(b.decode(),authorrefs)
for name in ['scope','independent']:
    assert (q/(name+'-exit.txt')).read_text().strip()=='0'
    addrefs((q/(name+'.log')).read_text(),freshrefs)
absence=[]
for ref in sorted(authorrefs|freshrefs):
    p=subprocess.run(['docker','inspect','--format','{{.Id}}',ref],capture_output=True,text=True,timeout=10)
    assert p.returncode!=0 and 'no such object' in (p.stdout+p.stderr).lower(),ref
    absence.append(dict(reference=ref,absent=True,exit=p.returncode))
pop=json.loads((q/'qa-populated-retained.json').read_text())
assert pop['ledger']==[{'version':1,'dirty':False}]
assert len(pop['events'])==len(pop['receipts'])==len(pop['operations'])==1
assert pop['events'][0]['owner_service']=='documents'
assert pop['events'][0]['company_id']==pop['receipts'][0]['company_id']==pop['operations'][0]['company_id']
out=dict(target=target,base=base,changed=changed,source_sha256=sources,base_preimages_checked=len(base_identities),base_identity_manifest_sha256=sha((q/'base-identities.sha1').read_bytes()),author_archive_sha256=sha(raw),author_entries=inventory,bootstrap_owners=7,foreign_database_denials=6,author_fixture_references=len(authorrefs),fresh_fixture_references=len(freshrefs),independent_absence_checks=absence,populated_retained_rows='one Documents event, receipt and operation; common synthetic company; clean v1',tracked_changes=git('diff','--name-only','HEAD').decode().splitlines())
assert not out['tracked_changes']
(q/'audit.json').write_text(json.dumps(out,indent=2)+'\n')
print(json.dumps({k:v for k,v in out.items() if k not in ['independent_absence_checks','author_entries']},indent=2))
