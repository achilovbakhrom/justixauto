\set ON_ERROR_STOP on
-- Explicit unnumbered owner-local template. The trusted runner verifies ORIGINAL
-- template/base bytes and all evidence before invoking psql -X. SQL cannot hash
-- its input, attest historical execution, backups or stopped processes. Required
-- variables: owner_service, runtime_role, template_sha256, base_schema_sha256,
-- base_artifact_sha256, base_manifest_sha256, profile_sha256, baseline_kind,
-- evidence_ref, approval_ref, backup_ref, stopped_runtimes_ref, request_id.
-- No reapply, automatic adoption, down migration or history inference.
BEGIN;
SET LOCAL search_path=pg_catalog;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='5s';
SET LOCAL transaction_timeout='5s';
SELECT set_config('justix.history_owner', :'owner_service', true);
SELECT set_config('justix.history_runtime', :'runtime_role', true);
SELECT set_config('justix.history_base_schema', :'base_schema_sha256', true);
-- Same one-argument signed big-endian key as the private migration driver.
-- Transaction-scoped acquisition is released on commit/rollback/disconnection.
SELECT pg_advisory_xact_lock(('x'||substr(encode(sha256(convert_to(
  'justixauto:owner-install:v1:'||current_database(),'UTF8')),'hex'),1,16))::bit(64)::bigint);

DO $preflight$
DECLARE o text:=current_setting('justix.history_owner');
  runtime text:=current_setting('justix.history_runtime');
  r pg_roles%ROWTYPE; p pg_roles%ROWTYPE; n record; history_table record; history_column record;
  marker regclass; marker_ok boolean; allowed_update boolean;
