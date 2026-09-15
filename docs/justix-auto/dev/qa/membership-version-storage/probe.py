"""Independent reduced PG feasibility probe; not the proposed migrations."""
import concurrent.futures
import hashlib
import json
import pathlib
import re
import subprocess
import time
import uuid

ROOT = pathlib.Path(__file__).resolve().parent
IMAGE = 'postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2'
NAME = 'justix-membership-qa-' + str(uuid.uuid4())
TOKEN = str(uuid.uuid4())
LABEL = 'justixauto.qa.membership'
observations = []
fixture_id = None

def run(*argv, data=None, check=True):
    p = subprocess.run(argv, input=data, text=True, capture_output=True, timeout=35)
    if check and p.returncode:
        raise RuntimeError((argv, p.returncode, p.stdout, p.stderr))
    return p

def inspect(target):
    p = run('docker', 'inspect', '--format', '{{json .Id}}|{{json .Name}}|{{json .Config.Image}}|{{json .Config.Labels}}|{{json .Mounts}}|{{json .HostConfig.Tmpfs}}', target, check=False)
    if p.returncode:
        if 'no such object' not in p.stderr.lower() and 'no such container' not in p.stderr.lower():
            raise RuntimeError(p.stderr)
        return None
    return [json.loads(x) for x in p.stdout.strip().split('|')]

def verify_owned(info):
    cid, name, image, labels, mounts, tmpfs = info
    assert re.fullmatch('[0-9a-f]{64}', cid)
    assert name == '/' + NAME and image == IMAGE and labels.get(LABEL) == TOKEN
    assert set(tmpfs or {}) == {'/var/lib/postgresql'}
    assert all(m['Type'] == 'tmpfs' and m['Destination'] == '/var/lib/postgresql' for m in mounts)
    assert fixture_id is None or cid == fixture_id
    return cid

def sql(q, check=True):
    return run('docker', 'exec', '-i', NAME, 'psql', '-X', '-h', '127.0.0.1', '-U', 'postgres', '-v', 'ON_ERROR_STOP=1', '-At', data=q, check=check)

def yes(label, q):
    p = sql(q)
    assert p.stdout.strip() == 't', (label, p.stdout, p.stderr)
    observations.append({'case': label, 'result': 'PASS'})

def denied(label, q, fragment):
    p = sql(q, False)
    assert p.returncode and fragment in p.stderr, (label, p.returncode, p.stdout, p.stderr)
    observations.append({'case': label, 'result': 'REJECTED', 'reason': fragment})

