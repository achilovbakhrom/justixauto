# Membership-version storage architecture result

2026-09-15. Proposal only, implementation and independent proposal QA not run.
Assigned base: `d59faa8dc5b26e13e19c47686dd47744b6d73a11`.
Branch: `task/membership-version-storage`.
Draft: [membership-version-storage.md](../../state/drafts/architect/membership-version-storage.md).

The draft closes the specification gap with immutable complete catalog/meaning
evidence, a distinct owner-local current selection and retained CAS transition
history, full per-stream selections, explicit late-stream discovery, zero-delta
receipts and original enrollment evidence links. It proposes four aliases of
at most four hours each, exact leaves and the T-920/921/016/022/934 dependency
delta. No IDs were reserved; no T-928 scope, canonical file, task board, manifest,
application source or dependency was changed.

Read the MAIN assignment/readiness and T-920 result-only commit
`0cbc724f3171786f78aa922f179d8cfe0b75fead`, the assigned worktree AGENTS/dev state,
approved messaging/storage/registration/owner migration contracts, actual
000002–000005/T-014/fence APIs, business/open rules and audited Gaze reference.
T-930 public Profile/Check source was inspected read-only in its separate active
worktree; it is explicitly in-progress evidence, not an integrated guarantee.
No Gaze credentials, reference infrastructure or prototype runtime was used.

## Verification

- Own-root Git gate passed; HEAD matched the assigned base.
- Temporary Go composition probe compiled actual T-014 and PreparedFences APIs.
  `go test -race -mod=readonly /private/tmp/justix-membership-interface_test.go`
  passed (`command-line-arguments 1.671s`); pinned-wrapper vet exited0.
  It verifies inert typed closure construction and fail-before-factory on an
  invalid transaction, not live bootstrap/selection atomicity.
- Reduced SQL proposal probe on pinned PostgreSQL18.6/server180006 passed:
  14 recorded observations including version and fixture cleanup. They are not
  14 independent application scenarios. It retained version identity without
  custody rows, rejected a changed existing version, rolled back an omitted
  stream and head/receipt, allowed exactly one competing successor, retained a
  zero-consumer-delta receipt, and read earlier history after head advancement.
- Each SQL call used a fresh psql client. This demonstrates retained server
  state across adapter/client recreation. It does **not** demonstrate server
  restart/crash durability, actual unknown-commit network injection, full source
  authority, omission of a required catalog claim, real late discovery or actual
  old enrollment preservation. Those remain explicit implementation QA cases.
- The reduced schema is intentionally not either proposed migration: it omits
  complete catalog/claim codecs, most FK/ACL/lineage/late-child guards, actual
  admissions, bootstrap snapshots, jobs and ordinary intake. No production
  readiness or full migration constraint guarantee is derived from it.
- No Go/module/package files changed. The two documentation leaves are the only
  assigned repository writes. `git diff --check` passes.

## Fixture attempts and limits

Cleanup was armed before each Docker create, with an exact UUID-name lookup
covering a lost creation response. Each fixture used network none, no host
ports, tmpfs at /var/lib/postgresql, captured zero attached named/anonymous
volumes, and removal of its exact owned container followed by absence check.
The fixed approved image digest is in the recoverable script below. No unknown
volume was removed and no existing container was changed.

Attempt1 reached the image's temporary initialization server through its Unix
socket; the next connection failed during final server startup. It was corrected
to await TCP readiness inside the network-isolated container. Attempt2 passed
the core SQL checks but its attempted `pg_ctl restart` stopped container PID1;
the restart operation failed. That attempt was not called GREEN. The final
probe removed the unsupported restart claim and checked only fresh clients.
All three exact containers were removed, including both failed attempts:

- `justix-membership-probe-f880e82c-7ced-42ee-a6dc-adcff52249e9`
- `justix-membership-probe-0bb1efb1-a809-400d-bbdc-56dda226eab1`
- `justix-membership-probe-dcedac49-834e-4f84-b7fe-429c7795dfd9`

The initial temporary Go snippet used an incorrect example module import;
inspection of actual go.mod corrected it to `justixauto` before execution.
A documentation patch with a mismatched context failed without changing any
file. Neither is application behavior or an implementation QA fix cycle.

## Retained evidence hashes

