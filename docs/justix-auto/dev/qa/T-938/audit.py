import base64,gzip,hashlib,json,pathlib,subprocess
r=pathlib.Path.cwd();q=r/'docs/justix-auto/dev/qa/T-938'
target='c45792e41fcbfa9a5316fb8b094e10821779a425';base='809d539a45ce4dae9a423e7f0d26ef17d478ad63'
def sha(b):return hashlib.sha256(b).hexdigest()
def git(*args):return subprocess.check_output(['git',*args],cwd=r)
assert git('rev-parse','HEAD').decode().strip()==target
changed=git('diff','--name-only',base,target).decode().splitlines()
assert set(changed)=={'tools/generate-contracts.mjs','tests/contracts/generation_reproducibility_test.go','docs/justix-auto/dev/results/T-938.md'}
source={}
for name in changed:
    b=(r/name).read_bytes();assert b==git('show',target+':'+name),name;source[name]=sha(b)
assert source['tools/generate-contracts.mjs']=='dbafd6cf17d2acc5ebde8e67ba4feafc2d28bd0878dc9c1811fa696370521226'
assert source['tests/contracts/generation_reproducibility_test.go']=='3fe00798f456fdf78ff4b72bc243dc1c4d90a86f6889b07f9c7aae69aa30de03'
unchanged={}
for name in ['go.mod','go.sum','package.json','package-lock.json','tools/contracts-generator.config.json']:
    b=(r/name).read_bytes();assert b==git('show',base+':'+name)==git('show',target+':'+name),name;unchanged[name]=sha(b)
archive=[]
for name in ['t938-semantic-attempt2.log','t938-semantic-attempt3.log','t938-semantic-attempt4.log','t938-contract-race.log','t938-contract-vet.log']:
    b=(pathlib.Path('/private/tmp')/name).read_bytes()
    archive.append(dict(name=name,sha256=sha(b),bytes=len(b),base64=base64.b64encode(b).decode(),final_newline=not b or b.endswith(b'\n')))
expected={'t938-semantic-attempt2.log':'5d8cef909f3cb521c677e51080f223656fe5ae50c96fd072a227ae333ecab0d0','t938-contract-race.log':'5591d16d1a860569aec701b67d3a69594042eac407bd9be57bd6a782918b1ab2'}
for x in archive:
    if x['name'] in expected:assert x['sha256']==expected[x['name']]
raw=json.dumps(archive,separators=(',',':')).encode()
(q/'author-logs.json.gz.b64').write_text(base64.b64encode(gzip.compress(raw,mtime=0)).decode()+'\n')
approval=git('show','37a5b31:docs/justix-auto/state/approvals/auth-transport-compatibility.md')
(q/'typed-error-approval.txt').write_bytes(approval)
entries=json.loads(gzip.decompress(base64.b64decode((q/'author-logs.json.gz.b64').read_text())))
for e in entries:assert sha(base64.b64decode(e['base64']))==e['sha256']
out=dict(target=target,base=base,changed=changed,source=source,unchanged=unchanged,author_log_entries=[{k:v for k,v in e.items() if k!='base64'} for e in entries],author_archive_json_sha256=sha(raw),typed_error_approval_commit='37a5b31ff7ac7d3cf83c26e6a3afc37fb6490ce8',typed_error_approval_sha256=sha(approval),tracked_changes=git('diff','--name-only','HEAD').decode().splitlines())
assert not out['tracked_changes']
(q/'audit.json').write_text(json.dumps(out,indent=2)+'\n')
print(json.dumps(out,indent=2))
