# Membership-version storage architecture result

Latest submission: **final fix cycle2, 2026-09-16; independent r3 QA required**.
This supersedes the fix-cycle1 submission status below. Both BOUNCE rounds and
all earlier result/evidence content remain preserved. A further BOUNCE requires
bounded scope review rather than a third automatic fix cycle.

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

## Final fix cycle2 — constraint timing and reverse mutation guards

2026-09-16. Exact prior proposal
`c3c0da45965c980fee22a32f43e90d7a674a55c4` received independent r2 BOUNCE:
a valid new current R2 link, flushed ALL/named constraints, then a sole R3 CAS
could commit with the new link still pointing to R2. Read MAIN's r2 report,
timing_probe and recovered lifecycle source, and the updated assignment. The
original lifecycle finding is closed; this final correction retains it and
addresses the timing mechanism without weakening the explicit SQL guarantee.

The revised draft specifies immediate ordinary guards on link insertion and
every head mutation, with transactional server-overwritten xid8 bookkeeping
for the one-retained-selection rule. Head mutation checks every newly inserted
link/enrollment against its proposed target; link insertion checks the exact
current or still-pending prospective selection and the complete existing jobs.
A link seals the job set. MS-SELECTION explicitly owns the additional old-table
job BEFORE INSERT trigger, alongside the earlier enrollment INSERT constraint
trigger; old functions/triggers/artifacts remain unchanged.

Snapshot additions are rejected after selection. Deferred checks remain for
incomplete pending records/orphans and may fail early; they are not treated as
commit hooks. ALL/named timing cannot disable the immediate guards. Savepoint
rollback restores row bookkeeping; rolling back a failed second attempt cannot
erase the first retained selection. Markers are bookkeeping, not authorization.
The proposal separately retains the actual transaction/fence-bound Go capability
and source/admission authority requirements.

CurrentSelection takes a head FOR SHARE lock before job/effect work;
ProspectiveSelection takes the prior head FOR UPDATE. The SQL link guard also
takes/revalidates those locks and holds them through transaction end. This closes
the cross-transaction case where a head writer would otherwise advance after
the current link's checks were flushed. Later transactions may advance normally,
preserving valid historical links.

The parameter/default audit now explicitly covers session_replication_role SET
and ALTER SYSTEM authority through PUBLIC/direct/NOINHERIT/SET ROLE paths and
applicable role/database defaults. Installer session/default observations cannot
prove effective runtime login: a server-wide default may be hidden by installer
overrides. Exact fresh runtime-login/database/configuration verification is an
outer T-022/T-933 startup handoff, and T-934 checks the actual runtime handle
before compatible UOW factory binding. Unverifiable startup remains unready;
an inert installation marker is not a runtime-login attestation. This latter
clarification is a specification handoff, not a new executed login probe.

Coordinator follow-up aligns revision7/8 read-only preflight with T-928's final
proposed prerequisite: explicit pg_db_role_setting origin for the exact runtime
LOGIN role and owner database, provisioned externally before quiesced install.
Unrelated/inherited/installer settings are insufficient; fresh direct runtime
login and actual-handle checks remain required. This T-928 configuration contract
is pending independent approval, and MS-CATALOG remains gated on its reviewed
integration. No extra configuration probe or provisioning was performed here.

### Final reduced probe results

`python3 /private/tmp/justix-membership-lifecycle-fix2.py` with the preserved
fix1 script as its base exited0. Final run recorded **63 observations**, including
the prior27 lifecycle observations, engine identity, exact SQL traces and cleanup;
this does not mean63 independent business scenarios.

The final probe checks:

- Both temporal orders with ALL and named immediate/deferred toggles; the sole
  conflicting CAS or stale new link rejects and rolls back. No-toggle control
  also rejects.
- Pending enrollment before CAS, post-selection prospective link, later snapshot
  addition, and job addition after link sealing reject through immediate guards.
- Second head selection rejects with both timing forms. Savepoint rollback of a
  complete first selection permits one replacement; recovery from a failed
  second selection retains the first marker and committed head.
- A legitimate sole head change and normal later current intake pass. Subsequent
  separate-transaction head advancement preserves historical R1/R5 links.
- Supplied bookkeeping values are overwritten in privileged synthetic SQL;
  ordinary runtime bookkeeping-column updates and replication-mode changes deny.
  A NOINHERIT-reachable parameter grant and dangerous role default are detected.
- A current-link transaction flushes its constraints then remains open. A
  concurrent head UPDATE times out on its held lock; no R8 transition survives.
  The current link commits, and a later R8 head change succeeds.

