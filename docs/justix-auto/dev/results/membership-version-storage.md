# Membership-version storage architecture result

Current submission: **fix cycle1; independent r2 QA required**. The original
proposal at `5996c77e3df371a3992154b639ce9ba91ffc3e75` received BOUNCE.
The original result/evidence below is preserved; the appended fix-cycle section
records the revised lifecycle contract and new reduced PostgreSQL verification.

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

## Fix cycle1 — separate frozen snapshots from future enrollment links

Independent QA BOUNCE at `5996c77e3df371a3992154b639ce9ba91ffc3e75`
correctly found that the common transition-created-in-current-transaction rule
would reject future ordinary intake against an older committed selection.
Read MAIN `dev/qa/membership-version-storage.md`, its reduced SQL probe/evidence
and the updated assignment. That report stays unchanged and is not superseded
by a claimed implementation success.

The revised draft explicitly freezes transition/stream/consumer snapshots while
allowing new enrollment evidence to reference an older current transition.
New enrollment/jobs/link remain atomic. MS-SELECTION now explicitly owns an
additive deferred INSERT constraint trigger on the existing enrollment table:
a missing link fails even when no link INSERT ever fired a trigger. All old
SQL/function/trigger definitions remain unchanged; this new attachment is an
enumerated catalog delta, alongside the already declared new FK attachments.

The draft distinguishes CurrentSelection and ProspectiveSelection sealed Go
capabilities, their transaction/backend/fence/request/event/set identity, and
the mandatory SQL versus adapter checks. Enrollment tuple xmin is an immediate
origin predicate, as observed in actual T-919; new link/transition tables retain
server xid8 separately. SQL transaction markers and supplied mode strings are
not authorization. Future current links may reference committed history;
prospective links require their creation transaction's final selection.
Read-only historical redelivery checks original evidence and the retained
lifecycle relation, never current-transaction provenance or current-head equality.
Missing historical links remain held, not repaired or automatically reenrolled.

Four aliases/eight disjoint leaves and six downstream dependency additions
remain unchanged. The new trigger is the missing-link completeness part of
MS-SELECTION's existing SQL008/test responsibility; both sealed capabilities
and exact enrollment verification belong to MS-STATE's existing Go pair.
No new task ID, earlier migration ownership, root or manifest edit is implied.
The at-most4h bounds and T-920's existing3h limit remain explicit; full work that
cannot fit must be resliced by the coordinator before dependent assignment.
No claim that this reduced probe establishes implementation duration is made.

A read-only overlay against the current main task index checked944 existing
tasks plus the four aliases:948 nodes, acyclic; T-591 and all seven owner roots
transitively include every alias, and all eight proposed leaves are disjoint
from existing ownership. This check reserves no IDs and changes no task state.
Seven embedded artifact hashes were verified, and removing only the new current
submission notice leaves the complete original result as an unchanged prefix.

### New verification and limitations

`python3 /private/tmp/justix-membership-lifecycle-fix1.py` exited0 on its first
execution, with 27 recorded observations including engine identity and cleanup:

- Additive deferred enrollment INSERT trigger installed with no active head;
  retained enrollment bytes and the old function/trigger definitions unchanged.
- Empty S selected as R1; later ordinary intake committed one initial enrollment,
  its job and evidence referencing already committed R1 without a new transition.
- Frozen snapshot addition/update, missing or mismatched link, missing/extra job,
  old-enrollment link repair, wrong current/prospective lifecycle and caller
  provenance override rejected. Missing-link failure rolled back message,
  enrollment and job even though no link-insert trigger ran.
- Prospective late B-only enrollment plus R2 selection committed together;
  omission of final selection rolled back transition/delta/head. The old A
  enrollment remained byte-identical and no A job was duplicated.
- Historical R1 evidence remained readable after R2; a new R1 intake rejected
  as stale. Future current R2 intake staged both A and B.
- Retained ambiguous evidence was never backfilled.

This is a **reduced SQL lifecycle model**, not either full proposed migration or
a live T-920/T-921 adapter. It intentionally omits complete catalog codecs,
source authority/bootstrap/high-water/finite-backlog validation, full migration
lineage/privilege denial matrices, cooperating advisory fence concurrency,
actual sealed Go capability implementation, real unknown-COMMIT fault injection
and server restart. It seeds R1 as a trusted fixture despite a separate retained
row, so it does not implement or approve retained-installation adoption. The
retained row tests only non-retroactive DDL and preservation. Existing proposal
probes cover other bounded mechanics but do not fill these implementation gaps.

