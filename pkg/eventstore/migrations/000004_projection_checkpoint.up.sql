\set ON_ERROR_STOP on
-- Forward owner-local storage only. Required psql variables: owner_service,
-- runtime_role, base_schema_sha256, prior_migration_sha256 (000002),
-- correction_migration_sha256 (000003), checkpoint_migration_sha256 (this file),
-- backup_ref, stopped_runtimes_ref, compatibility_ref. The runner checks files
-- and external evidence; SQL cannot hash its input or attest stopped processes.
BEGIN;
SET LOCAL search_path=pg_catalog;
SET LOCAL lock_timeout='5s';
SELECT set_config('justix.install_owner', :'owner_service', true);
SELECT set_config('justix.install_runtime', :'runtime_role', true);
SELECT set_config('justix.checkpoint_base_hash', :'base_schema_sha256', true);
SELECT set_config('justix.checkpoint_prior_hash', :'prior_migration_sha256', true);
SELECT set_config('justix.checkpoint_route_hash', :'correction_migration_sha256', true);
SELECT set_config('justix.checkpoint_hash', :'checkpoint_migration_sha256', true);
DO $prerequisite$
DECLARE r pg_roles%ROWTYPE; owner_oid oid;
BEGIN
  SELECT nspowner INTO owner_oid FROM pg_namespace WHERE nspname='eventstore';
  IF owner_oid IS NULL OR owner_oid<>current_user::regrole::oid THEN
    RAISE EXCEPTION 'existing eventstore migration owner required';
  END IF;
  IF to_regclass('eventstore.messaging_route_compatibility') IS NULL THEN
    RAISE EXCEPTION 'corrected route migration required';
  END IF;
  IF to_regclass('eventstore.projection_checkpoint_compatibility') IS NOT NULL THEN
    RAISE EXCEPTION 'checkpoint compatibility marker already exists';
  END IF;
  IF (SELECT count(*) FROM pg_proc WHERE pronamespace='eventstore'::regnamespace
    AND proname IN ('messaging_immutable','messaging_delivery_guard','messaging_job_guard','messaging_dispatch_route')
    AND pronargs=0 AND prorettype='trigger'::regtype AND proowner=owner_oid
    AND NOT prosecdef AND proconfig=ARRAY['search_path=pg_catalog'])<>4 THEN
    RAISE EXCEPTION 'incompatible prerequisite trigger function shape';
  END IF;
  IF current_setting('justix.checkpoint_base_hash')<>'86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53'
    OR current_setting('justix.checkpoint_prior_hash')<>'1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2'
    OR current_setting('justix.checkpoint_route_hash')<>'c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec'
    OR current_setting('justix.checkpoint_hash') !~ '^[0-9a-f]{64}$' THEN
    RAISE EXCEPTION 'incorrect migration artifact digest';
  END IF;
  SELECT * INTO r FROM pg_roles WHERE rolname=current_setting('justix.install_runtime');
  IF NOT FOUND THEN RAISE EXCEPTION 'runtime role must already exist'; END IF;
  IF r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls
    OR pg_has_role(r.oid,owner_oid,'MEMBER')
    OR pg_has_role(r.oid,(SELECT datdba FROM pg_database WHERE datname=current_database()),'MEMBER')
    OR EXISTS (SELECT FROM pg_roles p WHERE p.oid<>r.oid AND pg_has_role(r.oid,p.oid,'MEMBER')
      AND (p.rolname LIKE 'pg\_%' ESCAPE '\' OR p.rolsuper OR p.rolcreatedb OR p.rolcreaterole
        OR p.rolreplication OR p.rolbypassrls)) THEN
    RAISE EXCEPTION 'runtime has privileged ownership or membership';
  END IF;
  IF EXISTS (SELECT FROM pg_default_acl d CROSS JOIN LATERAL aclexplode(d.defaclacl) a
    WHERE d.defaclrole=current_user::regrole::oid AND d.defaclnamespace IN (0,'eventstore'::regnamespace::oid)
      AND ((a.grantee=0 AND d.defaclobjtype IN ('r','f')) OR
        (a.grantee<>0 AND pg_has_role(r.oid,a.grantee,'MEMBER')
          AND ((d.defaclobjtype='r' AND (a.privilege_type<>'SELECT' OR a.is_grantable)) OR d.defaclobjtype='f')))) THEN
    RAISE EXCEPTION 'unexpected runtime or PUBLIC default privileges';
  END IF;
