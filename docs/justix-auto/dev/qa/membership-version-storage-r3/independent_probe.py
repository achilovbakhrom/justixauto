"""Execute recovered guards plus independent temporal, concurrent and login cases.
No server-wide setting mutation. This remains a reduced feasibility model.
"""
import hashlib
import pathlib
import sys

HERE=pathlib.Path(__file__).resolve().parent
wrapper=(HERE/'recovered_timing.py').read_text()
assert hashlib.sha256(wrapper.encode()).hexdigest()=='fafd2ff96ac1b50a053adc411d6f4fa43a8178daf04610ef024509c33825205a'
assert hashlib.sha256((HERE/'recovered_lifecycle.py').read_bytes()).hexdigest()=='222d07307c97614f03a95dc9cebc8a2fac8a9ded3ddea6d767e72015f39ee634'
assert 'ALTER SYSTEM ' not in wrapper and 'pg_reload_conf' not in wrapper
wrapper=wrapper.replace("\"'justix-membership-fix2-'\"", "\"'justix-membership-r3-qa-'\"")
wrapper=wrapper.replace("\"'justixauto.arch.membership-fix2'\"", "\"'justixauto.qa.membership-r3'\"")

additional=r'''
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

 # Exact role-in-database prerequisite; inspect fixture-local catalog metadata.
 # No ALTER SYSTEM, server config edits, reload or replica-mode session here.
 exact_setting="EXISTS(SELECT FROM pg_db_role_setting s JOIN pg_roles r ON r.oid=s.setrole WHERE r.rolname='runtime' AND r.rolcanlogin AND s.setdatabase=(SELECT oid FROM pg_database WHERE datname=current_database()) AND 'session_replication_role=origin'=ANY(s.setconfig))"
 yes('QA existing origin session alone lacks exact LOGIN/database prerequisite',
  "SELECT current_setting('session_replication_role')='origin' AND NOT ("+exact_setting+")")
 sql('ALTER ROLE runtime LOGIN; ALTER ROLE runtime SET session_replication_role=origin;')
 yes('QA role-global origin still insufficient', 'SELECT NOT ('+exact_setting+')')
 sql('ALTER ROLE runtime RESET session_replication_role; ALTER ROLE parameter_holder IN DATABASE postgres SET session_replication_role=origin;')
 yes('QA other-role database origin still insufficient', 'SELECT NOT ('+exact_setting+')')
 sql('ALTER ROLE runtime IN DATABASE postgres SET session_replication_role=origin;')
 yes('QA exact runtime LOGIN plus current-database origin is present','SELECT '+exact_setting)
 direct=run('docker','exec','-i',NAME,'psql','-X','-h','127.0.0.1','-U','runtime','-d','postgres','-v','ON_ERROR_STOP=1','-At',data="SELECT session_user='runtime' AND current_user='runtime' AND current_database()='postgres' AND current_setting('session_replication_role')='origin' AND NOT has_parameter_privilege(current_user,'session_replication_role','SET') AND NOT has_parameter_privilege(current_user,'session_replication_role','ALTER SYSTEM') AND "+exact_setting+';')
 assert direct.stdout.strip()=='t',(direct.stdout,direct.stderr)
 observations.append({'case':'QA fresh direct runtime LOGIN verifies actual identities origin setting and denied parameter authority','result':'PASS'})
 yes('QA declaration attaches two extra custom old-table INSERT triggers only',
  "SELECT (SELECT count(*) FROM pg_trigger WHERE tgrelid IN ('probe.enrollment'::regclass,'probe.job'::regclass) AND NOT tgisinternal AND tgname IN ('membership_enrollment_complete','membership_job_set_guard'))=2 AND (SELECT tgtype=7 AND NOT tgdeferrable AND tgenabled='O' FROM pg_trigger WHERE tgrelid='probe.job'::regclass AND tgname='membership_job_set_guard') AND (SELECT hash FROM probe.preimages WHERE kind='trigger')=(SELECT md5(pg_get_triggerdef(oid)) FROM pg_trigger WHERE tgrelid='probe.enrollment'::regclass AND tgname='old_immutable')")
'''

needle="source=source.replace('\\nfinally:\\n','\\n'+additional+'\\nfinally:\\n')"
assert wrapper.count(needle)==1
wrapper=wrapper.replace(needle,"additional += QA_ADDITIONAL\n"+needle)
sys.argv=[str(HERE/'recovered_timing.py'),str(HERE/'recovered_lifecycle.py')]
exec(compile(wrapper,str(HERE/'recovered_timing.py')+'+independent-r3','exec'),{'__name__':'__main__','QA_ADDITIONAL':additional})