No Go code changed and the existing Go-interface probe was not rerun for this
SQL-contract correction. Original five embedded probe/source/log blocks retain
their exact prior hashes. The initial fix was documentation-only; two attempted
patches with nonexistent context made no file changes before their corrected
patches applied. These were not SQL probe failures or additional fix cycles.

New source SHA-256:
`222d07307c97614f03a95dc9cebc8a2fac8a9ded3ddea6d767e72015f39ee634`.
New output SHA-256:
`98b1816f823dd4fd0836bb942a24cdd23d46fb853805b07bccb63af2d53c3e07`.

Fixture `justix-membership-fix1-aa9b1066-8d68-49e6-ae7b-f2734057cc51`,
exact ID `f76471b7da03dda2e00154ed772dfc192f601cfbb9712970ea0c5e1b49a8122a`,
used the pinned PG image, network none, no host ports and tmpfs only.
Cleanup was armed before create, including exact-name recovery after lost
creation reply. Name, independent UUID label, 64-hex ID, image, network and
mounts were checked before removal by that exact ID; ID and name were absent
afterward. There were no volume/bind mounts and no existing resource was changed.

Only the assigned draft/result leaves changed. Revised exact-commit independent
r2 review and canonical promotion are still required before implementation.

### Recoverable fix-cycle lifecycle probe

