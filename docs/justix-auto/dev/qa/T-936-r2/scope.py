import hashlib,json,pathlib,re,subprocess
wt=pathlib.Path.cwd(); main=wt.parent.parent
sha='bf64253647a425f2d818ce30348bc75d91e9f2c8'; old='0adcda6d5011c6e602963d0392b1d8241fb4a5a4'
def git(*args,cwd=wt):return subprocess.check_output(['git',*args],cwd=cwd)
assert git('rev-parse','HEAD').decode().strip()==sha
assert not git('diff','--name-only')
changed=git('diff','--name-only',old,sha).decode().splitlines()
assert set(changed)=={'web/packages/api/src/client.ts','web/packages/api/src/client.test.ts','docs/justix-auto/dev/results/T-936.md'}
result='docs/justix-auto/dev/results/T-936.md'
before=git('show',old+':'+result); after=(wt/result).read_bytes();assert after.startswith(before)
originals=[wt/'docs/justix-auto/dev/qa/T-936.md',*sorted((wt/'docs/justix-auto/dev/qa/T-936').iterdir())]
assert len(originals)==13
evidence=[]
for p in originals:
 rel=str(p.relative_to(wt)); data=p.read_bytes()
 assert data==(main/rel).read_bytes()==git('show','ad800fc:'+rel,cwd=main)
 evidence.append({'path':rel,'sha256':hashlib.sha256(data).hexdigest()})
hashes=[]
for p,expected in re.findall(r'\| `([^`]+)` \| `([a-f0-9]{64})` \|',after[len(before):].decode()):
 file=pathlib.Path(p) if p.startswith('/') else wt/p
 actual=hashlib.sha256(file.read_bytes()).hexdigest();assert actual==expected
 hashes.append({'path':p,'sha256':actual})
assert len(hashes)==10
print(json.dumps({'sha':sha,'previousReview':old,'changedLeaves':changed,'resultPrefixBytesPreserved':len(before),'originalArtifacts':evidence,'fixResultHashes':hashes,'mainSHA':git('rev-parse','HEAD',cwd=main).decode().strip()},indent=2))