END
$prerequisite$;

LOCK TABLE eventstore.outbox, eventstore.inbox, eventstore.messaging_mode,
  eventstore.outbox_messages, eventstore.outbox_deliveries,
  eventstore.dispatch_messages, eventstore.dispatch_enrollments,
  eventstore.dispatch_jobs IN ACCESS EXCLUSIVE MODE;
-- Remaining existing prerequisites follow in stable name order. No old row,
-- function, trigger, ACL or base-mode identity is rewritten.
LOCK TABLE eventstore.messaging_admissions, eventstore.messaging_route_compatibility IN ACCESS EXCLUSIVE MODE;
DO $lineage$
DECLARE m eventstore.messaging_mode%ROWTYPE; r eventstore.messaging_route_compatibility%ROWTYPE;
BEGIN
  SELECT * INTO STRICT m FROM eventstore.messaging_mode WHERE singleton FOR UPDATE;
  SELECT * INTO STRICT r FROM eventstore.messaging_route_compatibility WHERE singleton;
  IF (SELECT count(*) FROM eventstore.messaging_mode)<>1
    OR (SELECT count(*) FROM eventstore.messaging_route_compatibility)<>1
    OR m.schema_version<>2 OR m.mode NOT IN ('legacy','custody')
    OR m.owner_service<>current_setting('justix.install_owner')
    OR m.runtime_role::text<>current_setting('justix.install_runtime')
    OR (r.owner_service,r.runtime_role,r.base_schema_version,r.migration_revision,r.route_format_version)
      IS DISTINCT FROM (m.owner_service,m.runtime_role,2,3,1)
    OR r.prior_migration_sha256<>decode(current_setting('justix.checkpoint_prior_hash'),'hex')
    OR r.correction_migration_sha256<>decode(current_setting('justix.checkpoint_route_hash'),'hex') THEN
    RAISE EXCEPTION 'installed owner, runtime or artifact lineage mismatch';
  END IF;
  IF (SELECT pg_get_expr(adbin,adrelid) FROM pg_attrdef d JOIN pg_attribute a
      ON a.attrelid=d.adrelid AND a.attnum=d.adnum
      WHERE d.adrelid='eventstore.events'::regclass AND a.attname='owner_service')
      IS DISTINCT FROM quote_literal(m.owner_service)||'::text' THEN
    RAISE EXCEPTION 'eventstore owner default mismatch';
  END IF;
END
$lineage$;

CREATE TABLE eventstore.projection_checkpoint_compatibility (
  singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
  owner_service text NOT NULL CHECK(owner_service=:'owner_service'),
  runtime_role name NOT NULL CHECK(runtime_role=:'runtime_role'::name),
  base_schema_version integer NOT NULL CHECK(base_schema_version=2),
  migration_revision integer NOT NULL CHECK(migration_revision=4),
  feature_format_version integer NOT NULL CHECK(feature_format_version=1),
  base_schema_sha256 bytea NOT NULL CHECK(base_schema_sha256=decode('86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53','hex')),
  prior_migration_sha256 bytea NOT NULL CHECK(prior_migration_sha256=decode('1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2','hex')),
  correction_migration_sha256 bytea NOT NULL CHECK(correction_migration_sha256=decode('c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec','hex')),
  checkpoint_migration_sha256 bytea NOT NULL CHECK(octet_length(checkpoint_migration_sha256)=32),
  backup_ref text NOT NULL CHECK(length(btrim(backup_ref))>0),
  stopped_runtimes_ref text NOT NULL CHECK(length(btrim(stopped_runtimes_ref))>0),
  compatibility_ref text NOT NULL CHECK(length(btrim(compatibility_ref))>0),
  installed_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(installed_at))
);
INSERT INTO eventstore.projection_checkpoint_compatibility(singleton,owner_service,runtime_role,
  base_schema_version,migration_revision,feature_format_version,base_schema_sha256,
  prior_migration_sha256,correction_migration_sha256,checkpoint_migration_sha256,
  backup_ref,stopped_runtimes_ref,compatibility_ref)