BEGIN
  IF o NOT IN ('identity','inventory','commerce','retail','financing','insurance','documents')
    OR current_database()<>'justix_'||o OR current_user<>'justix_'||o
    OR session_user<>current_user OR runtime<>'justix_'||o||'_runtime'
    OR (SELECT datdba FROM pg_database WHERE datname=current_database())<>current_user::regrole::oid
    OR current_setting('justix.history_base_schema')<>'86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53'
    OR to_regnamespace('owner_migrations') IS NOT NULL THEN
    RAISE EXCEPTION 'exact private owner, original shared artifact and absent history required';
  END IF;
  SELECT * INTO STRICT r FROM pg_roles WHERE rolname=runtime;
  marker:=to_regclass(format('%I.compatibility',o||'_mechanics'));
  IF marker IS NULL OR to_regclass('public.schema_migrations') IS NULL THEN
    RAISE EXCEPTION 'existing private version 1 mechanics required';
  END IF;
  LOCK TABLE public.schema_migrations IN ACCESS EXCLUSIVE MODE;
  EXECUTE format('LOCK TABLE %s IN ACCESS SHARE MODE',marker);
  IF (SELECT count(*) FROM pg_attribute WHERE attrelid='public.schema_migrations'::regclass AND attnum>0 AND NOT attisdropped)<>2
    OR NOT EXISTS (SELECT FROM pg_attribute WHERE attrelid='public.schema_migrations'::regclass AND attname='version' AND atttypid='bigint'::regtype AND attnotnull)
    OR NOT EXISTS (SELECT FROM pg_attribute WHERE attrelid='public.schema_migrations'::regclass AND attname='dirty' AND atttypid='boolean'::regtype AND attnotnull)
    OR (SELECT count(*)<>1 OR NOT coalesce(bool_and(version=1 AND NOT dirty),false) FROM public.schema_migrations) THEN
    RAISE EXCEPTION 'exact complete clean version 1 ledger required';
  END IF;
  EXECUTE format('SELECT count(*)=1 AND bool_and(singleton AND owner_service=$1 AND mechanics_version=1) FROM %s',marker) INTO marker_ok USING o;
  IF marker_ok IS DISTINCT FROM true OR (SELECT pg_get_expr(d.adbin,d.adrelid)
    FROM pg_attrdef d JOIN pg_attribute a ON a.attrelid=d.adrelid AND a.attnum=d.adnum
    WHERE d.adrelid='eventstore.events'::regclass AND a.attname='owner_service')
    IS DISTINCT FROM quote_literal(o)||'::text' THEN
    RAISE EXCEPTION 'owner mechanics marker or event owner default mismatch';
  END IF;
  IF to_regclass('eventstore.messaging_mode') IS NOT NULL THEN
    EXECUTE 'SELECT count(*)=1 AND bool_and(mode=''legacy'' AND owner_service=$1 AND runtime_role::text=$2) FROM eventstore.messaging_mode'
      INTO marker_ok USING o,runtime;
    IF marker_ok IS DISTINCT FROM true THEN RAISE EXCEPTION 'history baseline requires explicit legacy messaging mode'; END IF;
  END IF;
  IF NOT has_database_privilege(r.oid,current_database(),'CONNECT') OR EXISTS (
    SELECT FROM pg_database d CROSS JOIN LATERAL aclexplode(d.datacl) x
    WHERE d.datname=current_database() AND x.grantee<>0 AND x.is_grantable
      AND pg_has_role(r.oid,x.grantee,'MEMBER')) THEN
    RAISE EXCEPTION 'missing runtime connect or delegated database privilege';
  END IF;
  FOR n IN SELECT * FROM pg_namespace WHERE nspname IN ('eventstore',o||'_mechanics','public') LOOP
    IF n.nspname<>'public' AND n.nspowner<>current_user::regrole::oid THEN
      RAISE EXCEPTION 'wrong mechanics schema owner';
    END IF;
    IF NOT has_schema_privilege(r.oid,n.oid,'USAGE') THEN RAISE EXCEPTION 'missing runtime schema usage'; END IF;
  END LOOP;
  IF (SELECT count(*) FROM pg_class WHERE relnamespace='eventstore'::regnamespace AND relname IN
    ('events','command_receipts','outbox','inbox','operations','operation_steps') AND relkind='r')<>6 THEN
    RAISE EXCEPTION 'complete shared mechanics required';
  END IF;
  FOR p IN SELECT * FROM pg_roles WHERE pg_has_role(r.oid,oid,'MEMBER') LOOP
    IF p.rolsuper OR p.rolcreatedb OR p.rolcreaterole OR p.rolreplication OR p.rolbypassrls
      OR p.rolname LIKE 'pg\_%' ESCAPE '\' OR p.oid=current_user::regrole::oid
      OR has_database_privilege(p.oid,current_database(),'CREATE,TEMPORARY') THEN
      RAISE EXCEPTION 'runtime privileged role, membership or database grant';
    END IF;
    FOR n IN SELECT * FROM pg_namespace WHERE nspname IN ('eventstore',o||'_mechanics','public') LOOP
      IF has_schema_privilege(p.oid,n.oid,'CREATE') OR EXISTS (SELECT FROM aclexplode(n.nspacl) x
        WHERE x.grantee=p.oid AND x.is_grantable) THEN RAISE EXCEPTION 'runtime schema creation or delegation'; END IF;
    END LOOP;
    FOR history_table IN SELECT tbl.*, ns.nspname FROM pg_class tbl JOIN pg_namespace ns ON ns.oid=tbl.relnamespace
      WHERE tbl.oid IN (marker,'public.schema_migrations'::regclass)
        OR (ns.nspname='eventstore' AND tbl.relname IN ('events','command_receipts','outbox','inbox','operations','operation_steps')) LOOP
      IF history_table.relkind<>'r' OR history_table.relowner<>current_user::regrole::oid
        OR NOT has_table_privilege(r.oid,history_table.oid,'SELECT')
        OR (history_table.nspname='eventstore' AND NOT has_table_privilege(r.oid,history_table.oid,'INSERT'))
        OR has_table_privilege(p.oid,history_table.oid,'UPDATE,DELETE,TRUNCATE,TRIGGER,REFERENCES,MAINTAIN')
        OR (history_table.nspname<>'eventstore' AND has_table_privilege(p.oid,history_table.oid,'INSERT'))
        OR EXISTS (SELECT FROM aclexplode(history_table.relacl) x WHERE x.grantee IN (0,p.oid)
          AND (x.grantee=0 OR x.is_grantable)) THEN
        RAISE EXCEPTION 'incompatible mechanics table owner or privileges';
      END IF;
      FOR history_column IN SELECT * FROM pg_attribute WHERE attrelid=history_table.oid AND attnum>0 AND NOT attisdropped LOOP
        allowed_update:=history_table.nspname='eventstore' AND ((history_table.relname='outbox' AND history_column.attname IN
          ('attempts','next_attempt_at','lease_owner','lease_until','sent_at')) OR
          (history_table.relname='operations' AND history_column.attname IN ('phase','decision','attention_required','result','error',
          'attempts','next_attempt_at','lease_owner','lease_until','revision','updated_at')));
        IF has_column_privilege(p.oid,history_table.oid,history_column.attnum,'REFERENCES')
          OR (history_table.nspname<>'eventstore' AND has_column_privilege(p.oid,history_table.oid,history_column.attnum,'INSERT'))
          OR (has_column_privilege(p.oid,history_table.oid,history_column.attnum,'UPDATE') AND NOT allowed_update)
          OR (allowed_update AND NOT has_column_privilege(r.oid,history_table.oid,history_column.attnum,'UPDATE'))
          OR EXISTS (SELECT FROM aclexplode(history_column.attacl) x WHERE x.grantee IN (0,p.oid)
            AND (x.grantee=0 OR x.is_grantable)) THEN
          RAISE EXCEPTION 'incompatible mechanics column privileges';
        END IF;
      END LOOP;
    END LOOP;
  END LOOP;
  -- New history objects must start private. A schema-specific default cannot
  -- already name the absent schema; reject broad global defaults as well as
  -- privileges inherited through every reachable NOINHERIT role.
  IF EXISTS (SELECT FROM pg_default_acl d CROSS JOIN LATERAL aclexplode(d.defaclacl) a
    WHERE d.defaclrole=current_user::regrole::oid AND d.defaclobjtype IN ('r','f','S','n')
      AND a.grantee<>current_user::regrole::oid) THEN
    RAISE EXCEPTION 'broad non-owner default privileges';
  END IF;