| Artifact | SHA-256 |
|---|---|
| Temporary Go probe | `653b8bb6c7063fc2c67018518da22096ec121c380120c13c3d2a89bc18a36535` |
| Final Python SQL/fixture probe | `199cd4661aa6e6ba2148c5dbe58adaeecb7788bb82b6036343c534eec15eec3c` |
| Attempt1 log | `4c954eb5d1497e83744fa00d14017ba03e0791eb3f10f50f76a424f0be5605d8` |
| Attempt2 log | `534130dc57f22d58b9c5f1de7205969c2cfb74473ae79b8d2fd472512dea735c` |
| Final log | `189151301a2a544881fb48f10a45f6a830e84ba443a7c8889af8a6a2177f1c1b` |

Sources and logs below preserve exact UTF-8 content (including final newline)
so proposal QA can recover them without depending on temporary files. No new
application test or permanent test harness is added by this architecture slice.

## Recoverable Go interface probe

```go
package membershipprobe

import (
 "context"
 "testing"
 "justixauto/pkg/eventstore"
 "justixauto/pkg/projection"
 "gorm.io/gorm"
)

type SnapshotPort interface { Install(context.Context) error }
type BootstrapInstaller func(context.Context, *gorm.DB, eventstore.PreparedFences) error

func compose(c projection.Contract, b projection.VerifiedBootstrap, bind func(*gorm.DB) (SnapshotPort,error), verify func(context.Context, SnapshotPort, projection.VerifiedBootstrap) error) BootstrapInstaller {
 return func(ctx context.Context, tx *gorm.DB, f eventstore.PreparedFences) error {
  cp, err := projection.NewCheckpoints(ctx,tx,f,c,bind)
  if err != nil { return err }
  _,err = cp.InstallBootstrap(ctx,b,verify,func(ctx context.Context,p SnapshotPort) error { return p.Install(ctx) })
  return err
 }
}
func TestCompositionIsInert(t *testing.T) {
 calls:=0
 f:=compose(projection.Contract{},projection.VerifiedBootstrap{},func(*gorm.DB)(SnapshotPort,error){calls++;return nil,nil},nil)
 if f==nil || calls!=0 { t.Fatal("composition bound a factory") }
 if err:=f(context.Background(),nil,eventstore.PreparedFences{});err==nil || calls!=0 { t.Fatal("invalid transaction reached factory") }
}
```

## Recoverable reduced PostgreSQL probe