This layers concrete immediate guards over the earlier reduced model, not the
complete prospective migrations or a Go adapter. It does not prove complete
source/bootstrap/backlog authority, full catalog codecs, all real preimage/FK
lineage or the full privilege/default-setting matrix. It does not implement
effective fresh runtime login verification, real COMMIT-loss injection, server
restart, actual Go selection/fence capabilities or cross-service revocation.
The source's trusted seed is still not retained-installation adoption. Future
implementation QA must test the whole approved artifact and actual composition.

No Go/application code changed; the existing interface probe was not rerun.
Four aliases/eight disjoint application leaves and the existing dependency
delta remain. All new SQL/guard work is explicitly in the proposed SQL008/test
leaf; T-934 remains the checker successor and root/runner startup checks retain
their assigned owners. Their at-most4h bounds and T-920's3h limit still require
reslicing before dependent assignment if the full implementation cannot fit.

### Attempts, fixture ownership and reproducible evidence

The first timing-only prototype passed60 observations. A temporary editing/
execution tool call then hit an automatic approval-review timeout; it was not
an unsafe-action rejection. The authorized retry ran the still-unmodified
60-observation script and passed. Inspection showed the lock extension had not
been applied in the timed-out call; it was applied, verified in the source, and
the final63-observation contention run passed. No executed SQL probe failed in
this cycle. Some documentation patches with nonexistent context made no change
before corrected patches applied.

Each fixture had an independently random UUID name and label, exact pinned image,
network none, no host ports and tmpfs-only/no-volume/no-bind mounts. Cleanup was
armed before creation, with exact-name inspection after a lost create response.
Name/label/64-hex ID/image/network/mounts were verified before removing that ID,
then both ID and name absence were checked. No existing infrastructure or unknown
volume was touched. The final source preserves these controls from the
hash-verified fix1 model.

Final fixture:
`justix-membership-fix2-3319967c-673c-4348-b778-26a7fbdefa81`.
Exact ID:
`a79dde6af8bf5869ee048e3ce259a497198af965288df177f08ef7764a798288`.

| Evidence | SHA-256 |
|---|---|
| Final wrapper source | `fafd2ff96ac1b50a053adc411d6f4fa43a8178daf04610ef024509c33825205a` |
| Final63 output | `83341b44093fbf7d0bb5d2880ce325a4136d38795444071678c81e7ff49237e2` |
| First60 output | `e8e2115837ea863511f2b8dfae9ca9fd8faa89e9ba25a108bc862f3b54f84f93` |
| Retry60 output | `c55d60a8d37f2e24edbd8e6284261c03fb102ecc3af747875825b53b6c0e7788` |

Both60-output files reconstruct byte-for-byte from the final JSON: remove the
three observations named `concurrent head cannot change after current-link constraint flush`,
`current link committed while concurrent head attempt rolled back`, and
`head changes normally after current-link transaction ends`; set count60,
substitute the fixture and final cleanup ID below, and serialize using
`json.dumps(report, indent=2) + "\\n"`. Reconstruction was checked against both
actual files and the hashes above.

| Output | Fixture | Cleanup ID |
|---|---|---|
| First60 | `justix-membership-fix2-895878db-b8b4-489d-98ac-a497613c8a0b` | `28c0dba729e65b23af608f55c525bc9275337d7ff229edc3ab3c393a58b246bc` |
| Retry60 | `justix-membership-fix2-36ea4022-dd47-43da-9c2c-17f2b46295df` | `ddbaa1570b5c3c238e9ae0f4717f61ef8fac5beb8456bc905be642f3da83f764` |

Recover the previously embedded fix1 Python source at the wrapper's optional
first argument, or at its default /private/tmp path. The wrapper checks that
source's original SHA before execution. Earlier seven embedded artifacts and
both BOUNCE rounds remain untouched. Only the original assigned draft/result
leaves are changed; independent exact r3 QA and canonical promotion remain
required, with bounded scope review on another BOUNCE.

### Recoverable final timing/locking wrapper