END
$preflight$;

CREATE SCHEMA owner_migrations;
REVOKE ALL ON SCHEMA owner_migrations FROM PUBLIC;
CREATE TABLE owner_migrations.compatibility (
  singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
  owner_service text NOT NULL CHECK(owner_service=:'owner_service'),
  database_name name NOT NULL CHECK(database_name=('justix_'||:'owner_service')::name),
  runtime_role name NOT NULL CHECK(runtime_role=:'runtime_role'::name),
  format_revision integer NOT NULL CHECK(format_revision=1),
  template_sha256 bytea NOT NULL CHECK(octet_length(template_sha256)=32 AND template_sha256<>decode(repeat('00',32),'hex')),
  base_schema_sha256 bytea NOT NULL CHECK(base_schema_sha256=decode('86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53','hex')),
  base_version bigint NOT NULL CHECK(base_version=1),
  base_artifact_sha256 bytea NOT NULL CHECK(octet_length(base_artifact_sha256)=32 AND base_artifact_sha256<>decode(repeat('00',32),'hex')),
  baseline_kind text NOT NULL CHECK(baseline_kind IN ('verified-installation','attested-existing-v1')),
  evidence_ref text NOT NULL CHECK(length(btrim(evidence_ref)) BETWEEN 1 AND 2048 AND evidence_ref !~ '[[:cntrl:]]'),
  approval_ref text NOT NULL CHECK(length(btrim(approval_ref)) BETWEEN 1 AND 2048 AND approval_ref !~ '[[:cntrl:]]'),
  backup_ref text NOT NULL CHECK(length(btrim(backup_ref)) BETWEEN 1 AND 2048 AND backup_ref !~ '[[:cntrl:]]'),
  stopped_runtimes_ref text NOT NULL CHECK(length(btrim(stopped_runtimes_ref)) BETWEEN 1 AND 2048 AND stopped_runtimes_ref !~ '[[:cntrl:]]'),
  installation_request_id uuid NOT NULL UNIQUE CHECK(installation_request_id<>'00000000-0000-0000-0000-000000000000'),
  installed_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(installed_at))
);
CREATE TABLE owner_migrations.feature_contracts (
  contract_id text NOT NULL CHECK(contract_id ~ '^[a-z][a-z0-9.-]{0,127}$'),
  contract_revision bigint NOT NULL CHECK(contract_revision BETWEEN 1 AND 4294967295),
  migration_version bigint NOT NULL UNIQUE CHECK(migration_version>1),
  contract_sha256 bytea NOT NULL CHECK(octet_length(contract_sha256)=32 AND contract_sha256<>decode(repeat('00',32),'hex')),
  PRIMARY KEY(contract_id,contract_revision),
  UNIQUE(contract_id,contract_revision,migration_version,contract_sha256)
);
CREATE TABLE owner_migrations.artifacts (
  version bigint PRIMARY KEY CHECK(version>0),
  filename text NOT NULL UNIQUE CHECK(filename ~ '^migrations/[0-9]+_[a-z][a-z0-9_]*[.]up[.]sql$'),
  sql_sha256 bytea NOT NULL CHECK(octet_length(sql_sha256)=32 AND sql_sha256<>decode(repeat('00',32),'hex')),
  predecessor_version bigint REFERENCES owner_migrations.artifacts(version),
  predecessor_sha256 bytea CHECK(octet_length(predecessor_sha256)=32),
  feature_contract_id text,
  feature_contract_revision bigint,
  feature_contract_sha256 bytea,
  installation_request_id uuid NOT NULL UNIQUE CHECK(installation_request_id<>'00000000-0000-0000-0000-000000000000'),
  artifact_manifest_sha256 bytea NOT NULL CHECK(octet_length(artifact_manifest_sha256)=32 AND artifact_manifest_sha256<>decode(repeat('00',32),'hex')),
  installing_profile_sha256 bytea NOT NULL CHECK(octet_length(installing_profile_sha256)=32 AND installing_profile_sha256<>decode(repeat('00',32),'hex')),
  completed_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(completed_at)),
  provenance_kind text NOT NULL CHECK(provenance_kind IN ('verified-installation','attested-existing-v1')),
  provenance_ref text NOT NULL CHECK(length(btrim(provenance_ref)) BETWEEN 1 AND 2048 AND provenance_ref !~ '[[:cntrl:]]'),
  FOREIGN KEY(feature_contract_id,feature_contract_revision,version,feature_contract_sha256)
    REFERENCES owner_migrations.feature_contracts(contract_id,contract_revision,migration_version,contract_sha256),
  CHECK((version=1 AND predecessor_version IS NULL AND predecessor_sha256 IS NULL
      AND feature_contract_id IS NULL AND feature_contract_revision IS NULL AND feature_contract_sha256 IS NULL)
    OR (version>1 AND predecessor_version IS NOT NULL AND predecessor_version<version AND predecessor_sha256 IS NOT NULL
      AND feature_contract_id IS NOT NULL AND feature_contract_revision IS NOT NULL AND feature_contract_sha256 IS NOT NULL
      AND provenance_kind='verified-installation'))
);

