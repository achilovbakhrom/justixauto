#!/usr/bin/env python3
"""Bounded catalog feasibility probe; NOT implementation of migration 000006."""
import hashlib
import json
from pathlib import Path
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[5]
OUT = Path(__file__).with_name('probe-output.txt')
SHA = 'f3fc029de3da3dbadb0cab980ef6bd33c86723b3'
IMAGE = 'postgres@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2'
NAME = 'justix-qa-label-' + uuid.uuid4().hex[:12]
lines = []
checks = 0

def run(args, data=None, okay=True):
    p = subprocess.run(args, input=data, text=True, capture_output=True, timeout=40)
    if okay and p.returncode:
        raise AssertionError((args, p.returncode, p.stdout, p.stderr))
    return p

def log(s):
    lines.append(str(s))
    print(s, flush=True)

def assert_eq(got, want, name):
    global checks
    assert got == want, (name, got, want)
    checks += 1
    log('PASS ' + name)

def sql(query, okay=True):
    return run(['docker', 'exec', '-i', NAME, 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'postgres'], query, okay)

CAT = """SELECT conname, conkey::text, convalidated, conenforced, conislocal,
coninhcount, pg_get_expr(conbin, conrelid), pg_get_constraintdef(oid)
FROM pg_constraint WHERE conrelid='eventstore.consumer_bootstraps'::regclass
AND contype='c' AND 7=ANY(conkey) ORDER BY conname;"""
PREDICATE = """SELECT count(*)=1 AND bool_and(
conname='consumer_bootstraps_generation_check' AND conkey=ARRAY[7]::smallint[]
AND convalidated AND conenforced AND conislocal AND coninhcount=0
AND pg_get_expr(conbin,conrelid)='(length(btrim(generation)) > 0)')
FROM pg_constraint WHERE conrelid='eventstore.consumer_bootstraps'::regclass
AND contype='c' AND 7=ANY(conkey);"""
REPLACE = """ALTER TABLE eventstore.consumer_bootstraps DROP CONSTRAINT consumer_bootstraps_generation_check;
ALTER TABLE eventstore.consumer_bootstraps ADD CONSTRAINT consumer_bootstraps_generation_check CHECK(length(generation)>0);"""

def insert(label, n):
    literal = "'" + label.replace("'", "''") + "'"
    return f"""INSERT INTO eventstore.consumer_bootstraps
(bootstrap_id,consumer_name,source_owner,aggregate_type,aggregate_id,consumer_kind,generation,
contract_id,contract_version,contract_digest,authority_ref,scope_ref,purpose_ref,source_checkpoint_ref,
start_after,no_snapshot_contract_ref,new_empty_proof_ref,installation_request_id)
VALUES ('10000000-0000-4000-8000-{n:012d}','consumer-{n}','identity','test',
'20000000-0000-4000-8000-000000000001','process',{literal},'contract',1,
decode(repeat('ab',32),'hex'),'authority','scope','purpose','source',0,'none','empty',
'30000000-0000-4000-8000-{n:012d}');"""