```python
"""Reduced enrollment-link lifecycle model; not a production migration/adapter."""
import hashlib,json,re,subprocess,time,uuid
IMAGE='postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2'
NAME='justix-membership-fix1-'+str(uuid.uuid4())
TOKEN=str(uuid.uuid4()); LABEL='justixauto.arch.membership'; fixture_id=None; observations=[]
def run(*args,data=None,ok=True):
 p=subprocess.run(args,input=data,text=True,capture_output=True,timeout=40)
 if ok and p.returncode: raise RuntimeError((args,p.returncode,p.stdout,p.stderr))
 return p
def inspect(target):
 p=run('docker','inspect',target,ok=False)
 if p.returncode:
  if 'no such object' not in p.stderr.lower() and 'no such container' not in p.stderr.lower(): raise RuntimeError(p.stderr)
  return None
 return json.loads(p.stdout)[0]
def own(info):
 assert re.fullmatch('[0-9a-f]{64}',info['Id'])
 assert info['Name']=='/'+NAME and info['Config']['Image']==IMAGE
 assert info['Config']['Labels'][LABEL]==TOKEN
 assert info['HostConfig']['NetworkMode']=='none' and not info['HostConfig']['PortBindings']
 assert set(info['HostConfig']['Tmpfs'])=={'/var/lib/postgresql'}
 assert all(m['Type']=='tmpfs' and m['Destination']=='/var/lib/postgresql' for m in info['Mounts'])
 assert fixture_id is None or fixture_id==info['Id']
 return info['Id']
def sql(q,ok=True):
 return run('docker','exec','-i',NAME,'psql','-X','-h','127.0.0.1','-U','postgres','-v','ON_ERROR_STOP=1','-At',data=q,ok=ok)
def yes(label,q):
 p=sql(q); assert p.stdout.strip()=='t',(label,p.stdout,p.stderr)
 observations.append({'case':label,'result':'PASS'})
def denied(label,q,fragment):
 p=sql(q,False); assert p.returncode and fragment in p.stderr,(label,p.returncode,p.stdout,p.stderr)
 observations.append({'case':label,'result':'REJECTED','reason':fragment})
def enrollment(e,event='M1',version='V1',phase='initial',members='[{"consumer_name":"A","admission_id":"AA"}]'):
 return f"INSERT INTO probe.enrollment VALUES('{e}','{event}','S','{version}','{phase}','{members}'); INSERT INTO probe.job SELECT x->>'consumer_name','{event}','{e}',x->>'admission_id' FROM jsonb_array_elements('{members}'::jsonb) x;"
def link(e,request='R1',mode='current',phase='initial',members='[{"consumer_name":"A","admission_id":"AA"}]'):
 return f"INSERT INTO probe.link(enrollment,request,stream,mode,phase,consumers,bytes,digest) SELECT '{e}','{request}','S','{mode}','{phase}','{members}',convert_to('{members}','UTF8'),sha256(convert_to('{members}','UTF8'));"
def message(m,e,version='V1'):
 return f"INSERT INTO probe.message VALUES('{m}','S','{e}','{version}','fixed-original-bytes');"
assert inspect(NAME) is None
try:
 # Precreation cleanup is armed; exact name/label/ID/image/mount checks cover lost create reply.
 run('docker','create','--name',NAME,'--label',LABEL+'='+TOKEN,'--network','none','--tmpfs','/var/lib/postgresql','-e','POSTGRES_HOST_AUTH_METHOD=trust',IMAGE)
 fixture_id=own(inspect(NAME)); run('docker','start',fixture_id)
 for _ in range(150):
  if run('docker','exec',NAME,'pg_isready','-h','127.0.0.1','-U','postgres',ok=False).returncode==0: break
  time.sleep(.1)
 else: raise RuntimeError('startup timeout')
 yes('pinned PostgreSQL18.6',"SELECT current_setting('server_version_num')='180006'")
 sql('''
CREATE ROLE runtime NOLOGIN; CREATE SCHEMA probe;
CREATE TABLE probe.transition(request text PRIMARY KEY,epoch bigint UNIQUE NOT NULL,prior text UNIQUE REFERENCES probe.transition(request),catalog text NOT NULL,created_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),UNIQUE(request,epoch),CHECK((epoch=1)=(prior IS NULL)));
CREATE TABLE probe.stream(request text REFERENCES probe.transition(request),stream text,consumers jsonb NOT NULL,delta jsonb NOT NULL,PRIMARY KEY(request,stream));
CREATE TABLE probe.head(singleton bool PRIMARY KEY CHECK(singleton),request text NOT NULL,epoch bigint NOT NULL,FOREIGN KEY(request,epoch) REFERENCES probe.transition(request,epoch));
CREATE TABLE probe.message(id text PRIMARY KEY,stream text,initial_enrollment text NOT NULL,version text NOT NULL,bytes text NOT NULL);
CREATE TABLE probe.enrollment(id text PRIMARY KEY,event text REFERENCES probe.message(id),stream text,version text,phase text CHECK(phase IN ('initial','late')),consumers jsonb NOT NULL);
CREATE TABLE probe.job(consumer_name text,event text,enrollment text REFERENCES probe.enrollment(id),admission_id text,PRIMARY KEY(consumer_name,event));
CREATE TABLE probe.link(enrollment text PRIMARY KEY REFERENCES probe.enrollment(id),request text NOT NULL,stream text NOT NULL,mode text NOT NULL CHECK(mode IN ('current','prospective')),phase text NOT NULL CHECK(phase IN ('initial','late')),consumers jsonb NOT NULL,bytes bytea NOT NULL,digest bytea NOT NULL CHECK(digest=sha256(bytes)),created_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),FOREIGN KEY(request,stream) REFERENCES probe.stream(request,stream));
CREATE FUNCTION probe.old_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN RAISE EXCEPTION 'old immutable'; END $$;
CREATE TRIGGER old_immutable BEFORE UPDATE OR DELETE ON probe.enrollment FOR EACH STATEMENT EXECUTE FUNCTION probe.old_immutable();
-- A separate retained row demonstrates non-retroactive DDL; this model has no adoption API.
INSERT INTO probe.message VALUES('M-retained','old-S','E-retained','old-version','retained-bytes');
INSERT INTO probe.enrollment VALUES('E-retained','M-retained','old-S','old-version','initial','[]');
CREATE TABLE probe.preimages AS SELECT 'row'::text AS kind,md5(row_to_json(e)::text) AS hash FROM probe.enrollment e WHERE id='E-retained'
 UNION ALL SELECT 'function',md5(pg_get_functiondef('probe.old_immutable()'::regprocedure))
 UNION ALL SELECT 'trigger',md5(pg_get_triggerdef(oid)) FROM pg_trigger WHERE tgrelid='probe.enrollment'::regclass AND tgname='old_immutable';
CREATE FUNCTION probe.snapshot_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF (SELECT created_xid FROM probe.transition WHERE request=NEW.request) IS DISTINCT FROM pg_current_xact_id() THEN RAISE EXCEPTION 'frozen snapshot'; END IF; RETURN NEW; END $$;
CREATE CONSTRAINT TRIGGER snapshot_guard AFTER INSERT ON probe.stream DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION probe.snapshot_guard();
CREATE TRIGGER snapshot_immutable BEFORE UPDATE OR DELETE ON probe.stream FOR EACH STATEMENT EXECUTE FUNCTION probe.old_immutable();
CREATE FUNCTION probe.origin_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE origin xid; transition_xid xid8;
BEGIN
 SELECT xmin INTO origin FROM probe.enrollment WHERE id=NEW.enrollment;
 IF origin IS DISTINCT FROM pg_current_xact_id()::xid THEN RAISE EXCEPTION 'enrollment not created here'; END IF;
 IF NEW.created_xid IS DISTINCT FROM pg_current_xact_id() THEN RAISE EXCEPTION 'forged provenance'; END IF;
 SELECT created_xid INTO transition_xid FROM probe.transition WHERE request=NEW.request;
 IF (NEW.mode='current' AND transition_xid=pg_current_xact_id()) OR (NEW.mode='prospective' AND transition_xid IS DISTINCT FROM pg_current_xact_id()) THEN RAISE EXCEPTION 'selection lifecycle'; END IF;
 IF NEW.mode='current' AND NEW.phase<>'initial' THEN RAISE EXCEPTION 'late needs prospective'; END IF;
 RETURN NEW; END $$;
CREATE TRIGGER origin_guard BEFORE INSERT ON probe.link FOR EACH ROW EXECUTE FUNCTION probe.origin_guard();
CREATE FUNCTION probe.complete_enrollment() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE eid text; e probe.enrollment%ROWTYPE; l probe.link%ROWTYPE; m probe.message%ROWTYPE; expected jsonb; actual jsonb;
BEGIN
 IF TG_TABLE_NAME='enrollment' THEN eid:=NEW.id; ELSE eid:=NEW.enrollment; END IF;
 SELECT * INTO STRICT e FROM probe.enrollment WHERE id=eid;
 SELECT * INTO l FROM probe.link WHERE enrollment=eid;
 IF NOT FOUND THEN RAISE EXCEPTION 'missing enrollment link'; END IF;
 IF l.created_xid IS DISTINCT FROM pg_current_xact_id() THEN RAISE EXCEPTION 'link not created here'; END IF;
 SELECT * INTO STRICT m FROM probe.message WHERE id=e.event;
 IF (l.stream,l.phase) IS DISTINCT FROM (e.stream,e.phase) OR e.stream IS DISTINCT FROM m.stream THEN RAISE EXCEPTION 'stream or phase mismatch'; END IF;
 IF (e.phase='initial') IS DISTINCT FROM (m.initial_enrollment=e.id) THEN RAISE EXCEPTION 'initial identity mismatch'; END IF;
 IF e.version IS DISTINCT FROM (SELECT catalog FROM probe.transition WHERE request=l.request) OR (e.phase='initial' AND e.version IS DISTINCT FROM m.version) THEN RAISE EXCEPTION 'catalog mismatch'; END IF;
 IF NOT EXISTS(SELECT FROM probe.head h WHERE h.request=l.request) THEN RAISE EXCEPTION 'stale or absent head'; END IF;
 SELECT CASE WHEN e.phase='initial' THEN consumers ELSE delta END INTO expected FROM probe.stream WHERE request=l.request AND stream=l.stream;
 SELECT coalesce(jsonb_agg(jsonb_build_object('consumer_name',consumer_name,'admission_id',admission_id) ORDER BY consumer_name),'[]'::jsonb) INTO actual FROM probe.job WHERE enrollment=eid;
 IF expected IS DISTINCT FROM l.consumers OR l.consumers IS DISTINCT FROM e.consumers OR e.consumers IS DISTINCT FROM actual OR convert_from(l.bytes,'UTF8')::jsonb IS DISTINCT FROM l.consumers THEN RAISE EXCEPTION 'set mismatch'; END IF;
 IF EXISTS(SELECT FROM probe.job WHERE enrollment=eid AND (event<>e.event OR xmin<>pg_current_xact_id()::xid)) THEN RAISE EXCEPTION 'job provenance mismatch'; END IF;
 RETURN NEW; END $$;
-- The declared new attachment on the old table catches omission even without link INSERT.
CREATE CONSTRAINT TRIGGER membership_enrollment_complete AFTER INSERT ON probe.enrollment DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION probe.complete_enrollment();
CREATE CONSTRAINT TRIGGER link_complete AFTER INSERT ON probe.link DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION probe.complete_enrollment();
CREATE TRIGGER link_immutable BEFORE UPDATE OR DELETE ON probe.link FOR EACH STATEMENT EXECUTE FUNCTION probe.old_immutable();
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA probe FROM PUBLIC;
GRANT USAGE ON SCHEMA probe TO runtime;
GRANT SELECT ON ALL TABLES IN SCHEMA probe TO runtime;
GRANT INSERT ON probe.message,probe.enrollment,probe.job,probe.stream,probe.head TO runtime;
GRANT INSERT(request,epoch,prior,catalog) ON probe.transition TO runtime;
GRANT INSERT(enrollment,request,stream,mode,phase,consumers,bytes,digest) ON probe.link TO runtime;
GRANT UPDATE(request,epoch) ON probe.head TO runtime;
''')
 yes('inert installation needs no head and preserves old enrollment',"SELECT NOT EXISTS(SELECT FROM probe.head) AND (SELECT hash FROM probe.preimages WHERE kind='row')=(SELECT md5(row_to_json(e)::text) FROM probe.enrollment e WHERE id='E-retained')")
 yes('old function and trigger definitions unchanged',"SELECT (SELECT hash FROM probe.preimages WHERE kind='function')=md5(pg_get_functiondef('probe.old_immutable()'::regprocedure)) AND (SELECT hash FROM probe.preimages WHERE kind='trigger')=(SELECT md5(pg_get_triggerdef(oid)) FROM pg_trigger WHERE tgrelid='probe.enrollment'::regclass AND tgname='old_immutable')")
 yes('new enrollment constraint trigger is additive deferred INSERT',"SELECT tgconstraint<>0 AND tgdeferrable AND tginitdeferred AND tgenabled='O' AND tgtype=5 FROM pg_trigger WHERE tgrelid='probe.enrollment'::regclass AND tgname='membership_enrollment_complete'")
 # Trusted fixture seeds the membership model; no initial-adoption/source authorization claim.
 sql("BEGIN; SET LOCAL ROLE runtime; INSERT INTO probe.transition(request,epoch,prior,catalog) VALUES('R1',1,NULL,'V1'); INSERT INTO probe.stream VALUES('R1','S','[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"}]','[]'); INSERT INTO probe.head VALUES(true,'R1',1); COMMIT;")
 yes('R1 selection has no messages on S',"SELECT (SELECT request FROM probe.head)='R1' AND NOT EXISTS(SELECT FROM probe.message WHERE stream='S')")
 sql('BEGIN; SET LOCAL ROLE runtime; '+message('M1','E1')+enrollment('E1')+link('E1')+' COMMIT;')
 yes('future ordinary intake links committed R1',"SELECT l.created_xid<>t.created_xid AND l.mode='current' AND h.epoch=1 FROM probe.link l JOIN probe.transition t ON t.request=l.request CROSS JOIN probe.head h WHERE l.enrollment='E1'")
 sql("CREATE TABLE probe.original_e1 AS SELECT row_to_json(e)::text AS bytes FROM probe.enrollment e WHERE id='E1';")
 denied('post-commit frozen stream addition',"SET ROLE runtime; INSERT INTO probe.stream VALUES('R1','S-late','[]','[]');",'frozen snapshot')
 denied('post-commit snapshot mutation',"UPDATE probe.stream SET consumers='[]' WHERE request='R1';",'old immutable')
 denied('missing link caught from enrollment INSERT', 'BEGIN; SET LOCAL ROLE runtime; '+message('M2','E2')+enrollment('E2','M2')+' COMMIT;','missing enrollment link')
 yes('missing-link failure rolls back message enrollment jobs',"SELECT NOT EXISTS(SELECT FROM probe.message WHERE id='M2') AND NOT EXISTS(SELECT FROM probe.enrollment WHERE id='E2') AND NOT EXISTS(SELECT FROM probe.job WHERE event='M2')")
 denied('mismatched link set', 'BEGIN; SET LOCAL ROLE runtime; '+message('M3','E3')+enrollment('E3','M3')+link('E3',members='[]')+' COMMIT;','set mismatch')
 denied('missing required job', 'BEGIN; SET LOCAL ROLE runtime; '+message('M4','E4')+"INSERT INTO probe.enrollment VALUES('E4','M4','S','V1','initial','[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"}]');"+link('E4')+' COMMIT;','set mismatch')
 denied('extra consumer job', 'BEGIN; SET LOCAL ROLE runtime; '+message('M5','E5')+enrollment('E5','M5')+"INSERT INTO probe.job VALUES('B','M5','E5','BB');"+link('E5')+' COMMIT;','set mismatch')
 denied('link cannot repair old retained enrollment', 'BEGIN; SET LOCAL ROLE runtime; '+link('E-retained')+' COMMIT;','enrollment not created here')
 denied('prospective mode cannot reference committed transition', 'BEGIN; SET LOCAL ROLE runtime; '+message('M6','E6')+enrollment('E6','M6')+link('E6',mode='prospective')+' COMMIT;','selection lifecycle')
 denied('runtime cannot supply full transaction marker',"SET ROLE runtime; INSERT INTO probe.link(enrollment,request,stream,mode,phase,consumers,bytes,digest,created_xid) VALUES('E-retained','R1','S','current','initial','[]','\\x00',sha256('\\x00'::bytea),pg_current_xact_id());",'permission denied')
 full='[{"consumer_name":"A","admission_id":"AA"},{"consumer_name":"B","admission_id":"BB"}]'
 delta='[{"consumer_name":"B","admission_id":"BB"}]'
 upgrade=f"INSERT INTO probe.transition(request,epoch,prior,catalog) VALUES('R2',2,'R1','V2'); INSERT INTO probe.stream VALUES('R2','S','{full}','{delta}');"
 denied('prospective late append without final selection', 'BEGIN; SET LOCAL ROLE runtime; '+upgrade+enrollment('E-late','M1','V2','late',delta)+link('E-late','R2','prospective','late',delta)+' COMMIT;','stale or absent head')
 yes('failed prospective selection leaves old set and head',"SELECT (SELECT request FROM probe.head)='R1' AND NOT EXISTS(SELECT FROM probe.enrollment WHERE id='E-late') AND NOT EXISTS(SELECT FROM probe.transition WHERE request='R2')")
 denied('current mode cannot use a new transition', 'BEGIN; SET LOCAL ROLE runtime; '+upgrade+message('M-current','E-current','V2')+enrollment('E-current','M-current','V2','initial',full)+link('E-current','R2','current','initial',full)+' COMMIT;','selection lifecycle')
 sql('BEGIN; SET LOCAL ROLE runtime; '+upgrade+enrollment('E-late','M1','V2','late',delta)+link('E-late','R2','prospective','late',delta)+"UPDATE probe.head SET request='R2',epoch=2; COMMIT;")
 yes('prospective late delta and new head commit together',"SELECT (SELECT request FROM probe.head)='R2' AND (SELECT count(*) FROM probe.job WHERE event='M1')=2 AND (SELECT consumers FROM probe.enrollment WHERE id='E-late')='[{\"consumer_name\":\"B\",\"admission_id\":\"BB\"}]'::jsonb")
 yes('existing frozen initial enrollment remains byte-identical',"SELECT (SELECT bytes FROM probe.original_e1)=(SELECT row_to_json(e)::text FROM probe.enrollment e WHERE id='E1')")
 yes('historical redelivery reads old link without new insert or current-head condition',"SELECT l.request='R1' AND t.catalog=e.version AND h.request='R2' AND l.consumers=e.consumers AND (SELECT count(*) FROM probe.job WHERE enrollment=e.id)=1 FROM probe.link l JOIN probe.enrollment e ON e.id=l.enrollment JOIN probe.transition t ON t.request=l.request CROSS JOIN probe.head h WHERE e.id='E1'")
 denied('new intake cannot use stale R1', 'BEGIN; SET LOCAL ROLE runtime; '+message('M-stale','E-stale')+enrollment('E-stale','M-stale')+link('E-stale')+' COMMIT;','stale or absent head')
 denied('duplicate evidence cannot append to committed enrollment', 'BEGIN; SET LOCAL ROLE runtime; '+link('E1')+' COMMIT;','enrollment not created here')
 sql('BEGIN; SET LOCAL ROLE runtime; '+message('M7','E7','V2')+enrollment('E7','M7','V2','initial',full)+link('E7','R2','current','initial',full)+' COMMIT;')
 yes('future ordinary R2 intake stages all consumers',"SELECT (SELECT count(*) FROM probe.job WHERE event='M7')=2 AND (SELECT mode FROM probe.link WHERE enrollment='E7')='current'")
 yes('retained unknown membership still unmodified and has no invented link',"SELECT (SELECT hash FROM probe.preimages WHERE kind='row')=(SELECT md5(row_to_json(e)::text) FROM probe.enrollment e WHERE id='E-retained') AND NOT EXISTS(SELECT FROM probe.link WHERE enrollment='E-retained')")
finally:
 info=inspect(NAME)
 if info is not None:
  cid=own(info); run('docker','rm','-f',cid)
  assert inspect(cid) is None and inspect(NAME) is None
  observations.append({'case':'exact owned fixture removed; no volume mounts','result':'PASS','id':cid})
 print(json.dumps({'fixture':NAME,'image':IMAGE,'observations':observations,'count':len(observations)},indent=2))
```

