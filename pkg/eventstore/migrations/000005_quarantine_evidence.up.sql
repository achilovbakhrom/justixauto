\set ON_ERROR_STOP on
-- Forward owner-local storage only. Required psql variables: owner_service,
-- runtime_role, base_schema_sha256, prior_migration_sha256 (000002),
-- correction_migration_sha256 (000003), checkpoint_migration_sha256 (000004), quarantine_migration_sha256 (this file),
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
SELECT set_config('justix.quarantine_hash', :'quarantine_migration_sha256', true);
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
  IF to_regclass('eventstore.projection_checkpoint_compatibility') IS NULL THEN
    RAISE EXCEPTION 'checkpoint migration required';
  END IF;
  IF to_regclass('eventstore.quarantine_compatibility') IS NOT NULL THEN
    RAISE EXCEPTION 'quarantine compatibility marker already exists';
  END IF;
  IF (SELECT count(*) FROM pg_proc WHERE pronamespace='eventstore'::regnamespace
    AND proname IN ('messaging_immutable','messaging_delivery_guard','messaging_job_guard','messaging_dispatch_route',
      'consumer_bootstrap_guard','consumer_checkpoint_guard','consumer_gap_guard','consumer_gap_attempt_guard')
    AND pronargs=0 AND prorettype='trigger'::regtype AND proowner=owner_oid
    AND NOT prosecdef AND proconfig=ARRAY['search_path=pg_catalog'])<>8 THEN
    RAISE EXCEPTION 'incompatible prerequisite trigger function shape';
  END IF;
  IF current_setting('justix.checkpoint_base_hash')<>'86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53'
    OR current_setting('justix.checkpoint_prior_hash')<>'1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2'
    OR current_setting('justix.checkpoint_route_hash')<>'c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec'
    OR current_setting('justix.checkpoint_hash') <> '41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c'
    OR current_setting('justix.quarantine_hash') !~ '^[0-9a-f]{64}$' THEN
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
LOCK TABLE eventstore.consumer_bootstraps, eventstore.consumer_checkpoints,
  eventstore.consumer_gap_attempts, eventstore.consumer_gaps, eventstore.messaging_admissions,
  eventstore.messaging_route_compatibility, eventstore.projection_checkpoint_compatibility IN ACCESS EXCLUSIVE MODE;
DO $lineage$
DECLARE m eventstore.messaging_mode%ROWTYPE; r eventstore.messaging_route_compatibility%ROWTYPE;
  c eventstore.projection_checkpoint_compatibility%ROWTYPE;
BEGIN
  SELECT * INTO STRICT m FROM eventstore.messaging_mode WHERE singleton FOR UPDATE;
  SELECT * INTO STRICT r FROM eventstore.messaging_route_compatibility WHERE singleton;
  SELECT * INTO STRICT c FROM eventstore.projection_checkpoint_compatibility WHERE singleton;
  IF (SELECT count(*) FROM eventstore.messaging_mode)<>1
    OR (SELECT count(*) FROM eventstore.messaging_route_compatibility)<>1
    OR (SELECT count(*) FROM eventstore.projection_checkpoint_compatibility)<>1
    OR (c.owner_service,c.runtime_role,c.base_schema_version,c.migration_revision,c.feature_format_version)
      IS DISTINCT FROM (m.owner_service,m.runtime_role,2,4,1)
    OR c.base_schema_sha256<>decode(current_setting('justix.checkpoint_base_hash'),'hex')
    OR c.prior_migration_sha256<>decode(current_setting('justix.checkpoint_prior_hash'),'hex')
    OR c.correction_migration_sha256<>decode(current_setting('justix.checkpoint_route_hash'),'hex')
    OR c.checkpoint_migration_sha256<>decode(current_setting('justix.checkpoint_hash'),'hex')
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