started = False
volumes = []
try:
    assert_eq(run(['git', '-C', str(ROOT), 'rev-parse', 'HEAD']).stdout.strip(), SHA, 'exact reviewed commit')
    old = ROOT / 'pkg/eventstore/migrations/000004_projection_checkpoint.up.sql'
    original = old.read_bytes()
    assert_eq(hashlib.sha256(original).hexdigest(), '41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c', 'original 000004 hash')
    text = original.decode()
    table = text[text.index('CREATE TABLE eventstore.consumer_bootstraps ('):text.index('CREATE TABLE eventstore.consumer_checkpoints (')]
    log('Owned container ' + NAME)
    run(['docker', 'run', '-d', '--name', NAME, '--network', 'none', '--tmpfs', '/var/lib/postgresql:rw', '-e', 'POSTGRES_HOST_AUTH_METHOD=trust', IMAGE])
    started = True
    inspect = json.loads(run(['docker', 'inspect', NAME]).stdout)[0]
    volumes = [m['Name'] for m in inspect['Mounts'] if m['Type'] == 'volume']
    log('Attached mounts ' + json.dumps(inspect['Mounts'], sort_keys=True))
    assert_eq(volumes, [], 'tmpfs fixture has no attached Docker volume')
    for _ in range(60):
        if run(['docker', 'exec', NAME, 'pg_isready', '-U', 'postgres'], okay=False).returncode == 0:
            break
        time.sleep(.2)
    assert_eq(sql('SHOW server_version_num;').stdout.strip(), '180006', 'pinned PostgreSQL 18.6')
    sql("CREATE ROLE probe_migration; CREATE ROLE probe_runtime; CREATE SCHEMA eventstore AUTHORIZATION probe_migration; SET ROLE probe_migration; CREATE TABLE eventstore.messaging_admissions(admission_id uuid PRIMARY KEY);" + table + "GRANT USAGE ON SCHEMA eventstore TO probe_runtime; GRANT SELECT, INSERT ON eventstore.consumer_bootstraps TO probe_runtime;")
    log('Actual unmodified 000004 CREATE TABLE catalog:\n' + sql(CAT).stdout.strip())
    assert_eq(sql('SET search_path=pg_catalog;' + PREDICATE).stdout.strip(), 't', 'exact constraint catalog predicate')
    assert_eq(sql("SELECT length(btrim(' ')),length(btrim('   ')),length(btrim(E'\\t')),length(btrim(U&'\\00A0'));").stdout.strip(), '0|0|1|1', 'U+0020 versus tab and NBSP distinction')
    sql('SET ROLE probe_runtime;' + insert('  kept  ', 1))
    retained = sql('SELECT row_to_json(b) FROM eventstore.consumer_bootstraps b ORDER BY bootstrap_id;').stdout
    acl = sql("SELECT relacl::text FROM pg_class WHERE oid='eventstore.consumer_bootstraps'::regclass;").stdout
    constraints = sql("SELECT conname,pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='eventstore.consumer_bootstraps'::regclass AND conname<>'consumer_bootstraps_generation_check' ORDER BY conname;").stdout
    oldcat = sql(CAT).stdout
    for n, label in enumerate([' ', '   ', ''], 10):
        p = sql('SET ROLE probe_runtime;' + insert(label, n), okay=False)
        assert_eq('consumer_bootstraps_generation_check' in p.stderr and p.returncode != 0, True, 'old CHECK rejects ' + repr(label))
    sql('SET ROLE probe_runtime;' + insert('\t', 20))
    log('PASS old CHECK admits tab-only label (actual extracted table; no admission trigger installed)')
    retained = sql('SELECT row_to_json(b) FROM eventstore.consumer_bootstraps b ORDER BY bootstrap_id;').stdout
    alterations = {
        'missing': 'DROP CONSTRAINT consumer_bootstraps_generation_check',
        'renamed': 'RENAME CONSTRAINT consumer_bootstraps_generation_check TO wrong_name',
        'duplicate': 'ADD CONSTRAINT duplicate_generation CHECK(length(btrim(generation))>0)',
        'altered': 'DROP CONSTRAINT consumer_bootstraps_generation_check; ALTER TABLE eventstore.consumer_bootstraps ADD CONSTRAINT consumer_bootstraps_generation_check CHECK(length(generation)>0)',
        'unvalidated': 'DROP CONSTRAINT consumer_bootstraps_generation_check; ALTER TABLE eventstore.consumer_bootstraps ADD CONSTRAINT consumer_bootstraps_generation_check CHECK(length(btrim(generation))>0) NOT VALID',
    }
    for name, ddl in alterations.items():
        response = sql('BEGIN; SET LOCAL ROLE probe_migration; ALTER TABLE eventstore.consumer_bootstraps ' + ddl + ';' + PREDICATE + 'ROLLBACK;')
        assert_eq(response.stdout.strip(), 'f', 'catalog rejects ' + name)
        assert_eq(sql(CAT).stdout, oldcat, name + ' trial rollback restores catalog')
    disable = sql('SET ROLE probe_migration; ALTER TABLE eventstore.consumer_bootstraps ALTER CONSTRAINT consumer_bootstraps_generation_check NOT ENFORCED;', okay=False)
    assert_eq(disable.returncode != 0 and 'cannot alter enforceability' in disable.stderr, True, 'pinned server rejects disabling this CHECK through ALTER CONSTRAINT')
    assert_eq(sql(CAT).stdout, oldcat, 'denied disable leaves validated enforced CHECK unchanged')
    failure = sql('BEGIN; SET LOCAL ROLE probe_migration; SET LOCAL lock_timeout=\'5s\';' + REPLACE + 'CREATE TABLE eventstore.synthetic_rev6_marker(revision integer); INSERT INTO eventstore.synthetic_rev6_marker VALUES(6); SELECT 1/0; COMMIT;', okay=False)
    assert_eq(failure.returncode != 0 and 'division by zero' in failure.stderr, True, 'forced later transaction failure')
    assert_eq(sql(CAT).stdout, oldcat, 'failed installation restores old constraint identity and expression')
    assert_eq(sql("SELECT to_regclass('eventstore.synthetic_rev6_marker') IS NULL;").stdout.strip(), 't', 'failed installation leaves no synthetic revision marker')
    assert_eq(sql('SELECT row_to_json(b) FROM eventstore.consumer_bootstraps b ORDER BY bootstrap_id;').stdout, retained, 'failure preserves retained row bytes')
    denied = sql('SET ROLE probe_runtime;' + REPLACE, okay=False)
    assert_eq(denied.returncode != 0 and 'must be owner' in denied.stderr, True, 'runtime cannot replace constraint')
    sql('BEGIN; SET LOCAL ROLE probe_migration; SET LOCAL lock_timeout=\'5s\';' + REPLACE + 'COMMIT;')
    assert_eq(sql('SELECT row_to_json(b) FROM eventstore.consumer_bootstraps b ORDER BY bootstrap_id;').stdout, retained, 'successful replacement preserves retained row bytes')
    assert_eq(sql("SELECT relacl::text FROM pg_class WHERE oid='eventstore.consumer_bootstraps'::regclass;").stdout, acl, 'replacement preserves ACLs')
    assert_eq(sql("SELECT conname,pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='eventstore.consumer_bootstraps'::regclass AND conname<>'consumer_bootstraps_generation_check' ORDER BY conname;").stdout, constraints, 'replacement preserves all other constraints and foreign keys')
    assert_eq(sql("SELECT conname,convalidated,conenforced,pg_get_expr(conbin,conrelid) FROM pg_constraint WHERE conrelid='eventstore.consumer_bootstraps'::regclass AND conname='consumer_bootstraps_generation_check';").stdout.strip(), 'consumer_bootstraps_generation_check|t|t|(length(generation) > 0)', 'new CHECK stable name, validated and enforced')
    for n, label in enumerate([' ', '   ', '\t', '  padded  ', 'not-a-uuid'], 30):
        sql('SET ROLE probe_runtime;' + insert(label, n))
        assert_eq(sql(f"SELECT encode(convert_to(generation,'UTF8'),'hex') FROM eventstore.consumer_bootstraps WHERE consumer_name='consumer-{n}';").stdout.strip(), label.encode().hex(), 'new CHECK preserves exact ' + repr(label))
    empty = sql('SET ROLE probe_runtime;' + insert('', 99), okay=False)
    assert_eq(empty.returncode != 0 and 'consumer_bootstraps_generation_check' in empty.stderr, True, 'replacement still rejects empty')
    assert_eq(hashlib.sha256(old.read_bytes()).hexdigest(), hashlib.sha256(original).hexdigest(), 'no original SQL mutation')
    log(f'{checks} assertions PASS; bounded catalog/DDL feasibility only; no full forward migration or runtime activation proof.')
finally:
    if started:
        run(['docker', 'rm', '-f', '-v', NAME])
        assert_eq(run(['docker', 'inspect', NAME], okay=False).returncode != 0, True, 'owned container removed')
        for volume in volumes:
            assert_eq(run(['docker', 'volume', 'inspect', volume], okay=False).returncode != 0, True, 'known attached volume removed')
    OUT.write_text('\n'.join(lines) + '\n')
