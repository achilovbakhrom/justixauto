"""Reduced enrollment-link lifecycle model; not a production migration/adapter."""
import hashlib,json,re,subprocess,time,uuid
IMAGE='postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2'
NAME='justix-membership-r3-safe-'+str(uuid.uuid4())
TOKEN=str(uuid.uuid4()); LABEL='justixauto.qa.membership-r3-safe'; fixture_id=None; observations=[]
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


 # The recovered matrix independently executes the earlier counterexamples.
 # New QA cases below vary individual constraint toggles and mutation ordering.
 for idx,constraint in enumerate(['probe.membership_enrollment_complete','probe.link_complete']):
  toggle=' SET CONSTRAINTS '+constraint+' IMMEDIATE; SET CONSTRAINTS '+constraint+' DEFERRED; '
  denied('QA one named '+constraint+' cannot flush past reverse head guard',
   'BEGIN; SET LOCAL ROLE runtime; '+fresh('single-'+str(idx),'R8','V8')+toggle+selection('R9',9,'R8','V9')+' COMMIT;', 'new link would be stale')
  denied('QA one named '+constraint+' cannot permit head-first stale intake',
   'BEGIN; SET LOCAL ROLE runtime; '+selection('R9',9,'R8','V9')+toggle+fresh('single-head-'+str(idx),'R8','V8')+' COMMIT;', 'stale or absent head')
  denied('QA one named '+constraint+' cannot unseal an enrollment job set',
   'BEGIN; SET LOCAL ROLE runtime; '+fresh('single-job-'+str(idx),'R8','V8')+toggle+"INSERT INTO probe.job VALUES('C','M-single-job-"+str(idx)+"','E-single-job-"+str(idx)+"','CC'); COMMIT;", 'enrollment job set sealed')

 # Savepoint-contained enrollment writes cannot satisfy top-level xmin origin;
 # safe rejection is the specified limit, not savepoint write support.
 denied('QA subtransaction enrollment fails top-level provenance before link',
  'BEGIN; SET LOCAL ROLE runtime; SAVEPOINT nested; '+fresh('nested','R8','V8')+' COMMIT;', 'job enrollment not created here')
 yes('QA subtransaction failure left no enrollment or message',
  "SELECT NOT EXISTS(SELECT FROM probe.enrollment WHERE id='E-nested') AND NOT EXISTS(SELECT FROM probe.message WHERE id='M-nested')")

 # Runtime cannot forge a server stamp. Privileged fixture input must be replaced,
 # not preserved even if all other content is otherwise correct.
 stamped=link('E-stamp','R8','current','initial',full)
 stamped=stamped.replace('consumers,bytes,digest)', 'consumers,bytes,digest,created_xid)')
 stamped=stamped[:-1]+",'1'::xid8;"
 sql('BEGIN; '+message('M-stamp','E-stamp','V8')+enrollment('E-stamp','M-stamp','V8','initial',full)+stamped+' COMMIT;')
 yes('QA link birth guard overwrites privileged supplied stamp',
  "SELECT created_xid<>'1'::xid8 AND created_xid<>(SELECT created_xid FROM probe.transition WHERE request='R8') FROM probe.link WHERE enrollment='E-stamp'")

 # Reverse concurrent order: head writer owns its row first; stale current link
 # cannot pass using the snapshot observed before waiting for the head lock.
 args=['docker','exec','-i',NAME,'psql','-X','-h','127.0.0.1','-U','postgres','-v','ON_ERROR_STOP=1','-At']
 writer=subprocess.Popen(args,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
 try:
  writer.stdin.write("SET application_name='qa-head-first'; BEGIN; SET LOCAL ROLE runtime; "+selection('R9',9,'R8','V9')+" SELECT pg_sleep(2); COMMIT;")
  writer.stdin.close()
  for _ in range(100):
   if sql("SELECT EXISTS(SELECT FROM pg_stat_activity WHERE application_name='qa-head-first' AND wait_event='PgSleep')").stdout.strip()=='t':break
   time.sleep(.02)
  else:raise AssertionError('head writer did not hold row lock')
  started=time.monotonic()
  denied('QA concurrent stale link revalidates head after waiting for writer commit',
   "BEGIN; SET LOCAL ROLE runtime; SET LOCAL lock_timeout='5s'; "+fresh('head-wait','R8','V8')+' COMMIT;', 'stale or absent head')
  waited=time.monotonic()-started
  assert waited>0.5,waited
  writer.wait(timeout=10)
  assert writer.returncode==0,(writer.stdout.read(),writer.stderr.read())
  observations.append({'case':'QA head-first concurrency wait measured','seconds':round(waited,3),'result':'PASS'})
 finally:
  if writer.poll() is None:writer.kill();writer.wait(timeout=10)
 yes('QA head writer commits R9 and stale reader leaves no effects',
  "SELECT (SELECT request FROM probe.head)='R9' AND NOT EXISTS(SELECT FROM probe.enrollment WHERE id='E-head-wait') AND NOT EXISTS(SELECT FROM probe.message WHERE id='M-head-wait')")

 # Forward concurrent order with named checks instead of ALL. Remain open after
 # flush; the immediate link guard's SHARE lock must still obstruct a head CAS.
 reader=subprocess.Popen(args,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
 try:
  reader.stdin.write("SET application_name='qa-current-named'; BEGIN; SET LOCAL ROLE runtime; "+fresh('named-lock','R9','V9')+' SET CONSTRAINTS probe.membership_enrollment_complete,probe.link_complete IMMEDIATE; SET CONSTRAINTS probe.membership_enrollment_complete,probe.link_complete DEFERRED; SELECT pg_sleep(2); COMMIT;')
  reader.stdin.close()
  for _ in range(100):
   if sql("SELECT EXISTS(SELECT FROM pg_stat_activity WHERE application_name='qa-current-named' AND wait_event='PgSleep')").stdout.strip()=='t':break
   time.sleep(.02)
  else:raise AssertionError('current reader did not hold named-flush lock')
  denied('QA named-flush current link retains SHARE lock through commit',
   "BEGIN; SET LOCAL ROLE runtime; SET LOCAL lock_timeout='150ms'; "+selection('R10',10,'R9','V10')+' COMMIT;', 'lock timeout')
  reader.wait(timeout=10)
  assert reader.returncode==0,(reader.stdout.read(),reader.stderr.read())
 finally:
  if reader.poll() is None:reader.kill();reader.wait(timeout=10)
 yes('QA timed-out head left no R10 while named-flush link committed',
  "SELECT (SELECT request FROM probe.head)='R9' AND NOT EXISTS(SELECT FROM probe.transition WHERE request='R10') AND EXISTS(SELECT FROM probe.link WHERE enrollment='E-named-lock')")
 sql('BEGIN; SET LOCAL ROLE runtime; '+selection('R10',10,'R9','V10')+' COMMIT;')
 yes('QA separate later transaction advances after named-flush reader',
  "SELECT (SELECT request FROM probe.head)='R10' AND (SELECT request FROM probe.link WHERE enrollment='E-named-lock')='R9'")

 yes('QA declaration attaches two extra custom old-table INSERT triggers only',
  "SELECT (SELECT count(*) FROM pg_trigger WHERE tgrelid IN ('probe.enrollment'::regclass,'probe.job'::regclass) AND NOT tgisinternal AND tgname IN ('membership_enrollment_complete','membership_job_set_guard'))=2 AND (SELECT tgtype=7 AND NOT tgdeferrable AND tgenabled='O' FROM pg_trigger WHERE tgrelid='probe.job'::regclass AND tgname='membership_job_set_guard') AND (SELECT hash FROM probe.preimages WHERE kind='trigger')=(SELECT md5(pg_get_triggerdef(oid)) FROM pg_trigger WHERE tgrelid='probe.enrollment'::regclass AND tgname='old_immutable')")

 # Read-only prerequisite observation; no role/database/security setting mutation.
 yes('QA default origin alone does not establish exact LOGIN/database prerequisite',
  "SELECT current_setting('session_replication_role')='origin' AND NOT EXISTS(SELECT FROM pg_db_role_setting s JOIN pg_roles r ON r.oid=s.setrole WHERE r.rolname='runtime' AND r.rolcanlogin AND s.setdatabase=(SELECT oid FROM pg_database WHERE datname=current_database()) AND 'session_replication_role=origin'=ANY(s.setconfig))")
 yes('QA fixture runtime has no effective replication parameter authority',
  "SELECT NOT has_parameter_privilege('runtime','session_replication_role','SET') AND NOT has_parameter_privilege('runtime','session_replication_role','ALTER SYSTEM')")

finally:
 info=inspect(NAME)
 if info is not None:
  cid=own(info); run('docker','rm','-f',cid)
  assert inspect(cid) is None and inspect(NAME) is None
  observations.append({'case':'exact owned fixture removed; no volume mounts','result':'PASS','id':cid})
 print(json.dumps({'fixture':NAME,'image':IMAGE,'observations':observations,'count':len(observations)},indent=2))