VALUES(true,:'owner_service',:'runtime_role',2,4,1,decode(:'base_schema_sha256','hex'),
  decode(:'prior_migration_sha256','hex'),decode(:'correction_migration_sha256','hex'),
  decode(:'checkpoint_migration_sha256','hex'),:'backup_ref',:'stopped_runtimes_ref',:'compatibility_ref');

CREATE TABLE eventstore.consumer_bootstraps (
  bootstrap_id uuid PRIMARY KEY CHECK(bootstrap_id<>'00000000-0000-0000-0000-000000000000'),
  consumer_name text NOT NULL CHECK(length(btrim(consumer_name))>0),
  source_owner text NOT NULL CHECK(source_owner IN ('identity','inventory','commerce','retail','financing','insurance','documents')),
  aggregate_type text NOT NULL CHECK(aggregate_type ~ '^[a-z][a-z0-9-]*$'),
  aggregate_id uuid NOT NULL CHECK(aggregate_id<>'00000000-0000-0000-0000-000000000000'),
  consumer_kind text NOT NULL CHECK(consumer_kind IN ('projection','process')),
  generation text NOT NULL CHECK(length(btrim(generation))>0),
  admission_id uuid REFERENCES eventstore.messaging_admissions(admission_id) CHECK(admission_id<>'00000000-0000-0000-0000-000000000000'),
  contract_id text NOT NULL CHECK(length(btrim(contract_id))>0),
  contract_version bigint NOT NULL CHECK(contract_version BETWEEN 1 AND 4294967295),
  contract_digest bytea NOT NULL CHECK(octet_length(contract_digest)=32),
  authority_ref text NOT NULL CHECK(length(btrim(authority_ref))>0),
  scope_ref text NOT NULL CHECK(length(btrim(scope_ref))>0),
  purpose_ref text NOT NULL CHECK(length(btrim(purpose_ref))>0),
  source_checkpoint_ref text NOT NULL CHECK(length(btrim(source_checkpoint_ref))>0),
  start_after bigint NOT NULL CHECK(start_after>=0),
  snapshot_manifest_ref text CHECK(length(btrim(snapshot_manifest_ref))>0),
  snapshot_manifest_hash bytea CHECK(octet_length(snapshot_manifest_hash)=32),
  no_snapshot_contract_ref text CHECK(length(btrim(no_snapshot_contract_ref))>0),
  new_empty_proof_ref text CHECK(length(btrim(new_empty_proof_ref))>0),
  installation_request_id uuid NOT NULL UNIQUE CHECK(installation_request_id<>'00000000-0000-0000-0000-000000000000'),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
  UNIQUE(consumer_name,source_owner,aggregate_type,aggregate_id),
  UNIQUE(bootstrap_id,consumer_name,source_owner,aggregate_type,aggregate_id,consumer_kind,generation),
  CHECK((snapshot_manifest_ref IS NOT NULL AND snapshot_manifest_hash IS NOT NULL AND no_snapshot_contract_ref IS NULL)
    OR (snapshot_manifest_ref IS NULL AND snapshot_manifest_hash IS NULL AND no_snapshot_contract_ref IS NOT NULL)),
  CHECK((start_after=0)=(new_empty_proof_ref IS NOT NULL))
);

CREATE TABLE eventstore.consumer_checkpoints (
  consumer_name text NOT NULL,
  source_owner text NOT NULL,
  aggregate_type text NOT NULL,
  aggregate_id uuid NOT NULL,
  bootstrap_id uuid NOT NULL,
  consumer_kind text NOT NULL,
  generation text NOT NULL,
  position bigint NOT NULL CHECK(position>=0),
  last_event_id uuid CHECK(last_event_id<>'00000000-0000-0000-0000-000000000000'),
  last_event_hash bytea CHECK(octet_length(last_event_hash)=32),
  revision bigint NOT NULL CHECK(revision>=0),
  updated_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
  PRIMARY KEY(consumer_name,source_owner,aggregate_type,aggregate_id),
  FOREIGN KEY(bootstrap_id,consumer_name,source_owner,aggregate_type,aggregate_id,consumer_kind,generation)
    REFERENCES eventstore.consumer_bootstraps(bootstrap_id,consumer_name,source_owner,aggregate_type,aggregate_id,consumer_kind,generation),
  CHECK((last_event_id IS NULL)=(last_event_hash IS NULL)),
  CHECK((revision=0)=(last_event_id IS NULL))
);

