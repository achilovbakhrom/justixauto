import difflib,hashlib,json,pathlib,re,subprocess
wt=pathlib.Path.cwd(); sha='a4de45c811c8eaa7b8d3f7559df0db4c9454a3e3'; base='23d8cede88d228ef57fa73205c159b07e57a49b2'
def git(*args):return subprocess.check_output(['git',*args])
assert git('rev-parse','HEAD').decode().strip()==sha
assert not git('diff','--name-only')
owned=['tools/generate-contracts.mjs','tests/contracts/generation_reproducibility_test.go','docs/justix-auto/dev/results/T-937.md']
assert set(git('diff','--name-only',base,sha).decode().splitlines())==set(owned)
before=git('show',base+':'+owned[1]).decode();after=(wt/owned[1]).read_text()
changes=list(difflib.SequenceMatcher(a=before.splitlines(),b=after.splitlines(),autojunk=False).get_opcodes())
assert all(tag in ['equal','insert'] for tag,*_ in changes)
assert sum(j2-j1 for tag,i1,i2,j1,j2 in changes if tag=='insert')==213
copy='\tdecoder, err := os.ReadFile(filepath.Join(h.root, "web/packages/api/src/responseJSON.ts"))\n\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n\th.write("responseJSON.ts", string(decoder))'
assert before.count(copy)==after.count(copy)==3
result=(wt/owned[2]).read_text();hashes=[]
for name,expected in re.findall(r'\| `([^`]+)`(?: \([^|]*\))? \| `([a-f0-9]{64})` \|',result):
 p=pathlib.Path(name) if name.startswith('/') else wt/name
 actual=hashlib.sha256(p.read_bytes()).hexdigest();assert actual==expected,(name,actual)
 hashes.append({'path':name,'sha256':actual})
main=wt.parent.parent
index=json.loads((main/'docs/justix-auto/dev/task-index.json').read_text());tasks={t['id']:t for t in index['tasks']}
assert tasks['T-937']['files_owned']==owned[:2]
assert tasks['T-937']['depends_on']==['T-936','T-640']
assert tasks['T-938']['depends_on']==['T-937']
approval=(main/'docs/justix-auto/state/approvals/auth-transport-compatibility.md').read_text()
assert 'Closed header schema representation' in approval
print(json.dumps({'sha':sha,'base':base,'changedLeaves':owned,'existingTestLinesPreserved':True,'addedTestLines':213,'decoderCopiesPreserved':3,'hashesVerified':hashes,'serializedTasks':['T-936','T-937','T-938']},indent=2))