```python
"""Immediate reverse guards layered over the preserved reduced lifecycle model."""
import hashlib,pathlib,sys
base=pathlib.Path(sys.argv[1] if len(sys.argv)>1 else '/private/tmp/justix-membership-lifecycle-fix1.py')
source=base.read_text()
assert hashlib.sha256(source.encode()).hexdigest()=='222d07307c97614f03a95dc9cebc8a2fac8a9ded3ddea6d767e72015f39ee634'
source=source.replace("'justix-membership-fix1-'","'justix-membership-fix2-'")
source=source.replace("'justixauto.arch.membership'","'justixauto.arch.membership-fix2'")
guards=r'''
 sql("""
 ALTER TABLE probe.head ADD COLUMN last_selected_xid xid8;
 CREATE FUNCTION probe.server_birth() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 NEW.created_xid:=pg_current_xact_id(); RETURN NEW; END $$;
 CREATE TRIGGER a_server_birth BEFORE INSERT ON probe.transition FOR EACH ROW EXECUTE FUNCTION probe.server_birth();
 CREATE TRIGGER a_server_birth BEFORE INSERT ON probe.link FOR EACH ROW EXECUTE FUNCTION probe.server_birth();
 CREATE FUNCTION probe.head_reverse_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
 DECLARE t probe.transition%ROWTYPE;
 BEGIN
 IF TG_OP='UPDATE' AND OLD.last_selected_xid=pg_current_xact_id() THEN RAISE EXCEPTION 'second head selection'; END IF;
 SELECT * INTO STRICT t FROM probe.transition WHERE request=NEW.request;
 IF t.created_xid<>pg_current_xact_id() OR t.epoch<>NEW.epoch THEN RAISE EXCEPTION 'invalid selected origin'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.epoch<>1 OR t.prior IS NOT NULL THEN RAISE EXCEPTION 'invalid first head'; END IF;
 ELSE
  IF NEW.epoch<>OLD.epoch+1 OR t.prior IS DISTINCT FROM OLD.request THEN RAISE EXCEPTION 'bad exact head CAS'; END IF;
 END IF;
 IF NOT EXISTS(SELECT FROM probe.stream WHERE request=NEW.request) THEN RAISE EXCEPTION 'incomplete selected snapshot'; END IF;
 IF EXISTS(SELECT FROM probe.link WHERE created_xid=pg_current_xact_id() AND request<>NEW.request) THEN RAISE EXCEPTION 'new link would be stale'; END IF;
 IF EXISTS(SELECT FROM probe.enrollment e WHERE e.xmin=pg_current_xact_id()::xid AND NOT EXISTS(SELECT FROM probe.link l WHERE l.enrollment=e.id AND l.created_xid=pg_current_xact_id() AND l.request=NEW.request)) THEN RAISE EXCEPTION 'pending enrollment before selection'; END IF;
 NEW.last_selected_xid:=pg_current_xact_id(); RETURN NEW;
 END $$;
 CREATE TRIGGER head_reverse_guard BEFORE INSERT OR UPDATE ON probe.head FOR EACH ROW EXECUTE FUNCTION probe.head_reverse_guard();
 CREATE FUNCTION probe.orphan_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF NOT EXISTS(SELECT FROM probe.head WHERE request=NEW.request AND epoch=NEW.epoch) THEN RAISE EXCEPTION 'stale or absent head'; END IF; RETURN NEW; END $$;
 CREATE CONSTRAINT TRIGGER transition_selected AFTER INSERT ON probe.transition DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION probe.orphan_guard();
 CREATE FUNCTION probe.snapshot_seal_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF (SELECT created_xid FROM probe.transition WHERE request=NEW.request) IS DISTINCT FROM pg_current_xact_id() THEN RAISE EXCEPTION 'frozen snapshot'; END IF;
 IF EXISTS(SELECT FROM probe.head WHERE request=NEW.request) THEN RAISE EXCEPTION 'selected snapshot is sealed'; END IF;
 RETURN NEW; END $$;
 CREATE TRIGGER snapshot_seal_guard BEFORE INSERT ON probe.stream FOR EACH ROW EXECUTE FUNCTION probe.snapshot_seal_guard();
 CREATE FUNCTION probe.job_set_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF (SELECT xmin FROM probe.enrollment WHERE id=NEW.enrollment) IS DISTINCT FROM pg_current_xact_id()::xid THEN RAISE EXCEPTION 'job enrollment not created here'; END IF;
 IF EXISTS(SELECT FROM probe.link WHERE enrollment=NEW.enrollment) THEN RAISE EXCEPTION 'enrollment job set sealed'; END IF;
 RETURN NEW; END $$;
 CREATE TRIGGER membership_job_set_guard BEFORE INSERT ON probe.job FOR EACH ROW EXECUTE FUNCTION probe.job_set_guard();
 CREATE FUNCTION probe.link_immediate_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
 DECLARE e probe.enrollment%ROWTYPE; m probe.message%ROWTYPE; t probe.transition%ROWTYPE; expected jsonb; actual jsonb; selected text;
 BEGIN
 SELECT * INTO STRICT e FROM probe.enrollment WHERE id=NEW.enrollment;
 SELECT * INTO STRICT m FROM probe.message WHERE id=e.event;
 SELECT * INTO STRICT t FROM probe.transition WHERE request=NEW.request;
 IF NEW.mode='current' THEN
  SELECT request INTO selected FROM probe.head FOR SHARE;
 ELSE
  SELECT request INTO selected FROM probe.head FOR UPDATE;
 END IF;
 IF NEW.mode='current' AND (selected IS DISTINCT FROM NEW.request OR t.created_xid=pg_current_xact_id()) THEN RAISE EXCEPTION 'stale or absent head'; END IF;
 IF NEW.mode='prospective' AND (t.created_xid<>pg_current_xact_id() OR selected IS DISTINCT FROM t.prior OR EXISTS(SELECT FROM probe.head WHERE last_selected_xid=pg_current_xact_id())) THEN RAISE EXCEPTION 'prospective selection not pending'; END IF;
 IF (NEW.stream,NEW.phase) IS DISTINCT FROM (e.stream,e.phase) OR e.stream IS DISTINCT FROM m.stream THEN RAISE EXCEPTION 'stream or phase mismatch'; END IF;
 IF (e.phase='initial') IS DISTINCT FROM (m.initial_enrollment=e.id) THEN RAISE EXCEPTION 'initial identity mismatch'; END IF;
 IF e.version IS DISTINCT FROM t.catalog OR (e.phase='initial' AND e.version IS DISTINCT FROM m.version) THEN RAISE EXCEPTION 'catalog mismatch'; END IF;
 SELECT CASE WHEN e.phase='initial' THEN consumers ELSE delta END INTO expected FROM probe.stream WHERE request=NEW.request AND stream=NEW.stream;
 SELECT coalesce(jsonb_agg(jsonb_build_object('consumer_name',consumer_name,'admission_id',admission_id) ORDER BY consumer_name),'[]'::jsonb) INTO actual FROM probe.job WHERE enrollment=e.id;
 IF expected IS DISTINCT FROM NEW.consumers OR NEW.consumers IS DISTINCT FROM e.consumers OR e.consumers IS DISTINCT FROM actual OR convert_from(NEW.bytes,'UTF8')::jsonb IS DISTINCT FROM NEW.consumers THEN RAISE EXCEPTION 'set mismatch'; END IF;
 IF EXISTS(SELECT FROM probe.job WHERE enrollment=e.id AND (event<>e.event OR xmin<>pg_current_xact_id()::xid)) THEN RAISE EXCEPTION 'job provenance mismatch'; END IF;
 RETURN NEW; END $$;
 CREATE TRIGGER link_immediate_guard AFTER INSERT ON probe.link FOR EACH ROW EXECUTE FUNCTION probe.link_immediate_guard();
 REVOKE ALL ON ALL FUNCTIONS IN SCHEMA probe FROM PUBLIC;
 """)
'''
needle=" yes('inert installation needs no head and preserves old enrollment'"
assert source.count(needle)==1
source=source.replace(needle,guards+'\n'+needle)
additional=r'''
 def selection(request,epoch,prior,catalog):
  return f"INSERT INTO probe.transition(request,epoch,prior,catalog) VALUES('{request}',{epoch},'{prior}','{catalog}'); INSERT INTO probe.stream VALUES('{request}','S','{full}','[]'); UPDATE probe.head SET request='{request}',epoch={epoch};"
 def fresh(tag,request='R2',catalog='V2'):
  return message('M-'+tag,'E-'+tag,catalog)+enrollment('E-'+tag,'M-'+tag,catalog,'initial',full)+link('E-'+tag,request,'current','initial',full)
 timings=[('all','SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED;'),('named','SET CONSTRAINTS probe.membership_enrollment_complete,probe.link_complete IMMEDIATE; SET CONSTRAINTS probe.membership_enrollment_complete,probe.link_complete DEFERRED;')]
 for label,toggle in timings:
  q='BEGIN; SET LOCAL ROLE runtime; '+fresh(label)+' '+toggle+selection('R3',3,'R2','V3')+' COMMIT;'
  denied('current-link then sole CAS after '+label+' flush',q,'new link would be stale')
  yes(label+' reverse guard rolls back link enrollment and R3',"SELECT (SELECT request FROM probe.head)='R2' AND NOT EXISTS(SELECT FROM probe.transition WHERE request='R3') AND NOT EXISTS(SELECT FROM probe.enrollment WHERE id='E-"+label+"')")
  observations.append({'case':label+' link-first exact SQL','sql':q,'result':'REJECTED by immediate head guard'})
  q='BEGIN; SET LOCAL ROLE runtime; '+selection('R3',3,'R2','V3')+toggle+fresh('head-first-'+label)+' COMMIT;'
  denied('head first then stale current link after '+label+' flush',q,'stale or absent head')
  yes(label+' link guard rolls back head-first transaction',"SELECT (SELECT request FROM probe.head)='R2' AND NOT EXISTS(SELECT FROM probe.transition WHERE request='R3')")
  q='BEGIN; SET LOCAL ROLE runtime; '+fresh('jobs-'+label)+toggle+"INSERT INTO probe.job VALUES('C','M-jobs-"+label+"','E-jobs-"+label+"','CC'); COMMIT;"
  denied('job append after sealed link and '+label+' flush',q,'enrollment job set sealed')
 denied('reverse guard also rejects with no timing toggle','BEGIN; SET LOCAL ROLE runtime; '+fresh('plain')+selection('R3',3,'R2','V3')+' COMMIT;','new link would be stale')
 denied('CAS before pending enrollment gains a link','BEGIN; SET LOCAL ROLE runtime; '+message('M-pending','E-pending')+enrollment('E-pending','M-pending')+selection('R3',3,'R2','V3')+' COMMIT;','pending enrollment before selection')
 denied('new prospective link after its head was already selected','BEGIN; SET LOCAL ROLE runtime; '+selection('R3',3,'R2','V3')+message('M-post','E-post','V3')+enrollment('E-post','M-post','V3','initial',full)+link('E-post','R3','prospective','initial',full)+' COMMIT;','prospective selection not pending')
 denied('selected snapshot cannot gain children after flush','BEGIN; SET LOCAL ROLE runtime; '+selection('R3',3,'R2','V3')+' SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED; '+"INSERT INTO probe.stream VALUES('R3','other','[]','[]'); COMMIT;",'selected snapshot is sealed')
 for label,toggle in timings:
  denied('second head selection after '+label+' flush','BEGIN; SET LOCAL ROLE runtime; '+selection('R3',3,'R2','V3')+toggle+selection('R4',4,'R3','V4')+' COMMIT;','second head selection')
 yes('all rejected timing cases preserve R2',"SELECT (SELECT request FROM probe.head)='R2' AND NOT EXISTS(SELECT FROM probe.transition WHERE request IN ('R3','R4'))")
 # Exact bookkeeping is transactional, not an irreversible GUC counter.
 sql('BEGIN; SET LOCAL ROLE runtime; SAVEPOINT before_selection; '+selection('R3-rolled',3,'R2','V3')+' SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED; ROLLBACK TO before_selection; '+selection('R3',3,'R2','V3')+' COMMIT;')
 yes('savepoint rollback permits one retained replacement head change',"SELECT (SELECT request FROM probe.head)='R3' AND NOT EXISTS(SELECT FROM probe.transition WHERE request='R3-rolled')")
 # Restore no new links after rolling back a selected head: stale input must still reject.
 denied('savepoint rollback cannot leave stale current link accepted','BEGIN; SET LOCAL ROLE runtime; '+fresh('before-savepoint','R3','V3')+' SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED; SAVEPOINT s; '+selection('R4',4,'R3','V4')+' ROLLBACK TO s; COMMIT;','new link would be stale')
 # Preserve the first successful selection after recovering an attempted second inside a savepoint.
 q='BEGIN; SET LOCAL ROLE runtime; '+selection('R4',4,'R3','V4')+' SAVEPOINT bad_second;\n\\set ON_ERROR_STOP off\n'+selection('R5-bad',5,'R4','V5')+'\nROLLBACK TO bad_second;\n\\set ON_ERROR_STOP on\n'+"SELECT request='R4' AND last_selected_xid=pg_current_xact_id() FROM probe.head; COMMIT;"
 p=sql(q); assert p.stderr.count('second head selection')==1 and '\nt\n' in p.stdout,(p.stdout,p.stderr)
 observations.append({'case':'savepoint recovery preserves first-selection marker','result':'PASS','expected_error':p.stderr.strip()})
 yes('failed second transition rolled back while first head committed',"SELECT (SELECT request FROM probe.head)='R4' AND NOT EXISTS(SELECT FROM probe.transition WHERE request='R5-bad')")
 sql('BEGIN; SET LOCAL ROLE runtime; '+selection('R5',5,'R4','V5')+' SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED; COMMIT;')
 yes('one legitimate head change remains valid with timing flush',"SELECT (SELECT request FROM probe.head)='R5'")
 sql('BEGIN; SET LOCAL ROLE runtime; '+fresh('ordinary-final','R5','V5')+' SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED; COMMIT;')
 sql('BEGIN; SET LOCAL ROLE runtime; '+selection('R6',6,'R5','V6')+' COMMIT;')
 yes('later separate transaction may advance head after ordinary link commit',"SELECT l.request='R5' AND h.request='R6' AND l.created_xid<>h.last_selected_xid FROM probe.link l CROSS JOIN probe.head h WHERE l.enrollment='E-ordinary-final'")
 yes('historical R1 enrollment and link remain unchanged',"SELECT (SELECT bytes FROM probe.original_e1)=(SELECT row_to_json(e)::text FROM probe.enrollment e WHERE id='E1') AND (SELECT request FROM probe.link WHERE enrollment='E1')='R1'")
 # Server stamps overwrite even a privileged synthetic caller's supplied values.
 sql("BEGIN; INSERT INTO probe.transition(request,epoch,prior,catalog,created_xid) VALUES('R7',7,'R6','V7','1'::xid8); INSERT INTO probe.stream VALUES('R7','S','"+full+"','[]'); UPDATE probe.head SET request='R7',epoch=7,last_selected_xid='1'::xid8; SELECT 1; COMMIT;")
 yes('supplied bookkeeping was overwritten by server guard',"SELECT t.created_xid=h.last_selected_xid AND t.created_xid<>'1'::xid8 FROM probe.transition t CROSS JOIN probe.head h WHERE t.request='R7'")
 denied('runtime cannot update head bookkeeping column',"SET ROLE runtime; UPDATE probe.head SET last_selected_xid='1'::xid8;",'permission denied')
 denied('runtime cannot disable triggers with replication role',"SET ROLE runtime; SET session_replication_role=replica;",'permission denied')
 yes('normal runtime has no effective replication-role authority',"SELECT current_setting('session_replication_role')='origin' AND NOT has_parameter_privilege('runtime','session_replication_role','SET') AND NOT has_parameter_privilege('runtime','session_replication_role','ALTER SYSTEM')")
 sql("CREATE ROLE parameter_holder NOLOGIN; GRANT SET ON PARAMETER session_replication_role TO parameter_holder; GRANT parameter_holder TO runtime WITH INHERIT FALSE;")
 yes('parameter audit finds NOINHERIT reachable SET ROLE path',"SELECT NOT has_parameter_privilege('runtime','session_replication_role','SET') AND EXISTS(SELECT FROM pg_roles p WHERE pg_has_role('runtime',p.oid,'MEMBER') AND has_parameter_privilege(p.oid,'session_replication_role','SET'))")
 sql("REVOKE parameter_holder FROM runtime; REVOKE SET ON PARAMETER session_replication_role FROM parameter_holder; ALTER ROLE runtime SET session_replication_role=replica;")
 yes('default-setting audit finds dangerous role setting',"SELECT EXISTS(SELECT FROM pg_db_role_setting d CROSS JOIN LATERAL unnest(d.setconfig) s WHERE d.setrole='runtime'::regrole AND s='session_replication_role=replica')")
 sql('ALTER ROLE runtime RESET session_replication_role;')
 yes('origin restored; ordinary SET CONSTRAINTS needs no privileged grant',"SELECT current_setting('session_replication_role')='origin'")
 # A current-link transaction keeps the selected head locked even after early checks.
 args=['docker','exec','-i',NAME,'psql','-X','-h','127.0.0.1','-U','postgres','-v','ON_ERROR_STOP=1','-At']
 current_proc=subprocess.Popen(args,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
 try:
  q="SET application_name='membership-current-lock-probe'; BEGIN; SET LOCAL ROLE runtime; "+fresh('locked-current','R7','V7')+' SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED; SELECT pg_sleep(2); COMMIT;'
  current_proc.stdin.write(q); current_proc.stdin.close()
  for _ in range(50):
   ready=sql("SELECT EXISTS(SELECT FROM pg_stat_activity WHERE application_name='membership-current-lock-probe' AND wait_event='PgSleep')").stdout.strip()
   if ready=='t':break
   time.sleep(.02)
  else:raise AssertionError('current link did not reach held-lock interval')
  denied('concurrent head cannot change after current-link constraint flush',"BEGIN; SET LOCAL ROLE runtime; SET LOCAL lock_timeout='150ms'; "+selection('R8',8,'R7','V8')+' COMMIT;','lock timeout')
  current_proc.wait(timeout=10); out=current_proc.stdout.read(); err=current_proc.stderr.read()
  assert current_proc.returncode==0,(out,err)
 finally:
  if current_proc.poll() is None:
   current_proc.kill();current_proc.wait(timeout=10)
 yes('current link committed while concurrent head attempt rolled back',"SELECT (SELECT request FROM probe.head)='R7' AND NOT EXISTS(SELECT FROM probe.transition WHERE request='R8') AND EXISTS(SELECT FROM probe.link WHERE enrollment='E-locked-current' AND request='R7')")
 sql('BEGIN; SET LOCAL ROLE runtime; '+selection('R8',8,'R7','V8')+' COMMIT;')
 yes('head changes normally after current-link transaction ends',"SELECT (SELECT request FROM probe.head)='R8' AND (SELECT request FROM probe.link WHERE enrollment='E-locked-current')='R7'")
'''
assert source.count('\nfinally:\n')==1
source=source.replace('\nfinally:\n','\n'+additional+'\nfinally:\n')
exec(compile(source,str(base)+'+immediate-fix2','exec'))
```