CREATE TABLE eventstore.quarantine_compatibility (
  singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
  owner_service text NOT NULL CHECK(owner_service=:'owner_service'),
  runtime_role name NOT NULL CHECK(runtime_role=:'runtime_role'::name),
  base_schema_version integer NOT NULL CHECK(base_schema_version=2),
  migration_revision integer NOT NULL CHECK(migration_revision=5),
  feature_format_version integer NOT NULL CHECK(feature_format_version=1),
  base_schema_sha256 bytea NOT NULL CHECK(base_schema_sha256=decode('86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53','hex')),
  prior_migration_sha256 bytea NOT NULL CHECK(prior_migration_sha256=decode('1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2','hex')),
  correction_migration_sha256 bytea NOT NULL CHECK(correction_migration_sha256=decode('c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec','hex')),
  checkpoint_migration_sha256 bytea NOT NULL CHECK(checkpoint_migration_sha256=decode('41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c','hex')),
  quarantine_migration_sha256 bytea NOT NULL CHECK(octet_length(quarantine_migration_sha256)=32),
  backup_ref text NOT NULL CHECK(length(btrim(backup_ref))>0),
  stopped_runtimes_ref text NOT NULL CHECK(length(btrim(stopped_runtimes_ref))>0),
  compatibility_ref text NOT NULL CHECK(length(btrim(compatibility_ref))>0),
  installed_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(installed_at))
);
INSERT INTO eventstore.quarantine_compatibility(singleton,owner_service,runtime_role,
  base_schema_version,migration_revision,feature_format_version,base_schema_sha256,
  prior_migration_sha256,correction_migration_sha256,checkpoint_migration_sha256,quarantine_migration_sha256,
  backup_ref,stopped_runtimes_ref,compatibility_ref)
VALUES(true,:'owner_service',:'runtime_role',2,5,1,decode(:'base_schema_sha256','hex'),
  decode(:'prior_migration_sha256','hex'),decode(:'correction_migration_sha256','hex'),
  decode(:'checkpoint_migration_sha256','hex'),decode(:'quarantine_migration_sha256','hex'),:'backup_ref',:'stopped_runtimes_ref',:'compatibility_ref');

-- No raw body, parsed claimed identity or free-form error column is permitted.
-- The owner security port binds these fields before persistence. SQL checks
-- ciphertext integrity and shape, not cryptography, retention or authorization.
CREATE TABLE eventstore.quarantine_evidence (
  evidence_id uuid PRIMARY KEY CHECK(evidence_id<>'00000000-0000-0000-0000-000000000000'),
  capture_request_id uuid NOT NULL UNIQUE CHECK(capture_request_id<>'00000000-0000-0000-0000-000000000000'),
  stage text NOT NULL CHECK(stage IN ('intake','direct-handler','custody-handler')),
  owner_service text NOT NULL CHECK(owner_service=:'owner_service'),
  queue_ref text CHECK(length(btrim(queue_ref))>0),
  consumer_name text CHECK(length(btrim(consumer_name))>0),
  source_owner text CHECK(source_owner IN ('identity','inventory','commerce','retail','financing','insurance','documents')),
  aggregate_type text CHECK(aggregate_type ~ '^[a-z][a-z0-9-]*$'),
  aggregate_id uuid CHECK(aggregate_id<>'00000000-0000-0000-0000-000000000000'),
  job_event_id uuid CHECK(job_event_id<>'00000000-0000-0000-0000-000000000000'),
  job_observation text CHECK(job_observation IN ('unfinished','completed')),
  raw_sha256 bytea NOT NULL CHECK(octet_length(raw_sha256)=32),
  raw_byte_length bigint NOT NULL CHECK(raw_byte_length>=0),
  sealed_ciphertext bytea NOT NULL CHECK(octet_length(sealed_ciphertext)>0),
  ciphertext_sha256 bytea NOT NULL CHECK(octet_length(ciphertext_sha256)=32 AND ciphertext_sha256=sha256(sealed_ciphertext)),
  seal_format_ref text NOT NULL CHECK(length(btrim(seal_format_ref))>0),
  seal_key_ref text NOT NULL CHECK(length(btrim(seal_key_ref))>0),
  capture_policy_ref text NOT NULL CHECK(length(btrim(capture_policy_ref))>0),
  reason_code text NOT NULL CHECK(length(reason_code) BETWEEN 1 AND 64 AND reason_code ~ '^[a-z]' AND reason_code !~ '[^a-z0-9-]'),
  capture_context_digest bytea NOT NULL CHECK(octet_length(capture_context_digest)=32),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
  CHECK(num_nonnulls(source_owner,aggregate_type,aggregate_id) IN (0,3)),
  CHECK(source_owner IS NULL OR consumer_name IS NOT NULL),
  CHECK(stage<>'intake' OR (queue_ref IS NOT NULL AND job_event_id IS NULL)),
  CHECK(stage<>'direct-handler' OR (consumer_name IS NOT NULL AND job_event_id IS NULL)),
  CHECK((stage='custody-handler')=(job_event_id IS NOT NULL)),
  CHECK((job_event_id IS NULL)=(job_observation IS NULL)),
  CHECK(stage<>'custody-handler' OR (consumer_name IS NOT NULL AND source_owner IS NOT NULL))
);