CREATE TABLE eventstore.consumer_gaps (
  gap_id uuid PRIMARY KEY CHECK(gap_id<>'00000000-0000-0000-0000-000000000000'),
  consumer_name text NOT NULL,
  source_owner text NOT NULL,
  aggregate_type text NOT NULL,
  aggregate_id uuid NOT NULL,
  bootstrap_id uuid NOT NULL REFERENCES eventstore.consumer_bootstraps(bootstrap_id),
  blocked_event_id uuid NOT NULL CHECK(blocked_event_id<>'00000000-0000-0000-0000-000000000000'),
  blocked_event_hash bytea NOT NULL CHECK(octet_length(blocked_event_hash)=32),
  expected_position bigint NOT NULL CHECK(expected_position>0),
  observed_position bigint NOT NULL CHECK(observed_position>expected_position),
  admission_ref text NOT NULL CHECK(length(btrim(admission_ref))>0),
  authority_ref text NOT NULL CHECK(length(btrim(authority_ref))>0),
  request_id uuid NOT NULL UNIQUE CHECK(request_id<>'00000000-0000-0000-0000-000000000000'),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
  FOREIGN KEY(consumer_name,source_owner,aggregate_type,aggregate_id)
    REFERENCES eventstore.consumer_checkpoints(consumer_name,source_owner,aggregate_type,aggregate_id),
  UNIQUE(consumer_name,source_owner,aggregate_type,aggregate_id,blocked_event_id)
);

CREATE TABLE eventstore.consumer_gap_attempts (
  attempt_id uuid PRIMARY KEY CHECK(attempt_id<>'00000000-0000-0000-0000-000000000000'),
  gap_id uuid NOT NULL REFERENCES eventstore.consumer_gaps(gap_id),
  prior_attempt_id uuid UNIQUE REFERENCES eventstore.consumer_gap_attempts(attempt_id),
  request_id uuid NOT NULL UNIQUE CHECK(request_id<>'00000000-0000-0000-0000-000000000000'),
  action text NOT NULL CHECK(action IN ('requested','recovered','held','resumed','resolved')),
  evidence_format_version integer NOT NULL CHECK(evidence_format_version=1),
  interval_from bigint NOT NULL CHECK(interval_from>0),
  interval_through bigint NOT NULL CHECK(interval_through>=interval_from),
  recovery_manifest_ref text CHECK(length(btrim(recovery_manifest_ref))>0),
  recovery_manifest_hash bytea CHECK(octet_length(recovery_manifest_hash)=32),
  hold_reason_code text CHECK(hold_reason_code ~ '^[a-z][a-z0-9-]{0,63}$'),
  authority_ref text NOT NULL CHECK(length(btrim(authority_ref))>0),
  resulting_position bigint NOT NULL CHECK(resulting_position>=0),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
  CHECK(prior_attempt_id IS NULL OR prior_attempt_id<>attempt_id),
  CHECK((recovery_manifest_ref IS NULL)=(recovery_manifest_hash IS NULL)),
  CHECK((action='recovered')=(recovery_manifest_ref IS NOT NULL)),
  CHECK((action='held')=(hold_reason_code IS NOT NULL))
);
CREATE UNIQUE INDEX consumer_gap_attempts_one_root ON eventstore.consumer_gap_attempts(gap_id) WHERE prior_attempt_id IS NULL;

CREATE FUNCTION eventstore.consumer_bootstrap_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE a eventstore.messaging_admissions%ROWTYPE; current_mode text;
BEGIN
  SELECT mode INTO STRICT current_mode FROM eventstore.messaging_mode WHERE singleton;
  IF current_mode='custody' AND NEW.admission_id IS NULL THEN
    RAISE EXCEPTION 'custody bootstrap requires root local admission' USING ERRCODE='23514';
  END IF;
  IF NEW.admission_id IS NOT NULL THEN
    SELECT * INTO STRICT a FROM eventstore.messaging_admissions WHERE admission_id=NEW.admission_id;
    IF a.namespace<>'local-consumer' OR a.action<>'admit'
      OR (a.subject,a.source_owner,a.aggregate_type,a.aggregate_id,a.consumer_kind,a.generation,a.start_after,
          a.contract_id,a.contract_version,a.contract_digest,a.authority_ref,a.scope_ref,a.purpose)
        IS DISTINCT FROM (NEW.consumer_name,NEW.source_owner,NEW.aggregate_type,NEW.aggregate_id,NEW.consumer_kind,NEW.generation,NEW.start_after,
          NEW.contract_id,NEW.contract_version,NEW.contract_digest,NEW.authority_ref,NEW.scope_ref,NEW.purpose_ref) THEN
      RAISE EXCEPTION 'bootstrap admission identity mismatch' USING ERRCODE='23514';
    END IF;
  END IF;
  -- This shape check cannot validate source authority, snapshot contents or
  -- admission's current chain. The typed adapter revalidates those under fences.
  RETURN NEW;