CREATE FUNCTION owner_migrations.immutable() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION 'owner migration evidence is immutable' USING ERRCODE='23514'; END
$$;
CREATE FUNCTION owner_migrations.feature_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE b owner_migrations.compatibility%ROWTYPE;
BEGIN
  SELECT * INTO STRICT b FROM owner_migrations.compatibility WHERE singleton;
  LOCK TABLE public.schema_migrations IN ROW EXCLUSIVE MODE;
  PERFORM FROM public.schema_migrations FOR UPDATE;
  IF current_user<>'justix_'||b.owner_service OR current_database()<>b.database_name
    OR (SELECT count(*)<>1 OR NOT coalesce(bool_and(version=NEW.migration_version AND dirty),false) FROM public.schema_migrations)
    OR NEW.migration_version<=(SELECT max(version) FROM owner_migrations.artifacts) THEN
    RAISE EXCEPTION 'feature marker requires exact owner and next dirty ledger' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END
$$;
CREATE FUNCTION owner_migrations.artifact_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE b owner_migrations.compatibility%ROWTYPE; prior owner_migrations.artifacts%ROWTYPE;
BEGIN
  SELECT * INTO STRICT b FROM owner_migrations.compatibility WHERE singleton;
  PERFORM FROM public.schema_migrations FOR UPDATE;
  IF current_user<>'justix_'||b.owner_service OR current_database()<>b.database_name THEN
    RAISE EXCEPTION 'artifact requires exact migration owner' USING ERRCODE='23514';
  END IF;
  IF NEW.version=1 THEN
    IF EXISTS (SELECT FROM owner_migrations.artifacts)
      OR (SELECT count(*)<>1 OR NOT coalesce(bool_and(version=1 AND NOT dirty),false) FROM public.schema_migrations)
      OR (NEW.filename,NEW.sql_sha256,NEW.installation_request_id,NEW.provenance_kind,NEW.provenance_ref)
        IS DISTINCT FROM ('migrations/0001_mechanics.up.sql',b.base_artifact_sha256,b.installation_request_id,b.baseline_kind,b.evidence_ref) THEN
      RAISE EXCEPTION 'baseline receipt must match explicit clean version 1 evidence' USING ERRCODE='23514';
    END IF;
  ELSE
    SELECT * INTO STRICT prior FROM owner_migrations.artifacts ORDER BY version DESC LIMIT 1;
    IF (NEW.predecessor_version,NEW.predecessor_sha256) IS DISTINCT FROM (prior.version,prior.sql_sha256)
      OR NEW.version<=prior.version
      OR (SELECT count(*)<>1 OR NOT coalesce(bool_and(version=NEW.version AND dirty),false) FROM public.schema_migrations) THEN
      RAISE EXCEPTION 'artifact requires exact predecessor and dirty target ledger' USING ERRCODE='23514';
    END IF;
  END IF;
  RETURN NEW;