CREATE TABLE eventstore.quarantine_actions (
  action_id uuid PRIMARY KEY CHECK(action_id<>'00000000-0000-0000-0000-000000000000'),
  evidence_id uuid NOT NULL REFERENCES eventstore.quarantine_evidence(evidence_id),
  request_id uuid NOT NULL UNIQUE CHECK(request_id<>'00000000-0000-0000-0000-000000000000'),
  prior_action_id uuid UNIQUE REFERENCES eventstore.quarantine_actions(action_id),
  action text NOT NULL CHECK(action IN ('hold','redrive-request','redrive-result')),
  evidence_format_version integer NOT NULL CHECK(evidence_format_version=1),
  actor_ref text NOT NULL CHECK(length(btrim(actor_ref))>0),
  authority_ref text NOT NULL CHECK(length(btrim(authority_ref))>0),
  scope_ref text NOT NULL CHECK(length(btrim(scope_ref))>0),
  purpose_ref text NOT NULL CHECK(length(btrim(purpose_ref))>0),
  repair_ref text NOT NULL CHECK(length(btrim(repair_ref))>0),
  intended_path text NOT NULL CHECK(intended_path IN ('intake','direct-handler','custody-handler')),
  intended_consumer text CHECK(length(btrim(intended_consumer))>0),
  manifest_ref text NOT NULL CHECK(length(btrim(manifest_ref))>0),
  manifest_sha256 bytea NOT NULL CHECK(octet_length(manifest_sha256)=32),
  accepted_event_id uuid CHECK(accepted_event_id<>'00000000-0000-0000-0000-000000000000'),
  accepted_event_hash bytea CHECK(octet_length(accepted_event_hash)=32),
  receipt_kind text CHECK(receipt_kind IN ('inbox','custody','job')),
  receipt_consumer_name text CHECK(length(btrim(receipt_consumer_name))>0),
  receipt_event_id uuid CHECK(receipt_event_id<>'00000000-0000-0000-0000-000000000000'),
  outcome_code text NOT NULL CHECK(outcome_code IN ('held','requested','denied','failed','accepted','released')),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
  CHECK(prior_action_id IS NULL OR prior_action_id<>action_id),
  CHECK(intended_path='intake' OR intended_consumer IS NOT NULL),
  CHECK((accepted_event_id IS NULL)=(accepted_event_hash IS NULL)),
  CHECK((receipt_kind IS NULL)=(receipt_event_id IS NULL)),
  CHECK(receipt_event_id IS NULL OR receipt_event_id=accepted_event_id),
  CHECK(receipt_kind IS NOT NULL OR receipt_consumer_name IS NULL),
  CHECK(receipt_kind IS DISTINCT FROM 'custody' OR receipt_consumer_name IS NULL),
  CHECK(receipt_kind IS NULL OR receipt_kind='custody' OR receipt_consumer_name IS NOT NULL),
  CHECK((action='hold' AND outcome_code='held') OR (action='redrive-request' AND outcome_code='requested')
    OR (action='redrive-result' AND outcome_code IN ('denied','failed','accepted','released'))),
  CHECK((outcome_code='accepted')=(receipt_kind IS NOT NULL)),
  CHECK(outcome_code='accepted' OR accepted_event_id IS NULL),
  CHECK(outcome_code<>'released' OR intended_path='custody-handler')
);
CREATE UNIQUE INDEX quarantine_actions_one_root ON eventstore.quarantine_actions(evidence_id) WHERE prior_action_id IS NULL;