END
$$;

CREATE FUNCTION eventstore.consumer_checkpoint_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE b eventstore.consumer_bootstraps%ROWTYPE;
BEGIN
  SELECT * INTO STRICT b FROM eventstore.consumer_bootstraps WHERE bootstrap_id=NEW.bootstrap_id;
  IF TG_OP='INSERT' THEN
    IF NEW.position<>b.start_after OR NEW.revision<>0 OR NEW.last_event_id IS NOT NULL OR NEW.last_event_hash IS NOT NULL THEN
      RAISE EXCEPTION 'checkpoint must initialize at exact bootstrap' USING ERRCODE='23514';
    END IF;
  ELSE
    IF (NEW.consumer_name,NEW.source_owner,NEW.aggregate_type,NEW.aggregate_id,NEW.bootstrap_id,NEW.consumer_kind,NEW.generation)
      IS DISTINCT FROM (OLD.consumer_name,OLD.source_owner,OLD.aggregate_type,OLD.aggregate_id,OLD.bootstrap_id,OLD.consumer_kind,OLD.generation)
      OR NEW.position::numeric<>OLD.position::numeric+1 OR NEW.revision::numeric<>OLD.revision::numeric+1
      OR NEW.last_event_id IS NULL OR NEW.last_event_hash IS NULL OR NEW.last_event_id=OLD.last_event_id THEN
      RAISE EXCEPTION 'checkpoint identity or contiguous increment violated' USING ERRCODE='23514';
    END IF;
    IF NOT EXISTS (SELECT FROM eventstore.inbox i WHERE i.consumer_name=NEW.consumer_name
      AND i.event_id=NEW.last_event_id AND i.envelope_hash=NEW.last_event_hash) THEN
      RAISE EXCEPTION 'checkpoint requires matching inbox identity and hash' USING ERRCODE='23514';
    END IF;
  END IF;
  RETURN NEW;
END
$$;

CREATE FUNCTION eventstore.consumer_gap_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE c eventstore.consumer_checkpoints%ROWTYPE; b eventstore.consumer_bootstraps%ROWTYPE;
BEGIN
  SELECT * INTO STRICT c FROM eventstore.consumer_checkpoints WHERE
    (consumer_name,source_owner,aggregate_type,aggregate_id)=(NEW.consumer_name,NEW.source_owner,NEW.aggregate_type,NEW.aggregate_id);
  SELECT * INTO STRICT b FROM eventstore.consumer_bootstraps WHERE bootstrap_id=c.bootstrap_id;
  IF NEW.bootstrap_id<>c.bootstrap_id OR NEW.expected_position<=b.start_after
    OR NEW.expected_position::numeric>c.position::numeric+1 THEN
    RAISE EXCEPTION 'gap does not match checkpoint bootstrap or observed progress' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END
$$;

CREATE FUNCTION eventstore.consumer_gap_attempt_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE g eventstore.consumer_gaps%ROWTYPE; p eventstore.consumer_gap_attempts%ROWTYPE; current_position bigint;
BEGIN
  SELECT * INTO STRICT g FROM eventstore.consumer_gaps WHERE gap_id=NEW.gap_id;
  SELECT position INTO STRICT current_position FROM eventstore.consumer_checkpoints WHERE
    (consumer_name,source_owner,aggregate_type,aggregate_id)=(g.consumer_name,g.source_owner,g.aggregate_type,g.aggregate_id);
  IF NEW.prior_attempt_id IS NOT NULL THEN
    SELECT * INTO STRICT p FROM eventstore.consumer_gap_attempts WHERE attempt_id=NEW.prior_attempt_id;
    IF p.gap_id<>NEW.gap_id OR p.action='resolved' OR NEW.resulting_position<p.resulting_position THEN
      RAISE EXCEPTION 'gap attempt parent or progress conflict' USING ERRCODE='23514';
    END IF;
  END IF;
  IF NEW.interval_from<g.expected_position OR NEW.interval_through>=g.observed_position
    OR NEW.resulting_position<>current_position
    OR (NEW.action='resolved' AND current_position<g.observed_position-1)
    OR (NEW.action='held' AND current_position>=g.observed_position-1) THEN
    RAISE EXCEPTION 'gap attempt interval or authoritative checkpoint conflict' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END