### Final timing/locking output

```json
{
  "fixture": "justix-membership-fix2-3319967c-673c-4348-b778-26a7fbdefa81",
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
      "case": "current-link then sole CAS after all flush",
      "result": "REJECTED",
      "reason": "new link would be stale"
    },
    {
      "case": "all reverse guard rolls back link enrollment and R3",
      "result": "PASS"
    },
    {
      "case": "all link-first exact SQL",
      "sql": "BEGIN; SET LOCAL ROLE runtime; INSERT INTO probe.message VALUES('M-all','S','E-all','V2','fixed-original-bytes');INSERT INTO probe.enrollment VALUES('E-all','M-all','S','V2','initial','[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"},{\"consumer_name\":\"B\",\"admission_id\":\"BB\"}]'); INSERT INTO probe.job SELECT x->>'consumer_name','M-all','E-all',x->>'admission_id' FROM jsonb_array_elements('[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"},{\"consumer_name\":\"B\",\"admission_id\":\"BB\"}]'::jsonb) x;INSERT INTO probe.link(enrollment,request,stream,mode,phase,consumers,bytes,digest) SELECT 'E-all','R2','S','current','initial','[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"},{\"consumer_name\":\"B\",\"admission_id\":\"BB\"}]',convert_to('[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"},{\"consumer_name\":\"B\",\"admission_id\":\"BB\"}]','UTF8'),sha256(convert_to('[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"},{\"consumer_name\":\"B\",\"admission_id\":\"BB\"}]','UTF8')); SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED;INSERT INTO probe.transition(request,epoch,prior,catalog) VALUES('R3',3,'R2','V3'); INSERT INTO probe.stream VALUES('R3','S','[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"},{\"consumer_name\":\"B\",\"admission_id\":\"BB\"}]','[]'); UPDATE probe.head SET request='R3',epoch=3; COMMIT;",
      "result": "REJECTED by immediate head guard"
    },
    {
      "case": "head first then stale current link after all flush",
      "result": "REJECTED",
      "reason": "stale or absent head"
    },
    {
      "case": "all link guard rolls back head-first transaction",
      "result": "PASS"
    },
    {
      "case": "job append after sealed link and all flush",
      "result": "REJECTED",
      "reason": "enrollment job set sealed"
    },
    {
      "case": "current-link then sole CAS after named flush",
      "result": "REJECTED",
      "reason": "new link would be stale"
    },
    {
      "case": "named reverse guard rolls back link enrollment and R3",
      "result": "PASS"
    },
    {
      "case": "named link-first exact SQL",
      "sql": "BEGIN; SET LOCAL ROLE runtime; INSERT INTO probe.message VALUES('M-named','S','E-named','V2','fixed-original-bytes');INSERT INTO probe.enrollment VALUES('E-named','M-named','S','V2','initial','[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"},{\"consumer_name\":\"B\",\"admission_id\":\"BB\"}]'); INSERT INTO probe.job SELECT x->>'consumer_name','M-named','E-named',x->>'admission_id' FROM jsonb_array_elements('[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"},{\"consumer_name\":\"B\",\"admission_id\":\"BB\"}]'::jsonb) x;INSERT INTO probe.link(enrollment,request,stream,mode,phase,consumers,bytes,digest) SELECT 'E-named','R2','S','current','initial','[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"},{\"consumer_name\":\"B\",\"admission_id\":\"BB\"}]',convert_to('[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"},{\"consumer_name\":\"B\",\"admission_id\":\"BB\"}]','UTF8'),sha256(convert_to('[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"},{\"consumer_name\":\"B\",\"admission_id\":\"BB\"}]','UTF8')); SET CONSTRAINTS probe.membership_enrollment_complete,probe.link_complete IMMEDIATE; SET CONSTRAINTS probe.membership_enrollment_complete,probe.link_complete DEFERRED;INSERT INTO probe.transition(request,epoch,prior,catalog) VALUES('R3',3,'R2','V3'); INSERT INTO probe.stream VALUES('R3','S','[{\"consumer_name\":\"A\",\"admission_id\":\"AA\"},{\"consumer_name\":\"B\",\"admission_id\":\"BB\"}]','[]'); UPDATE probe.head SET request='R3',epoch=3; COMMIT;",
      "result": "REJECTED by immediate head guard"
    },
    {
      "case": "head first then stale current link after named flush",
      "result": "REJECTED",
      "reason": "stale or absent head"
    },
    {
      "case": "named link guard rolls back head-first transaction",
      "result": "PASS"
    },
    {
      "case": "job append after sealed link and named flush",
      "result": "REJECTED",
      "reason": "enrollment job set sealed"
    },
    {
      "case": "reverse guard also rejects with no timing toggle",
      "result": "REJECTED",
      "reason": "new link would be stale"
    },
    {
      "case": "CAS before pending enrollment gains a link",
      "result": "REJECTED",
      "reason": "pending enrollment before selection"
    },
    {
      "case": "new prospective link after its head was already selected",
      "result": "REJECTED",
      "reason": "prospective selection not pending"
    },
    {
      "case": "selected snapshot cannot gain children after flush",
      "result": "REJECTED",
      "reason": "selected snapshot is sealed"
    },
    {
      "case": "second head selection after all flush",
      "result": "REJECTED",
      "reason": "second head selection"
    },
    {
      "case": "second head selection after named flush",
      "result": "REJECTED",
      "reason": "second head selection"
    },
    {
      "case": "all rejected timing cases preserve R2",
      "result": "PASS"
    },
    {
      "case": "savepoint rollback permits one retained replacement head change",
      "result": "PASS"
    },
    {
      "case": "savepoint rollback cannot leave stale current link accepted",
      "result": "REJECTED",
      "reason": "new link would be stale"
    },
    {
      "case": "savepoint recovery preserves first-selection marker",
      "result": "PASS",
      "expected_error": "ERROR:  second head selection\nCONTEXT:  PL/pgSQL function probe.head_reverse_guard() line 4 at RAISE"
    },
    {
      "case": "failed second transition rolled back while first head committed",
      "result": "PASS"
    },
    {
      "case": "one legitimate head change remains valid with timing flush",
      "result": "PASS"
    },
    {
      "case": "later separate transaction may advance head after ordinary link commit",
      "result": "PASS"
    },
    {
      "case": "historical R1 enrollment and link remain unchanged",
      "result": "PASS"
    },
    {
      "case": "supplied bookkeeping was overwritten by server guard",
      "result": "PASS"
    },
    {
      "case": "runtime cannot update head bookkeeping column",
      "result": "REJECTED",
      "reason": "permission denied"
    },
    {
      "case": "runtime cannot disable triggers with replication role",
      "result": "REJECTED",
      "reason": "permission denied"
    },
    {
      "case": "normal runtime has no effective replication-role authority",
      "result": "PASS"
    },
    {
      "case": "parameter audit finds NOINHERIT reachable SET ROLE path",
      "result": "PASS"
    },
    {
      "case": "default-setting audit finds dangerous role setting",
      "result": "PASS"
    },
    {
      "case": "origin restored; ordinary SET CONSTRAINTS needs no privileged grant",
      "result": "PASS"
    },
    {
      "case": "concurrent head cannot change after current-link constraint flush",
      "result": "REJECTED",
      "reason": "lock timeout"
    },
    {
      "case": "current link committed while concurrent head attempt rolled back",
      "result": "PASS"
    },
    {
      "case": "head changes normally after current-link transaction ends",
      "result": "PASS"
    },
    {
      "case": "exact owned fixture removed; no volume mounts",
      "result": "PASS",
      "id": "a79dde6af8bf5869ee048e3ce259a497198af965288df177f08ef7764a798288"
    }
  ],
  "count": 63
}
```