### Fix-cycle lifecycle output

```json
{
  "fixture": "justix-membership-fix1-aa9b1066-8d68-49e6-ae7b-f2734057cc51",
  "image": "postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2",
  "observations": [
    {
      "case": "pinned PostgreSQL18.6",
      "result": "PASS"
    },
    {
      "case": "inert installation needs no head and preserves old enrollment",
      "result": "PASS"
    },
    {
      "case": "old function and trigger definitions unchanged",
      "result": "PASS"
    },
    {
      "case": "new enrollment constraint trigger is additive deferred INSERT",
      "result": "PASS"
    },
    {
      "case": "R1 selection has no messages on S",
      "result": "PASS"
    },
    {
      "case": "future ordinary intake links committed R1",
      "result": "PASS"
    },
    {
      "case": "post-commit frozen stream addition",
      "result": "REJECTED",
      "reason": "frozen snapshot"
    },
    {
      "case": "post-commit snapshot mutation",
      "result": "REJECTED",
      "reason": "old immutable"
    },
    {
      "case": "missing link caught from enrollment INSERT",
      "result": "REJECTED",
      "reason": "missing enrollment link"
    },
    {
      "case": "missing-link failure rolls back message enrollment jobs",
      "result": "PASS"
    },
    {
      "case": "mismatched link set",
      "result": "REJECTED",
      "reason": "set mismatch"
    },
    {
      "case": "missing required job",
      "result": "REJECTED",
      "reason": "set mismatch"
    },
    {
      "case": "extra consumer job",
      "result": "REJECTED",
      "reason": "set mismatch"
    },
    {
      "case": "link cannot repair old retained enrollment",
      "result": "REJECTED",
      "reason": "enrollment not created here"
    },
    {
      "case": "prospective mode cannot reference committed transition",
      "result": "REJECTED",
      "reason": "selection lifecycle"
    },
    {
      "case": "runtime cannot supply full transaction marker",
      "result": "REJECTED",
      "reason": "permission denied"
    },
    {
      "case": "prospective late append without final selection",
      "result": "REJECTED",
      "reason": "stale or absent head"
    },
    {
      "case": "failed prospective selection leaves old set and head",
      "result": "PASS"
    },
    {
      "case": "current mode cannot use a new transition",
      "result": "REJECTED",
      "reason": "selection lifecycle"
    },
    {
      "case": "prospective late delta and new head commit together",
      "result": "PASS"
    },
    {
      "case": "existing frozen initial enrollment remains byte-identical",
      "result": "PASS"
    },
    {
      "case": "historical redelivery reads old link without new insert or current-head condition",
      "result": "PASS"
    },
    {
      "case": "new intake cannot use stale R1",
      "result": "REJECTED",
      "reason": "stale or absent head"
    },
    {
      "case": "duplicate evidence cannot append to committed enrollment",
      "result": "REJECTED",
      "reason": "enrollment not created here"
    },
    {
      "case": "future ordinary R2 intake stages all consumers",
      "result": "PASS"
    },
    {
      "case": "retained unknown membership still unmodified and has no invented link",
      "result": "PASS"
    },
    {
      "case": "exact owned fixture removed; no volume mounts",
      "result": "PASS",
      "id": "f76471b7da03dda2e00154ed772dfc192f601cfbb9712970ea0c5e1b49a8122a"
    }
  ],
  "count": 27
}
```