$$;

CREATE TRIGGER bootstrap_guard BEFORE INSERT ON eventstore.consumer_bootstraps FOR EACH ROW EXECUTE FUNCTION eventstore.consumer_bootstrap_guard();
CREATE TRIGGER checkpoint_guard BEFORE INSERT OR UPDATE ON eventstore.consumer_checkpoints FOR EACH ROW EXECUTE FUNCTION eventstore.consumer_checkpoint_guard();
CREATE TRIGGER gap_guard BEFORE INSERT ON eventstore.consumer_gaps FOR EACH ROW EXECUTE FUNCTION eventstore.consumer_gap_guard();
CREATE TRIGGER gap_attempt_guard BEFORE INSERT ON eventstore.consumer_gap_attempts FOR EACH ROW EXECUTE FUNCTION eventstore.consumer_gap_attempt_guard();
DO $immutability$
DECLARE tab text;
BEGIN
  FOREACH tab IN ARRAY ARRAY['consumer_bootstraps','consumer_gaps','consumer_gap_attempts','projection_checkpoint_compatibility'] LOOP
    EXECUTE format('CREATE TRIGGER immutable_content BEFORE UPDATE OR DELETE ON eventstore.%I FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_immutable()',tab);
  END LOOP;
END
$immutability$;
CREATE TRIGGER immutable_checkpoint_delete BEFORE DELETE ON eventstore.consumer_checkpoints FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_immutable();
REVOKE ALL ON eventstore.consumer_bootstraps,eventstore.consumer_checkpoints,eventstore.consumer_gaps,
  eventstore.consumer_gap_attempts,eventstore.projection_checkpoint_compatibility FROM PUBLIC;
GRANT SELECT ON eventstore.projection_checkpoint_compatibility TO :"runtime_role";
GRANT SELECT,INSERT ON eventstore.consumer_bootstraps,eventstore.consumer_checkpoints,eventstore.consumer_gaps,eventstore.consumer_gap_attempts TO :"runtime_role";
GRANT UPDATE(position,last_event_id,last_event_hash,revision,updated_at) ON eventstore.consumer_checkpoints TO :"runtime_role";
REVOKE ALL ON FUNCTION eventstore.consumer_bootstrap_guard(),eventstore.consumer_checkpoint_guard(),
  eventstore.consumer_gap_guard(),eventstore.consumer_gap_attempt_guard() FROM PUBLIC;

