import base64,gzip,json,pathlib
qa=pathlib.Path(__file__).resolve().parent
for archive in ['author-archive.json.gz.b64','fresh-evidence.json.gz.b64']:
    entries=json.loads(gzip.decompress(base64.b64decode((qa/archive).read_text())))
    bypath={x['path']:base64.b64decode(x['base64']) for x in entries}
    populated=[]
    for path,b in bypath.items():
        if not path.endswith('/at17.json'):continue
        parent=path.rsplit('/',1)[0]
        head=json.loads(b);prior=json.loads(bypath[parent+'/at12.json']);base=json.loads(bypath[parent+'/base.json'])
        assert [x['version'] for x in head['history']]==[1,12,17]
        assert head['history'][:2]==prior['history'] and prior['history'][:1]==base['history']
        assert len(head['events'])==1 and head['events']==base['events']
        assert len(head['command_receipts'])==1 and head['command_receipts']==base['command_receipts']
        assert bypath[parent+'/feature12-before.json']==bypath[parent+'/feature12-after.json']
        assert len(json.loads(bypath[parent+'/feature12-before.json']))==1
        populated.append(parent)
    assert len(populated)==1,(archive,populated)
    print(archive,'PASS populated [1,12,17], immutable prior receipts, one retained event/command receipt/feature row:',populated[0])
