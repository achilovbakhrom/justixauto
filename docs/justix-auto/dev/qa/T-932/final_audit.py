import base64, difflib, gzip, hashlib, json, pathlib, re, subprocess

root=pathlib.Path.cwd();qa=root/'docs/justix-auto/dev/qa/T-932'
def sha(b):return hashlib.sha256(b).hexdigest()
target='555480d7b1c794fd5e9016545d459c975edd756f'
assert subprocess.check_output(['git','rev-parse','HEAD']).decode().strip()==target
author=json.loads((qa/'source-author-audit.json').read_text())
for x in author['source']:
    assert sha((root/x['path']).read_bytes())==x['sha256'],x['path']
compressed=base64.b64decode((qa/'author-archive.json.gz.b64').read_text())
assert sha(compressed)==author['archive_compressed_sha256']
decoded=gzip.decompress(compressed);assert sha(decoded)==author['archive_json_sha256']
entries=json.loads(decoded);assert len(entries)==author['archive_entries']
for x in entries:
    b=base64.b64decode(x['base64']);assert len(b)==x['bytes'] and sha(b)==x['sha256'],x['path']

raw=(root/'pkg/persistence/check.go').read_text().split('const catalogSQL = `',1)[1].split('`',1)[0]
owner=(root/'tools/owner-migrate/driver.go').read_text().split('const catalogSQL = `',1)[1].split('`',1)[0]
normalized=raw
for i in range(1,4):normalized=normalized.replace('?',f'${i}',1)
normalized=normalized.replace('current_user','(SELECT oid FROM runtime)')
normalized=normalized.replace("has_schema_privilege(r.oid,n.oid,'USAGE') AND NOT EXISTS", "has_schema_privilege(r.oid,n.oid,'USAGE') AND n.nspname<>'public' AND NOT EXISTS")
assert normalized==owner,'unaccounted owner/runtime policy drift'
(qa/'catalog-policy.diff').write_text(''.join(difflib.unified_diff(raw.splitlines(True),owner.splitlines(True),fromfile='runtime/check.go',tofile='owner/driver.go')))
logs=['scoped-race.log','independent-race.log','independent-shared-race-r2.log','profile-live.log']
refs=set(author['author_fixture_references']);qaentries=[];dirs=set();qarefs=set()
for name in logs:
    log=(qa/name).read_text();assert '\nFAIL' not in log,name
    for p in re.findall(r'retained (?:synthetic )?evidence directory (\S+)',log):dirs.add(pathlib.Path(p).resolve())
    for a,b in re.findall(r'validated[^\n]*?absent: ([0-9a-f]{64}) (justixauto-[\w-]+)',log):qarefs.update([a,b])
refs.update(qarefs)
sidecars=[]
for directory in sorted(dirs):
    for sidecar in sorted(directory.glob('*.sha256')):
        p=pathlib.Path(str(sidecar)[:-7]);b=p.read_bytes();assert sha(b)==sidecar.read_text().split()[0],p
        sidecars.append(dict(path=str(p),sha256=sha(b)))
        for file in [p,sidecar]:
            data=file.read_bytes();qaentries.append(dict(path=str(file),sha256=sha(data),bytes=len(data),base64=base64.b64encode(data).decode()))
qaarchive=json.dumps(qaentries,separators=(',',':')).encode()
(qa/'fresh-evidence.json.gz.b64').write_text(base64.b64encode(gzip.compress(qaarchive,mtime=0)).decode()+'\n')
absence=[]
for ref in sorted(refs):
    result=subprocess.run(['docker','inspect','--format','{{.Id}}',ref],capture_output=True,text=True,timeout=10)
    assert result.returncode!=0 and 'no such object' in (result.stdout+result.stderr).lower(),(ref,result.returncode)
    absence.append(dict(reference=ref,result='absent',exit=result.returncode))
report=dict(target=target,source_unchanged=True,author_archive_reconstructed_entries=len(entries),catalog_policy='exact after documented placeholder/runtime-OID/fresh-public-schema adjustments',fresh_sidecars=sidecars,fresh_sidecars_count=len(sidecars),fresh_directories=len(dirs),fresh_archive_entries=len(qaentries),fresh_archive_json_sha256=sha(qaarchive),author_fixture_references=len(author['author_fixture_references']),fresh_fixture_references=len(qarefs),independent_absence_checks=absence)
(qa/'final-audit.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k not in ['fresh_sidecars','independent_absence_checks']},indent=2))
