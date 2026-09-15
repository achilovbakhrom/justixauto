"""Independent constraints-timing regressions added to the exact recovered model.
The model is synthetic, not either complete prospective migration.
"""
import hashlib
import pathlib

p = pathlib.Path(__file__).with_name('recovered_lifecycle.py')
source = p.read_text()
assert hashlib.sha256(source.encode()).hexdigest() == '222d07307c97614f03a95dc9cebc8a2fac8a9ded3ddea6d767e72015f39ee634'
assert source.count('\nfinally:\n') == 1
source = source.replace("'justix-membership-fix1-'", "'justix-membership-r2-qa-'")
source = source.replace("'justixauto.arch.membership'", "'justixauto.qa.membership-r2'")

additional = r'''
 # Add independent one-CAS/current-origin guards so timing failures do not rely
 # on the recovered model's missing head-CAS constraint. No application edits.
 sql("""
 CREATE FUNCTION probe.qa_head_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF NEW.epoch<>OLD.epoch+1 OR NOT EXISTS(SELECT FROM probe.transition WHERE request=NEW.request AND epoch=NEW.epoch AND prior=OLD.request AND created_xid=pg_current_xact_id()) THEN RAISE EXCEPTION 'bad exact head CAS'; END IF;
 IF (SELECT count(*) FROM probe.transition WHERE created_xid=pg_current_xact_id())<>1 THEN RAISE EXCEPTION 'multiple transitions in transaction'; END IF;
 RETURN NEW; END $$;
 REVOKE ALL ON FUNCTION probe.qa_head_guard() FROM PUBLIC;
 CREATE TRIGGER qa_head_guard BEFORE UPDATE ON probe.head FOR EACH ROW EXECUTE FUNCTION probe.qa_head_guard();
 CREATE FUNCTION probe.qa_transition_selected() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF NOT EXISTS(SELECT FROM probe.head WHERE request=NEW.request AND epoch=NEW.epoch) THEN RAISE EXCEPTION 'orphan transition'; END IF; RETURN NEW; END $$;
 REVOKE ALL ON FUNCTION probe.qa_transition_selected() FROM PUBLIC;
 CREATE CONSTRAINT TRIGGER qa_transition_selected AFTER INSERT ON probe.transition DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION probe.qa_transition_selected();
 """)
 def selection(request, epoch, prior, catalog):
  return f"INSERT INTO probe.transition(request,epoch,prior,catalog) VALUES('{request}',{epoch},'{prior}','{catalog}'); INSERT INTO probe.stream VALUES('{request}','S','{full}','[]'); UPDATE probe.head SET request='{request}',epoch={epoch};"
 def fresh_intake(tag, request, catalog):
  return message('M-'+tag,'E-'+tag,catalog)+enrollment('E-'+tag,'M-'+tag,catalog,'initial',full)+link('E-'+tag,request,'current','initial',full)

 # Baseline desired failure: current R2 enrollment followed by the sole R3 CAS.
 denied('deferred baseline rejects current intake followed by different head',
  'BEGIN; SET LOCAL ROLE runtime; '+fresh_intake('control','R2','V2')+selection('R3',3,'R2','V3')+' COMMIT;', 'stale or absent head')
 yes('baseline failure preserves R2 and rolls back intake plus R3',
  "SELECT (SELECT request FROM probe.head)='R2' AND NOT EXISTS(SELECT FROM probe.transition WHERE request='R3') AND NOT EXISTS(SELECT FROM probe.enrollment WHERE id='E-control')")

 # Flush precisely the enrollment/link checks, then allow a single valid head
 # transition. Nothing re-enqueues old enrollment checks when the head changes.
 q='BEGIN; SET LOCAL ROLE runtime; '+fresh_intake('all','R2','V2')+' SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED; '+selection('R3',3,'R2','V3')+' COMMIT;'
 sql(q)
 yes('COUNTEREXAMPLE ALL timing flush commits new current R2 evidence with final R3 head',
  "SELECT l.request='R2' AND h.request='R3' AND l.created_xid=(SELECT created_xid FROM probe.transition WHERE request='R3') AND l.created_xid<>(SELECT created_xid FROM probe.transition WHERE request='R2') FROM probe.link l CROSS JOIN probe.head h WHERE l.enrollment='E-all'")
 observations.append({'case':'ALL timing exact transaction','sql':q,'result':'COMMITTED contrary to final-head requirement'})

 q='BEGIN; SET LOCAL ROLE runtime; '+fresh_intake('named','R3','V3')+' SET CONSTRAINTS probe.membership_enrollment_complete,probe.link_complete IMMEDIATE; SET CONSTRAINTS probe.membership_enrollment_complete,probe.link_complete DEFERRED; '+selection('R4',4,'R3','V4')+' COMMIT;'
 sql(q)
 yes('COUNTEREXAMPLE named timing flush has same final-head bypass',
  "SELECT l.request='R3' AND h.request='R4' AND l.created_xid=(SELECT created_xid FROM probe.transition WHERE request='R4') FROM probe.link l CROSS JOIN probe.head h WHERE l.enrollment='E-named'")
 observations.append({'case':'named timing exact transaction','sql':q,'result':'COMMITTED contrary to final-head requirement'})

 # The introduced immediate CAS guard really does reject a second activation.
 denied('independent immediate guard rejects two transitions in one transaction',
  'BEGIN; SET LOCAL ROLE runtime; '+selection('R5',5,'R4','V5')+' SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED; '+selection('R6',6,'R5','V6')+' COMMIT;', 'multiple transitions in transaction')
 yes('two-activation failure keeps R4 head and no R5/R6',
  "SELECT (SELECT request FROM probe.head)='R4' AND NOT EXISTS(SELECT FROM probe.transition WHERE request IN ('R5','R6'))")

 # Early validation is allowed to fail closed, but cannot manufacture evidence.
 denied('immediate check still rejects missing link',
  'BEGIN; SET LOCAL ROLE runtime; '+message('M-missing-now','E-missing-now','V4')+enrollment('E-missing-now','M-missing-now','V4','initial',full)+' SET CONSTRAINTS ALL IMMEDIATE; COMMIT;', 'missing enrollment link')
 denied('immediate prospective validation before final CAS fails closed',
  "BEGIN; SET LOCAL ROLE runtime; INSERT INTO probe.transition(request,epoch,prior,catalog) VALUES('R5',5,'R4','V5'); INSERT INTO probe.stream VALUES('R5','S','"+full+"','[]'); "+message('M-early','E-early','V5')+enrollment('E-early','M-early','V5','initial',full)+link('E-early','R5','prospective','initial',full)+' SET CONSTRAINTS ALL IMMEDIATE; COMMIT;', 'orphan transition')
 sql('BEGIN; SET LOCAL ROLE runtime; '+fresh_intake('after','R4','V4')+' COMMIT;')
 yes('normal future current intake still succeeds after timing probes',
  "SELECT l.request='R4' AND h.request='R4' FROM probe.link l CROSS JOIN probe.head h WHERE l.enrollment='E-after'")
 yes('historical original request and old enrollment remain unchanged',
  "SELECT (SELECT catalog FROM probe.transition WHERE request='R1')='V1' AND (SELECT bytes FROM probe.original_e1)=(SELECT row_to_json(e)::text FROM probe.enrollment e WHERE id='E1')")
'''
source = source.replace('\nfinally:\n', '\n'+additional+'\nfinally:\n')
exec(compile(source, str(p)+'+independent-timing', 'exec'))
