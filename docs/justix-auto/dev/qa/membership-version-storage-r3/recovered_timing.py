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