CREATE FUNCTION eventstore.quarantine_evidence_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE j eventstore.dispatch_jobs%ROWTYPE; m eventstore.dispatch_messages%ROWTYPE;
BEGIN
  IF NEW.stage='custody-handler' THEN
    -- The outer adapter takes the complete receiver/generation fences and job
    -- lock before this INSERT. No trigger acquires an earlier-order lock.
    SELECT * INTO STRICT j FROM eventstore.dispatch_jobs WHERE
      (consumer_name,event_id)=(NEW.consumer_name,NEW.job_event_id);
    SELECT * INTO STRICT m FROM eventstore.dispatch_messages WHERE event_id=j.event_id;
    IF (m.source_owner,m.aggregate_type,m.aggregate_id,m.envelope_hash,octet_length(m.envelope)::bigint)
      IS DISTINCT FROM (NEW.source_owner,NEW.aggregate_type,NEW.aggregate_id,NEW.raw_sha256,NEW.raw_byte_length)
      OR (NEW.job_observation='completed') IS DISTINCT FROM (j.completed_at IS NOT NULL) THEN
      RAISE EXCEPTION 'quarantine job context mismatch' USING ERRCODE='23514';
    END IF;
  END IF;
  RETURN NEW;
END
$$;

CREATE FUNCTION eventstore.quarantine_action_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE p eventstore.quarantine_actions%ROWTYPE; e eventstore.quarantine_evidence%ROWTYPE; matched boolean;
BEGIN
  SELECT * INTO STRICT e FROM eventstore.quarantine_evidence WHERE evidence_id=NEW.evidence_id;
  IF NEW.prior_action_id IS NOT NULL THEN
    SELECT * INTO STRICT p FROM eventstore.quarantine_actions WHERE action_id=NEW.prior_action_id;
    IF p.evidence_id<>NEW.evidence_id THEN
      RAISE EXCEPTION 'quarantine action parent mismatch' USING ERRCODE='23514';
    END IF;
  END IF;
  IF NEW.action='redrive-result' AND (NEW.prior_action_id IS NULL OR p.action<>'redrive-request'
    OR (NEW.intended_path,NEW.intended_consumer,NEW.actor_ref,NEW.authority_ref,NEW.scope_ref,NEW.purpose_ref,NEW.repair_ref)
      IS DISTINCT FROM (p.intended_path,p.intended_consumer,p.actor_ref,p.authority_ref,p.scope_ref,p.purpose_ref,p.repair_ref)) THEN
    RAISE EXCEPTION 'redrive result requires exact prior request context' USING ERRCODE='23514';
  END IF;
  IF e.stage='custody-handler' AND (NEW.intended_path<>'custody-handler' OR NEW.intended_consumer IS DISTINCT FROM e.consumer_name) THEN
    RAISE EXCEPTION 'redrive cannot target another custody obligation' USING ERRCODE='23514';
  END IF;
  IF NEW.outcome_code='accepted' THEN
    -- These rows are receipts, never inferred progress. In particular an inbox
    -- receipt cannot certify a missing checkpoint or an unfinished custody job.
    IF NEW.receipt_kind='inbox' THEN
      SELECT EXISTS(SELECT FROM eventstore.inbox WHERE (consumer_name,event_id,envelope_hash)=
        (NEW.receipt_consumer_name,NEW.receipt_event_id,NEW.accepted_event_hash)) INTO matched;
      IF NEW.intended_path<>'direct-handler' OR NEW.receipt_consumer_name IS DISTINCT FROM NEW.intended_consumer THEN matched:=false; END IF;
    ELSIF NEW.receipt_kind='custody' THEN
      SELECT EXISTS(SELECT FROM eventstore.dispatch_messages WHERE (event_id,envelope_hash)=
        (NEW.receipt_event_id,NEW.accepted_event_hash)) INTO matched;
      IF NEW.intended_path<>'intake' THEN matched:=false; END IF;
    ELSE
      SELECT EXISTS(SELECT FROM eventstore.dispatch_jobs j JOIN eventstore.inbox i
        ON (i.consumer_name,i.event_id)=(j.inbox_consumer_name,j.inbox_event_id)
        WHERE (j.consumer_name,j.event_id,i.envelope_hash)=(NEW.receipt_consumer_name,NEW.receipt_event_id,NEW.accepted_event_hash)
          AND j.completed_at IS NOT NULL) INTO matched;
      IF NEW.intended_path<>'custody-handler' OR NEW.receipt_consumer_name IS DISTINCT FROM NEW.intended_consumer
        OR (e.stage='custody-handler' AND NEW.receipt_event_id<>e.job_event_id) THEN matched:=false; END IF;
    END IF;
    IF NOT matched THEN RAISE EXCEPTION 'missing matching authoritative redrive receipt' USING ERRCODE='23514'; END IF;
  END IF;
  -- Release authorization, original-byte restoration, current holds, complete
  -- intake membership and the job reference update remain typed adapter duties.
  -- The manifest hash is supplied by that adapter; SQL cannot verify its content.
  RETURN NEW;