```python
import subprocess, uuid, json, hashlib, pathlib
name='justix-membership-probe-'+str(uuid.uuid4())
image='postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2'
observations=[]
def run(*args, input=None, ok=True):
 p=subprocess.run(args,input=input,text=True,capture_output=True)
 if ok and p.returncode: raise RuntimeError(p.stderr+p.stdout)
 return p
def sql(q,ok=True): return run('docker','exec','-i',name,'psql','-X','-U','postgres','-v','ON_ERROR_STOP=1','-At',input=q,ok=ok)
def check(label,q,expected='t'):
 got=sql(q).stdout.strip()
 if got!=expected: raise AssertionError((label,got,expected))
 observations.append(label)
try:
 # Cleanup is armed before create; inspect by exact random name also covers a lost create reply.
 run('docker','create','--name',name,'--network','none','--tmpfs','/var/lib/postgresql','-e','POSTGRES_HOST_AUTH_METHOD=trust',image)
 run('docker','start',name)
 import time
 for _ in range(100):
  if run('docker','exec',name,'pg_isready','-h','127.0.0.1','-U','postgres',ok=False).returncode==0: break
  time.sleep(.1)
 else: raise RuntimeError('postgres not ready')
 check('pinned PostgreSQL',"SHOW server_version_num",'180006')
 check('no attached volume',"SELECT true")
 inspect=json.loads(run('docker','inspect',name).stdout)[0]
 if any(m['Type']=='volume' for m in inspect['Mounts']): raise AssertionError('unexpected volume')
 sql('''CREATE SCHEMA probe;
 CREATE TABLE probe.catalog(version text PRIMARY KEY,body bytea NOT NULL,digest bytea NOT NULL CHECK(digest=sha256(body)));
 CREATE TABLE probe.transition(request uuid PRIMARY KEY,epoch bigint UNIQUE NOT NULL CHECK(epoch>0),prior uuid UNIQUE REFERENCES probe.transition(request),version text NOT NULL REFERENCES probe.catalog(version),body bytea NOT NULL,digest bytea NOT NULL CHECK(digest=sha256(body)),stream_count integer NOT NULL CHECK(stream_count>=0),UNIQUE(request,epoch),CHECK((epoch=1)=(prior IS NULL)));
 CREATE TABLE probe.stream(request uuid REFERENCES probe.transition(request),stream text,selection bytea NOT NULL,PRIMARY KEY(request,stream));
 CREATE TABLE probe.head(singleton boolean PRIMARY KEY CHECK(singleton),request uuid NOT NULL,epoch bigint NOT NULL,FOREIGN KEY(request,epoch) REFERENCES probe.transition(request,epoch));
 CREATE FUNCTION probe.guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'immutable'; END $$;
 CREATE TRIGGER immutable BEFORE UPDATE OR DELETE ON probe.transition FOR EACH STATEMENT EXECUTE FUNCTION probe.guard();
 CREATE FUNCTION probe.complete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF (SELECT count(*) FROM probe.stream WHERE request=NEW.request)<>(SELECT stream_count FROM probe.transition WHERE request=NEW.request) THEN RAISE EXCEPTION 'incomplete'; END IF;
 IF TG_OP='UPDATE' AND (NEW.epoch<>OLD.epoch+1 OR (SELECT prior FROM probe.transition WHERE request=NEW.request)<>OLD.request) THEN RAISE EXCEPTION 'bad successor'; END IF;
 RETURN NEW; END $$;
 CREATE CONSTRAINT TRIGGER complete AFTER INSERT OR UPDATE ON probe.head DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION probe.complete();
 INSERT INTO probe.catalog SELECT 'V1',convert_to('catalog-A','UTF8'),sha256(convert_to('catalog-A','UTF8'));
 INSERT INTO probe.catalog SELECT 'V2',convert_to('catalog-B','UTF8'),sha256(convert_to('catalog-B','UTF8'));
 BEGIN;
 INSERT INTO probe.transition SELECT '00000000-0000-0000-0000-000000000001',1,NULL,'V1','\\x01',sha256('\\x01'::bytea),1;
 INSERT INTO probe.stream VALUES ('00000000-0000-0000-0000-000000000001','empty-source-S','\\x00');
 INSERT INTO probe.head VALUES(true,'00000000-0000-0000-0000-000000000001',1);
 COMMIT;''')
 check('empty stream retains V1 without custody tables',"SELECT h.epoch=1 AND t.version='V1' AND count(s.*)=1 FROM probe.head h JOIN probe.transition t ON t.request=h.request JOIN probe.stream s ON s.request=t.request GROUP BY h.epoch,t.version")
 check('same catalog version changed identity denied',"SELECT count(*)=2 FROM probe.catalog")
 if sql("INSERT INTO probe.catalog VALUES ('V1','\\x00',sha256('\\x00'::bytea));",False).returncode==0: raise AssertionError('duplicate version accepted')
 if sql("BEGIN; INSERT INTO probe.transition SELECT '00000000-0000-0000-0000-000000000002',2,'00000000-0000-0000-0000-000000000001','V2','\\x02',sha256('\\x02'::bytea),1; UPDATE probe.head SET request='00000000-0000-0000-0000-000000000002',epoch=2; COMMIT;",False).returncode==0: raise AssertionError('omitted stream committed')
 check('omitted required stream rolls back receipt and head',"SELECT (SELECT epoch FROM probe.head)=1 AND NOT EXISTS(SELECT FROM probe.transition WHERE epoch=2)")
 def transition(request,version,delay=''):
  return f"BEGIN; SELECT pg_advisory_xact_lock(7612921); {delay} INSERT INTO probe.transition SELECT '{request}',2,'00000000-0000-0000-0000-000000000001','{version}','\\x02',sha256('\\x02'::bytea),1; INSERT INTO probe.stream VALUES ('{request}','empty-source-S','\\x00'); UPDATE probe.head SET request='{request}',epoch=2 WHERE epoch=1; COMMIT;"
 # Both sessions request the same expected head; distinct immutable successors cannot both commit.
 args=['docker','exec','-i',name,'psql','-X','-U','postgres','-v','ON_ERROR_STOP=1','-At']
 a=subprocess.Popen(args,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
 a.stdin.write(transition('00000000-0000-0000-0000-000000000002','V2','SELECT pg_sleep(0.4);'));a.stdin.close()
 b=sql(transition('00000000-0000-0000-0000-000000000003','V1'),False)
 a.wait();ao=a.stdout.read();ae=a.stderr.read()
 if sorted([a.returncode,b.returncode])!=[0,3]: raise AssertionError((a.returncode,b.returncode,ae,b.stderr))
 observations.append('concurrent distinct versions: one committed successor, one conflict')
 check('zero new consumer delta creates durable revision',"SELECT epoch=2 FROM probe.head")
 check('earlier committed request reconciles after head advance',"SELECT version='V1' AND digest=sha256('\\x01'::bytea) FROM probe.transition WHERE request='00000000-0000-0000-0000-000000000001'")
 check('changed request digest is distinguishable',"SELECT digest<>sha256('\\x99'::bytea) FROM probe.transition WHERE request='00000000-0000-0000-0000-000000000001'")
 check('current selection differs from prior history',"SELECT h.request<>t.request FROM probe.head h CROSS JOIN probe.transition t WHERE t.epoch=1")
 if sql("UPDATE probe.transition SET body='\\x01' WHERE epoch=1",False).returncode==0: raise AssertionError('history mutable')
 observations.append('immutable transition update denied')
 # Fresh client reads model a restarted adapter, not a server/storage restart.
 check('fresh client retains committed receipt',"SELECT count(*)=2 FROM probe.transition")
 # Discarding successful SQL output is not real unknown-commit fault injection.
 sql('BEGIN; SELECT request FROM probe.head; COMMIT;')
 check('fresh connection reads authoritative head',"SELECT count(*)=1 FROM probe.head")
finally:
 info=run('docker','inspect',name,ok=False)
 if info.returncode==0:
  volumes=[m['Name'] for m in json.loads(info.stdout)[0]['Mounts'] if m['Type']=='volume']
  if volumes: raise RuntimeError('unexpected volumes; preserve for review')
  run('docker','rm','-f','-v',name)
  if run('docker','inspect',name,ok=False).returncode==0: raise AssertionError('container remains')
  observations.append('exact owned container removed; captured volumes empty')
 print(json.dumps({'container':name,'observations':observations,'count':len(observations)},indent=2))
```