END
$$;
CREATE TRIGGER compatibility_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON owner_migrations.compatibility
  FOR EACH STATEMENT EXECUTE FUNCTION owner_migrations.immutable();
CREATE TRIGGER artifacts_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON owner_migrations.artifacts
  FOR EACH STATEMENT EXECUTE FUNCTION owner_migrations.immutable();
CREATE TRIGGER feature_contracts_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON owner_migrations.feature_contracts
  FOR EACH STATEMENT EXECUTE FUNCTION owner_migrations.immutable();
CREATE TRIGGER artifacts_insert BEFORE INSERT ON owner_migrations.artifacts
  FOR EACH ROW EXECUTE FUNCTION owner_migrations.artifact_guard();
CREATE TRIGGER feature_contracts_insert BEFORE INSERT ON owner_migrations.feature_contracts
  FOR EACH ROW EXECUTE FUNCTION owner_migrations.feature_guard();
REVOKE ALL ON ALL TABLES IN SCHEMA owner_migrations FROM PUBLIC;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA owner_migrations FROM PUBLIC;
GRANT USAGE ON SCHEMA owner_migrations TO :"runtime_role";
GRANT SELECT ON ALL TABLES IN SCHEMA owner_migrations TO :"runtime_role";
INSERT INTO owner_migrations.compatibility(singleton,owner_service,database_name,runtime_role,
  format_revision,template_sha256,base_schema_sha256,base_version,base_artifact_sha256,baseline_kind,
  evidence_ref,approval_ref,backup_ref,stopped_runtimes_ref,installation_request_id)
VALUES(true,:'owner_service',current_database(),:'runtime_role',1,decode(:'template_sha256','hex'),
  decode(:'base_schema_sha256','hex'),1,decode(:'base_artifact_sha256','hex'),:'baseline_kind',
  :'evidence_ref',:'approval_ref',:'backup_ref',:'stopped_runtimes_ref',:'request_id');
INSERT INTO owner_migrations.artifacts(version,filename,sql_sha256,installation_request_id,
  artifact_manifest_sha256,installing_profile_sha256,provenance_kind,provenance_ref)
VALUES(1,'migrations/0001_mechanics.up.sql',decode(:'base_artifact_sha256','hex'),:'request_id',
  decode(:'base_manifest_sha256','hex'),decode(:'profile_sha256','hex'),:'baseline_kind',:'evidence_ref');
COMMIT;