assert inspect(NAME) is None
try:
    # finally is registered before allocation; inspect-by-name handles a lost create reply.
    run('docker', 'create', '--name', NAME, '--label', LABEL + '=' + TOKEN,
        '--network', 'none', '--tmpfs', '/var/lib/postgresql',
        '-e', 'POSTGRES_HOST_AUTH_METHOD=trust', IMAGE)
    fixture_id = verify_owned(inspect(NAME))
    run('docker', 'start', fixture_id)
    for _ in range(150):
        if run('docker', 'exec', NAME, 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres', check=False).returncode == 0:
            break
        time.sleep(.1)
    else:
        raise RuntimeError('PostgreSQL startup timeout')
    yes('pinned PostgreSQL 18.6', "SELECT current_setting('server_version_num')='180006'")
    sql('''
CREATE ROLE runtime NOLOGIN;
CREATE ROLE outsider NOLOGIN;
CREATE SCHEMA probe;
CREATE TABLE probe.catalog(version text PRIMARY KEY, bytes bytea NOT NULL, digest bytea NOT NULL CHECK(digest=sha256(bytes)));
CREATE TABLE probe.transition(request text PRIMARY KEY, epoch bigint UNIQUE NOT NULL CHECK(epoch>0), prior text UNIQUE REFERENCES probe.transition(request), catalog text NOT NULL REFERENCES probe.catalog(version), bytes bytea NOT NULL, digest bytea NOT NULL CHECK(digest=sha256(bytes)), stream_count integer NOT NULL CHECK(stream_count>=0), created_xid xid8 NOT NULL DEFAULT pg_current_xact_id(), UNIQUE(request,epoch), CHECK((epoch=1)=(prior IS NULL)));
CREATE TABLE probe.stream(request text REFERENCES probe.transition(request), stream text, PRIMARY KEY(request,stream));
CREATE TABLE probe.head(singleton bool PRIMARY KEY CHECK(singleton), request text NOT NULL, epoch bigint NOT NULL, FOREIGN KEY(request,epoch) REFERENCES probe.transition(request,epoch));
CREATE TABLE probe.enrollment(id text PRIMARY KEY, created_xid xid8 NOT NULL DEFAULT pg_current_xact_id());
CREATE TABLE probe.link(enrollment text PRIMARY KEY REFERENCES probe.enrollment(id), request text NOT NULL, stream text NOT NULL, FOREIGN KEY(request,stream) REFERENCES probe.stream(request,stream));
CREATE FUNCTION probe.immutable() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN RAISE EXCEPTION 'immutable evidence'; END $$;
CREATE TRIGGER immutable BEFORE UPDATE OR DELETE ON probe.transition FOR EACH STATEMENT EXECUTE FUNCTION probe.immutable();
CREATE FUNCTION probe.complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF (SELECT count(*) FROM probe.stream WHERE request=NEW.request)<>(SELECT stream_count FROM probe.transition WHERE request=NEW.request) THEN RAISE EXCEPTION 'missing stream'; END IF;
 IF TG_OP='UPDATE' AND (NEW.epoch<>OLD.epoch+1 OR (SELECT prior FROM probe.transition WHERE request=NEW.request)<>OLD.request) THEN RAISE EXCEPTION 'bad successor'; END IF;
 RETURN NEW; END $$;
CREATE CONSTRAINT TRIGGER complete AFTER INSERT OR UPDATE ON probe.head DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION probe.complete();
CREATE FUNCTION probe.same_transition_tx() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF (SELECT created_xid FROM probe.transition WHERE request=NEW.request) IS DISTINCT FROM pg_current_xact_id() THEN RAISE EXCEPTION 'transition already committed'; END IF;
 RETURN NEW; END $$;
CREATE CONSTRAINT TRIGGER child_tx AFTER INSERT ON probe.stream DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION probe.same_transition_tx();
-- Literal application of proposal section 4's common new-row provenance guard.
CREATE CONSTRAINT TRIGGER child_tx AFTER INSERT ON probe.link DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION probe.same_transition_tx();
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA probe FROM PUBLIC;
GRANT USAGE ON SCHEMA probe TO runtime;
GRANT SELECT,INSERT ON ALL TABLES IN SCHEMA probe TO runtime;
GRANT UPDATE(request,epoch) ON probe.head TO runtime;
BEGIN;
 SET LOCAL ROLE runtime;
 INSERT INTO probe.catalog SELECT 'V1','\\x01',sha256('\\x01'::bytea);
 INSERT INTO probe.catalog SELECT 'V2','\\x02',sha256('\\x02'::bytea);
 INSERT INTO probe.transition(request,epoch,prior,catalog,bytes,digest,stream_count) VALUES('R1',1,NULL,'V1','\\x01',sha256('\\x01'::bytea),1);
 INSERT INTO probe.stream VALUES('R1','S');
 INSERT INTO probe.head VALUES(true,'R1',1);
COMMIT;
''')
    yes('empty stream retains catalog without custody', "SELECT (SELECT epoch FROM probe.head)=1 AND NOT EXISTS(SELECT FROM probe.enrollment)")
    denied('same version changed bytes', "SET ROLE runtime; INSERT INTO probe.catalog VALUES('V1','\\x02',sha256('\\x02'::bytea));", 'duplicate key')
    denied('hash mismatch', "SET ROLE runtime; INSERT INTO probe.catalog VALUES('V3','\\x02',sha256('\\x03'::bytea));", 'check constraint')
    denied('post-commit transition child rejected', "SET ROLE runtime; INSERT INTO probe.stream VALUES('R1','S-extra');", 'transition already committed')
    denied('future ordinary intake fails under literal transition guard', "BEGIN; SET LOCAL ROLE runtime; INSERT INTO probe.enrollment VALUES('E1',pg_current_xact_id()); INSERT INTO probe.link VALUES('E1','R1','S'); COMMIT;", 'transition already committed')
    yes('failed future intake leaves no custody enrollment', "SELECT NOT EXISTS(SELECT FROM probe.enrollment) AND (SELECT epoch FROM probe.head)=1")
    denied('omitted stream rolls back successor', "BEGIN; SET LOCAL ROLE runtime; INSERT INTO probe.transition(request,epoch,prior,catalog,bytes,digest,stream_count) VALUES('R2',2,'R1','V2','\\x02',sha256('\\x02'::bytea),1); UPDATE probe.head SET request='R2',epoch=2; COMMIT;", 'missing stream')
    yes('missing stream preserved prior state', "SELECT (SELECT epoch FROM probe.head)=1 AND NOT EXISTS(SELECT FROM probe.transition WHERE request='R2')")

    def successor(request):
        return sql(f"BEGIN; SET LOCAL ROLE runtime; SELECT pg_advisory_xact_lock(9145996); INSERT INTO probe.transition(request,epoch,prior,catalog,bytes,digest,stream_count) VALUES('{request}',2,'R1','V2','\\x02',sha256('\\x02'::bytea),1); INSERT INTO probe.stream VALUES('{request}','S'); UPDATE probe.head SET request='{request}',epoch=2 WHERE epoch=1; COMMIT;", False)
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        results = list(pool.map(successor, ['R2a','R2b']))
    assert sorted(x.returncode for x in results) == [0,3]
    observations.append({'case': 'competing exact successors', 'result': 'one commit, one conflict'})
    yes('zero delta version upgrade retains independent receipt', "SELECT (SELECT epoch FROM probe.head)=2 AND (SELECT count(*) FROM probe.transition)=2 AND NOT EXISTS(SELECT FROM probe.enrollment)")
    yes('historical exact request survives later head', "SELECT t.bytes='\\x01'::bytea AND t.digest=sha256(t.bytes) AND h.request<>t.request FROM probe.transition t CROSS JOIN probe.head h WHERE t.request='R1'")
    yes('changed historical request distinguishes conflict', "SELECT digest<>sha256('\\xff'::bytea) FROM probe.transition WHERE request='R1'")
    denied('runtime cannot rewrite receipt', "SET ROLE runtime; UPDATE probe.transition SET catalog='V2' WHERE request='R1';", 'permission denied')
    denied('runtime cannot delete head', "SET ROLE runtime; DELETE FROM probe.head;", 'permission denied')
    denied('isolated role cannot read owner evidence', "SET ROLE outsider; SELECT * FROM probe.transition;", 'permission denied')

    # Feasibility contrast only: enrollment-bound append evidence may reference old transition.
    sql('''DROP TRIGGER child_tx ON probe.link;
CREATE FUNCTION probe.same_enrollment_tx() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF (SELECT created_xid FROM probe.enrollment WHERE id=NEW.enrollment) IS DISTINCT FROM pg_current_xact_id() THEN RAISE EXCEPTION 'enrollment already committed'; END IF;
 RETURN NEW; END $$;
REVOKE ALL ON FUNCTION probe.same_enrollment_tx() FROM PUBLIC;
CREATE CONSTRAINT TRIGGER child_tx AFTER INSERT ON probe.link DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION probe.same_enrollment_tx();
BEGIN; SET LOCAL ROLE runtime; INSERT INTO probe.enrollment VALUES('E2',pg_current_xact_id()); INSERT INTO probe.link SELECT 'E2',request,'S' FROM probe.head; COMMIT;
''')
    yes('separate enrollment provenance permits valid later intake', "SELECT (SELECT count(*) FROM probe.link)=1 AND (SELECT epoch FROM probe.head)=2")
    denied('duplicate enrollment cannot duplicate evidence', "SET ROLE runtime; INSERT INTO probe.link SELECT 'E2',request,'S' FROM probe.head;", 'duplicate key')
    denied('transition children still immutable after separated provenance', "SET ROLE runtime; INSERT INTO probe.stream VALUES('R1','S-extra');", 'transition already committed')
finally:
    info = inspect(NAME)
    if info is not None:
        cid = verify_owned(info)
        run('docker', 'rm', '-f', cid)
        assert inspect(cid) is None and inspect(NAME) is None
        observations.append({'case': 'exact owned fixture cleanup', 'result': 'ID and name absent', 'id': cid})
    report = {'fixture': NAME, 'image': IMAGE, 'observations': observations, 'count': len(observations)}
    print(json.dumps(report, indent=2))
