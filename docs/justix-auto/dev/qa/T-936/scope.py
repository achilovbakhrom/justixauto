import hashlib, json, pathlib, re, subprocess

wt = pathlib.Path.cwd()
main = wt.parent.parent
sha = '0adcda6d5011c6e602963d0392b1d8241fb4a5a4'
base = '3b796c28fa04105cb0bc7154c17f27f7e47e3470'
def git(*args, cwd=wt):
    return subprocess.check_output(['git', *args], cwd=cwd).decode().strip()
assert git('rev-parse', 'HEAD') == sha
assert not git('diff', '--name-only')
owned = ['web/packages/api/src/client.ts', 'web/packages/api/src/client.test.ts', 'tests/contracts/generation_reproducibility_test.go']
assert set(git('diff', '--name-only', base, sha).splitlines()) == set(owned + ['docs/justix-auto/dev/results/T-936.md'])
diff = git('diff', '--unified=0', base, sha, '--', owned[2])
added = [line[1:] for line in diff.splitlines() if line.startswith('+') and not line.startswith('+++')]
deleted = [line for line in diff.splitlines() if line.startswith('-') and not line.startswith('---')]
copy = ['\tdecoder, err := os.ReadFile(filepath.Join(h.root, "web/packages/api/src/responseJSON.ts"))', '\tif err != nil {', '\t\tt.Fatal(err)', '\t}', '\th.write("responseJSON.ts", string(decoder))']
assert added == copy * 3 and not deleted
index = json.loads((main/'docs/justix-auto/dev/task-index.json').read_text())
tasks = {t['id']:t for t in index['tasks']}
assert len(tasks) == 944 and sum(t['effort_hours'] for t in tasks.values()) == 3138
assert tasks['T-936']['files_owned'] == owned
assert tasks['T-936']['depends_on'] == ['T-935','T-058','T-032','T-640']
assert tasks['T-937']['depends_on'] == ['T-936','T-640']
visiting, done = set(), set()
def visit(id):
    assert id not in visiting
    if id in done: return
    visiting.add(id)
    for dep in tasks[id]['depends_on']: visit(dep)
    visiting.remove(id); done.add(id)
for id in tasks: visit(id)
for id in ['T-936','T-937']:
    text=(main/f'docs/justix-auto/dev/tasks/{id}.md').read_text()
    assert '- Depends on: '+', '.join(tasks[id]['depends_on']) in text
    for file in tasks[id]['files_owned']: assert file in text
    line=next(line for line in (main/'docs/justix-auto/dev/task-board.md').read_text().splitlines() if line.startswith(f'| [{id}]'))
    assert ', '.join(tasks[id]['depends_on']) in line
approval=(main/'docs/justix-auto/state/approvals/auth-transport-compatibility.md').read_text()
assert 'Isolated generator fixture dependency correction' in approval
assert 'T-640 predecessor and existing T-937 successor' in approval
result=(wt/'docs/justix-auto/dev/results/T-936.md').read_text()
hashes=[]
for source, expected in re.findall(r'\| `([^`]+)`(?: \([^|]*\))? \| `([a-f0-9]{64})` \|', result):
    p=pathlib.Path(source) if source.startswith('/') else wt/source
    actual=hashlib.sha256(p.read_bytes()).hexdigest()
    assert actual==expected, (source,actual,expected)
    hashes.append({'path':source,'sha256':actual})
# The table labels this unchanged source explicitly, so inspect it separately.
decoder=hashlib.sha256((wt/'web/packages/api/src/responseJSON.ts').read_bytes()).hexdigest()
assert decoder=='88ec9529a314229b42345de8bcd4179e34139265b7d577c0989631c1f5d3cf6c'
print(json.dumps({'sha':sha,'mainAtScopeCheck':git('rev-parse','HEAD',cwd=main),'changedLeaves':owned+['docs/justix-auto/dev/results/T-936.md'],'fixtureAddedLines':len(added),'fixtureDeletedLines':len(deleted),'graphNodes':len(tasks),'graphHours':3138,'acyclic':True,'serializedChain':['T-640','T-936','T-937'],'resultHashesVerified':hashes,'unchangedDecoder':decoder},indent=2))