## Attempt1 log — failed fixture startup

```text
{
  "container": "justix-membership-probe-f880e82c-7ced-42ee-a6dc-adcff52249e9",
  "observations": [
    "pinned PostgreSQL",
    "exact owned container removed; captured volumes empty"
  ],
  "count": 2
}
Traceback (most recent call last):
  File "/private/tmp/justix-membership-storage-probe.py", line 24, in <module>
    check('no attached volume',"SELECT true")
  File "/private/tmp/justix-membership-storage-probe.py", line 11, in check
    got=sql(q).stdout.strip()
  File "/private/tmp/justix-membership-storage-probe.py", line 9, in sql
    def sql(q,ok=True): return run('docker','exec','-i',name,'psql','-X','-U','postgres','-v','ON_ERROR_STOP=1','-At',input=q,ok=ok)
  File "/private/tmp/justix-membership-storage-probe.py", line 7, in run
    if ok and p.returncode: raise RuntimeError(p.stderr+p.stdout)
RuntimeError: psql: error: connection to server on socket "/var/run/postgresql/.s.PGSQL.5432" failed: No such file or directory
	Is the server running locally and accepting connections on that socket?

```

## Attempt2 log — failed restart harness

```text
{
  "container": "justix-membership-probe-0bb1efb1-a809-400d-bbdc-56dda226eab1",
  "observations": [
    "pinned PostgreSQL",
    "no attached volume",
    "empty stream retains V1 without custody tables",
    "same catalog version changed identity denied",
    "omitted required stream rolls back receipt and head",
    "concurrent distinct versions: one committed successor, one conflict",
    "zero new consumer delta creates durable revision",
    "earlier committed request reconciles after head advance",
    "changed request digest is distinguishable",
    "current selection differs from prior history",
    "immutable transition update denied",
    "exact owned container removed; captured volumes empty"
  ],
  "count": 12
}
Traceback (most recent call last):
  File "/private/tmp/justix-membership-storage-probe.py", line 67, in <module>
    run('docker','exec','-u','postgres',name,'sh','-c','pg_ctl -D "$PGDATA" restart -m fast -w')
  File "/private/tmp/justix-membership-storage-probe.py", line 7, in run
    if ok and p.returncode: raise RuntimeError(p.stderr+p.stdout)
RuntimeError: waiting for server to shut down....
```

## Final probe log

```json
{
  "container": "justix-membership-probe-dcedac49-834e-4f84-b7fe-429c7795dfd9",
  "observations": [
    "pinned PostgreSQL",
    "no attached volume",
    "empty stream retains V1 without custody tables",
    "same catalog version changed identity denied",
    "omitted required stream rolls back receipt and head",
    "concurrent distinct versions: one committed successor, one conflict",
    "zero new consumer delta creates durable revision",
    "earlier committed request reconciles after head advance",
    "changed request digest is distinguishable",
    "current selection differs from prior history",
    "immutable transition update denied",
    "fresh client retains committed receipt",
    "fresh connection reads authoritative head",
    "exact owned container removed; captured volumes empty"
  ],
  "count": 14
}
```