END
$$;
CREATE TRIGGER quarantine_evidence_guard BEFORE INSERT ON eventstore.quarantine_evidence FOR EACH ROW EXECUTE FUNCTION eventstore.quarantine_evidence_guard();
CREATE TRIGGER quarantine_action_guard BEFORE INSERT ON eventstore.quarantine_actions FOR EACH ROW EXECUTE FUNCTION eventstore.quarantine_action_guard();
DO $immutability$
DECLARE tab text;
BEGIN
  FOREACH tab IN ARRAY ARRAY['quarantine_evidence','quarantine_actions','quarantine_compatibility'] LOOP
    EXECUTE format('CREATE TRIGGER immutable_content BEFORE UPDATE OR DELETE ON eventstore.%I FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_immutable()',tab);
    EXECUTE format('CREATE TRIGGER immutable_truncate BEFORE TRUNCATE ON eventstore.%I FOR EACH STATEMENT EXECUTE FUNCTION eventstore.messaging_immutable()',tab);
  END LOOP;
END
$immutability$;
REVOKE ALL ON eventstore.quarantine_evidence,eventstore.quarantine_actions,eventstore.quarantine_compatibility FROM PUBLIC;
GRANT SELECT ON eventstore.quarantine_compatibility TO :"runtime_role";
GRANT SELECT,INSERT ON eventstore.quarantine_evidence,eventstore.quarantine_actions TO :"runtime_role";
REVOKE ALL ON FUNCTION eventstore.quarantine_evidence_guard(),eventstore.quarantine_action_guard() FROM PUBLIC;

-- Audit old and new objects without rewriting their existing ACLs.
DO $audit$
DECLARE r record; tab text; col record; allowed text[];
BEGIN
  IF NOT has_schema_privilege(current_setting('justix.install_runtime'),'eventstore','USAGE') THEN
    RAISE EXCEPTION 'missing required runtime schema USAGE';
  END IF;
  FOREACH tab IN ARRAY ARRAY['inbox','messaging_admissions','consumer_bootstraps','consumer_checkpoints',
    'consumer_gaps','consumer_gap_attempts','quarantine_evidence','quarantine_actions'] LOOP
    IF NOT has_table_privilege(current_setting('justix.install_runtime'),'eventstore.'||tab,'INSERT') THEN
      RAISE EXCEPTION 'missing required runtime INSERT on %',tab;
    END IF;
  END LOOP;
  FOREACH tab IN ARRAY ARRAY['inbox','dispatch_messages','dispatch_jobs','messaging_admissions','messaging_mode','messaging_route_compatibility',
    'consumer_bootstraps','consumer_checkpoints','consumer_gaps','consumer_gap_attempts','projection_checkpoint_compatibility',
    'quarantine_evidence','quarantine_actions','quarantine_compatibility'] LOOP
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
      'consumer_gaps','consumer_gap_attempts','projection_checkpoint_compatibility',
      'quarantine_evidence','quarantine_actions','quarantine_compatibility'] LOOP
      IF (SELECT relowner FROM pg_class WHERE oid=('eventstore.'||tab)::regclass)<>current_user::regrole::oid THEN
        RAISE EXCEPTION 'unexpected mechanics table owner on %',tab;
      END IF;
      IF has_table_privilege(r.oid,'eventstore.'||tab,'UPDATE,DELETE,TRUNCATE,TRIGGER,REFERENCES,MAINTAIN') THEN
        RAISE EXCEPTION 'unexpected messaging table privileges on %',tab;
      END IF;
      IF tab IN ('quarantine_compatibility','projection_checkpoint_compatibility','messaging_route_compatibility','messaging_mode','messaging_legacy_authorizations','messaging_cutovers','messaging_legacy_evidence')
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