-- Audit old and new objects without rewriting their existing ACLs.
DO $audit$
DECLARE r record; tab text; col record; allowed text[];
BEGIN
  FOREACH tab IN ARRAY ARRAY['inbox','messaging_admissions','messaging_mode','messaging_route_compatibility',
    'consumer_bootstraps','consumer_checkpoints','consumer_gaps','consumer_gap_attempts','projection_checkpoint_compatibility'] LOOP
    IF NOT has_table_privilege(current_setting('justix.install_runtime'),'eventstore.'||tab,'SELECT') THEN
      RAISE EXCEPTION 'missing required runtime SELECT on %',tab;
    END IF;
  END LOOP;
  FOR r IN SELECT oid FROM pg_roles WHERE pg_has_role(current_setting('justix.install_runtime'),oid,'MEMBER') LOOP
    IF has_database_privilege(r.oid,current_database(),'CREATE,TEMP') OR has_schema_privilege(r.oid,'eventstore','CREATE') THEN RAISE EXCEPTION 'runtime schema CREATE'; END IF;
    FOREACH tab IN ARRAY ARRAY['events','command_receipts','outbox','inbox','operations','operation_steps',
      'messaging_route_compatibility','messaging_mode','messaging_admissions','messaging_legacy_authorizations',
      'messaging_cutovers','messaging_legacy_evidence','outbox_messages','outbox_deliveries',
      'dispatch_messages','dispatch_enrollments','dispatch_jobs','consumer_bootstraps','consumer_checkpoints',
      'consumer_gaps','consumer_gap_attempts','projection_checkpoint_compatibility'] LOOP
      IF has_table_privilege(r.oid,'eventstore.'||tab,'UPDATE,DELETE,TRUNCATE,TRIGGER,REFERENCES,MAINTAIN') THEN
        RAISE EXCEPTION 'unexpected messaging table privileges on %',tab;
      END IF;
      IF tab IN ('projection_checkpoint_compatibility','messaging_route_compatibility','messaging_mode','messaging_legacy_authorizations','messaging_cutovers','messaging_legacy_evidence')
        AND has_any_column_privilege(r.oid,'eventstore.'||tab,'INSERT') THEN
        RAISE EXCEPTION 'unexpected migration evidence INSERT';
      END IF;
      IF tab='outbox' AND (SELECT mode FROM eventstore.messaging_mode WHERE singleton)='custody'
        AND (has_any_column_privilege(r.oid,'eventstore.outbox','INSERT,UPDATE')) THEN
        RAISE EXCEPTION 'custody legacy runtime grants remain writable';
      END IF;
      allowed:=CASE tab
        WHEN 'consumer_checkpoints' THEN ARRAY['position','last_event_id','last_event_hash','revision','updated_at']
        WHEN 'outbox' THEN ARRAY['attempts','next_attempt_at','lease_owner','lease_until','sent_at']
        WHEN 'operations' THEN ARRAY['phase','decision','attention_required','result','error','attempts','next_attempt_at','lease_owner','lease_until','revision','updated_at']
        WHEN 'outbox_deliveries' THEN ARRAY['attempts','next_attempt_at','lease_owner','lease_until','sent_at','hold_ref']
        WHEN 'dispatch_jobs' THEN ARRAY['attempts','next_attempt_at','lease_owner','lease_until','completed_at','inbox_consumer_name','inbox_event_id','hold_ref','quarantine_ref']
        ELSE ARRAY[]::text[] END;
      FOR col IN SELECT attname FROM pg_attribute WHERE attrelid=('eventstore.'||tab)::regclass AND attnum>0 AND NOT attisdropped LOOP
        IF (NOT col.attname=ANY(allowed) AND has_column_privilege(r.oid,'eventstore.'||tab,col.attname,'UPDATE'))
          OR has_column_privilege(r.oid,'eventstore.'||tab,col.attname,'REFERENCES') THEN
          RAISE EXCEPTION 'unexpected messaging column privileges';
        END IF;
      END LOOP;
    END LOOP;
    IF EXISTS (SELECT FROM pg_proc p WHERE p.pronamespace='eventstore'::regnamespace
      AND has_function_privilege(r.oid,p.oid,'EXECUTE')) THEN
      RAISE EXCEPTION 'runtime may activate migration';
    END IF;
  END LOOP;
  IF EXISTS (SELECT FROM pg_class c CROSS JOIN LATERAL aclexplode(c.relacl) a
    WHERE c.relnamespace='eventstore'::regnamespace AND
      (a.grantee=0 OR (a.is_grantable AND a.grantee<>0
        AND pg_has_role(current_setting('justix.install_runtime'),a.grantee,'MEMBER'))))
    OR EXISTS (SELECT FROM pg_attribute attribute_acl JOIN pg_class c ON c.oid=attribute_acl.attrelid
      CROSS JOIN LATERAL aclexplode(attribute_acl.attacl) a
      WHERE c.relnamespace='eventstore'::regnamespace AND
        (a.grantee=0 OR (a.is_grantable AND a.grantee<>0
          AND pg_has_role(current_setting('justix.install_runtime'),a.grantee,'MEMBER')))) THEN
    RAISE EXCEPTION 'unexpected PUBLIC or delegable eventstore privileges';
  END IF;
  IF EXISTS (SELECT FROM pg_namespace n CROSS JOIN LATERAL aclexplode(n.nspacl) a
    WHERE n.nspname='eventstore' AND (a.grantee=0 OR (a.is_grantable AND a.grantee<>0
      AND pg_has_role(current_setting('justix.install_runtime'),a.grantee,'MEMBER')))) THEN
    RAISE EXCEPTION 'unexpected PUBLIC or delegable schema privileges';
  END IF;
END
$audit$;
COMMIT;
