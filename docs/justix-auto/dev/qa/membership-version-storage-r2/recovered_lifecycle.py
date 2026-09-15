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
