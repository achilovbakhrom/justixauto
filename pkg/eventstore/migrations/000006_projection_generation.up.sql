\set ON_ERROR_STOP on
-- Forward owner-local storage only. Required psql variables: owner_service,
-- runtime_role, base_schema_sha256, prior_migration_sha256 (000002),
-- correction_migration_sha256 (000003), checkpoint_migration_sha256 (000004), quarantine_migration_sha256 (000005),
-- generation_migration_sha256 (this file),
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
SELECT set_config('justix.generation_hash', :'generation_migration_sha256', true);
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
  IF to_regclass('eventstore.quarantine_compatibility') IS NULL THEN
    RAISE EXCEPTION 'quarantine migration required';
  END IF;
  IF to_regclass('eventstore.projection_generation_compatibility') IS NOT NULL THEN
    RAISE EXCEPTION 'generation compatibility marker already exists';
  END IF;
  IF (SELECT count(*) FROM pg_proc WHERE pronamespace='eventstore'::regnamespace
    AND proname IN ('messaging_immutable','messaging_delivery_guard','messaging_job_guard','messaging_dispatch_route',
      'consumer_bootstrap_guard','consumer_checkpoint_guard','consumer_gap_guard','consumer_gap_attempt_guard','quarantine_evidence_guard','quarantine_action_guard')
    AND pronargs=0 AND prorettype='trigger'::regtype AND proowner=owner_oid
    AND NOT prosecdef AND proconfig=ARRAY['search_path=pg_catalog'])<>10 THEN
    RAISE EXCEPTION 'incompatible prerequisite trigger function shape';
  END IF;
  IF current_setting('justix.checkpoint_base_hash')<>'86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53'
    OR current_setting('justix.checkpoint_prior_hash')<>'1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2'
    OR current_setting('justix.checkpoint_route_hash')<>'c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec'
    OR current_setting('justix.checkpoint_hash') <> '41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c'
    OR current_setting('justix.quarantine_hash') <> '1c1e7d17efa8ff84f2469d6d55b3b6c810143e5260aabf5f82d748797f2296a3'
    OR current_setting('justix.generation_hash') !~ '^[0-9a-f]{64}$' THEN
    RAISE EXCEPTION 'incorrect migration artifact digest';
  END IF;
  SELECT * INTO r FROM pg_roles WHERE rolname=current_setting('justix.install_runtime');
  IF NOT FOUND THEN RAISE EXCEPTION 'runtime role must already exist'; END IF;
  -- The installer's effective setting cannot prove a different login's startup
  -- mode. Require a concrete administrator-provisioned role/database default:
  -- fresh direct runtime logins apply this ahead of server-wide defaults.
  -- This does not reset existing sessions or prove external quiescence. The
  -- reviewed installer must stop old sessions and runtime roots must connect
  -- directly as this LOGIN role, without a proxy role/session-auth substitution.
  IF NOT r.rolcanlogin OR (SELECT count(*) FROM pg_db_role_setting s
    CROSS JOIN LATERAL unnest(s.setconfig) setting
    WHERE s.setrole=r.oid AND s.setdatabase=(SELECT oid FROM pg_database WHERE datname=current_database())
      AND setting='session_replication_role=origin')<>1 THEN
    RAISE EXCEPTION 'explicit runtime LOGIN database origin setting required';
  END IF;
  -- Trigger/FK integrity also depends on parameter authority. PUBLIC and
  -- inherited access are included by has_parameter_privilege; MEMBER covers
  -- conservative SET ROLE reachability, including NOINHERIT chains.
  IF current_setting('session_replication_role')<>'origin'
    OR EXISTS (SELECT FROM pg_roles p WHERE pg_has_role(r.oid,p.oid,'MEMBER')
      AND (has_parameter_privilege(p.oid,'session_replication_role','SET')
        OR has_parameter_privilege(p.oid,'session_replication_role','ALTER SYSTEM')))
    OR EXISTS (SELECT FROM pg_db_role_setting s CROSS JOIN LATERAL unnest(s.setconfig) setting
      WHERE s.setdatabase IN (0,(SELECT oid FROM pg_database WHERE datname=current_database()))
        AND (s.setrole=0 OR s.setrole=r.oid OR pg_has_role(r.oid,s.setrole,'MEMBER'))
        AND split_part(setting,'=',1)='session_replication_role' AND split_part(setting,'=',2)<>'origin') THEN
    RAISE EXCEPTION 'unsafe session replication mode or runtime parameter authority';
  END IF;
  IF r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls
    OR pg_has_role(r.oid,owner_oid,'MEMBER')
    OR pg_has_role(r.oid,(SELECT datdba FROM pg_database WHERE datname=current_database()),'MEMBER')
    OR EXISTS (SELECT FROM pg_roles p WHERE p.oid<>r.oid AND pg_has_role(r.oid,p.oid,'MEMBER')
      AND (p.rolname LIKE 'pg\_%' ESCAPE '\' OR p.rolsuper OR p.rolcreatedb OR p.rolcreaterole
        OR p.rolreplication OR p.rolbypassrls)) THEN
    RAISE EXCEPTION 'runtime has privileged ownership or membership';
  END IF;
  -- A missing global pg_default_acl row means acldefault(), not an empty ACL.
  -- Schema defaults add to global defaults. Deny implicit PUBLIC function/type
  -- grants too: the reviewed installer must establish defaults before this script.
  IF EXISTS (
    WITH families(kind) AS (VALUES ('r'::"char"),('S'::"char"),('f'::"char"),('T'::"char"),('n'::"char"),('L'::"char")),
    effective AS (
      SELECT f.kind, a.* FROM families f
      LEFT JOIN pg_default_acl d ON d.defaclrole=owner_oid AND d.defaclnamespace=0 AND d.defaclobjtype=f.kind
      CROSS JOIN LATERAL aclexplode(coalesce(d.defaclacl,acldefault(f.kind,owner_oid))) a
      UNION ALL
      SELECT d.defaclobjtype,a.* FROM pg_default_acl d CROSS JOIN LATERAL aclexplode(d.defaclacl) a
      WHERE d.defaclrole=owner_oid AND d.defaclnamespace='eventstore'::regnamespace::oid
    ) SELECT FROM effective a WHERE a.grantee=0 OR
      (a.grantee<>owner_oid AND a.grantee<>0 AND pg_has_role(r.oid,a.grantee,'MEMBER'))
  ) THEN RAISE EXCEPTION 'unexpected effective runtime or PUBLIC default privileges'; END IF;
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
  eventstore.messaging_route_compatibility, eventstore.projection_checkpoint_compatibility,
  eventstore.quarantine_actions, eventstore.quarantine_compatibility, eventstore.quarantine_evidence IN ACCESS EXCLUSIVE MODE;
DO $lineage$
DECLARE m eventstore.messaging_mode%ROWTYPE; r eventstore.messaging_route_compatibility%ROWTYPE;
  c eventstore.projection_checkpoint_compatibility%ROWTYPE; q eventstore.quarantine_compatibility%ROWTYPE;
BEGIN
  SELECT * INTO STRICT m FROM eventstore.messaging_mode WHERE singleton FOR UPDATE;
  SELECT * INTO STRICT r FROM eventstore.messaging_route_compatibility WHERE singleton;
  SELECT * INTO STRICT c FROM eventstore.projection_checkpoint_compatibility WHERE singleton;
  SELECT * INTO STRICT q FROM eventstore.quarantine_compatibility WHERE singleton;
  IF (SELECT count(*) FROM eventstore.quarantine_compatibility)<>1
    OR (q.owner_service,q.runtime_role,q.base_schema_version,q.migration_revision,q.feature_format_version)
      IS DISTINCT FROM (m.owner_service,m.runtime_role,2,5,1)
    OR q.base_schema_sha256<>decode(current_setting('justix.checkpoint_base_hash'),'hex')
    OR q.prior_migration_sha256<>decode(current_setting('justix.checkpoint_prior_hash'),'hex')
    OR q.correction_migration_sha256<>decode(current_setting('justix.checkpoint_route_hash'),'hex')
    OR q.checkpoint_migration_sha256<>decode(current_setting('justix.checkpoint_hash'),'hex')
    OR q.quarantine_migration_sha256<>decode(current_setting('justix.quarantine_hash'),'hex')
    OR (SELECT count(*) FROM eventstore.messaging_mode)<>1
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

-- Replace only the exact retained revision-4 generation CHECK. All other
-- bootstrap identities, guards, labels, rows and ACLs remain untouched.
DO $label$
DECLARE a pg_attribute%ROWTYPE; c pg_constraint%ROWTYPE;
BEGIN
  SELECT * INTO STRICT a FROM pg_attribute WHERE attrelid='eventstore.consumer_bootstraps'::regclass
    AND attname='generation' AND attnum>0 AND NOT attisdropped;
  IF a.atttypid<>'text'::regtype OR NOT a.attnotnull OR a.attgenerated<>'' OR a.attidentity<>'' THEN
    RAISE EXCEPTION 'unexpected bootstrap generation column';
  END IF;
  IF NOT EXISTS (SELECT FROM pg_trigger WHERE tgrelid=a.attrelid AND tgname='bootstrap_guard'
    AND tgfoid='eventstore.consumer_bootstrap_guard()'::regprocedure AND tgtype=7 AND tgenabled='O'
    AND tgqual IS NULL AND tgnargs=0 AND NOT tgisinternal AND NOT tgdeferrable AND NOT tginitdeferred)
    OR NOT EXISTS (SELECT FROM pg_trigger WHERE tgrelid='eventstore.consumer_checkpoints'::regclass AND tgname='checkpoint_guard'
    AND tgfoid='eventstore.consumer_checkpoint_guard()'::regprocedure AND tgtype=23 AND tgenabled='O'
    AND tgqual IS NULL AND tgnargs=0 AND NOT tgisinternal AND NOT tgdeferrable AND NOT tginitdeferred) THEN
    RAISE EXCEPTION 'required unconditional bootstrap and checkpoint guards';
  END IF;
  IF (SELECT count(*) FROM pg_constraint WHERE conrelid=a.attrelid AND contype='c' AND a.attnum=ANY(conkey))<>1 THEN
    RAISE EXCEPTION 'single exact generation CHECK required';
  END IF;
  SELECT * INTO STRICT c FROM pg_constraint WHERE conrelid=a.attrelid AND contype='c' AND a.attnum=ANY(conkey);
  IF c.conname<>'consumer_bootstraps_generation_check' OR c.conkey<>ARRAY[a.attnum]
    OR NOT c.convalidated OR NOT c.conenforced OR NOT c.conislocal OR c.coninhcount<>0 OR c.connoinherit
    OR pg_get_expr(c.conbin,c.conrelid)<>'(length(btrim(generation)) > 0)' THEN
    RAISE EXCEPTION 'unexpected bootstrap generation CHECK';
  END IF;
END
$label$;
ALTER TABLE eventstore.consumer_bootstraps DROP CONSTRAINT consumer_bootstraps_generation_check;
ALTER TABLE eventstore.consumer_bootstraps ADD CONSTRAINT consumer_bootstraps_generation_check CHECK(length(generation)>0);

CREATE TABLE eventstore.projection_generation_compatibility (
  singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
  owner_service text NOT NULL CHECK(owner_service=:'owner_service'),
  runtime_role name NOT NULL CHECK(runtime_role=:'runtime_role'::name),
  base_schema_version integer NOT NULL CHECK(base_schema_version=2),
  migration_revision integer NOT NULL CHECK(migration_revision=6),
  feature_format_version integer NOT NULL CHECK(feature_format_version=1),
  base_schema_sha256 bytea NOT NULL CHECK(base_schema_sha256=decode('86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53','hex')),
  prior_migration_sha256 bytea NOT NULL CHECK(prior_migration_sha256=decode('1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2','hex')),
  correction_migration_sha256 bytea NOT NULL CHECK(correction_migration_sha256=decode('c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec','hex')),
  checkpoint_migration_sha256 bytea NOT NULL CHECK(checkpoint_migration_sha256=decode('41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c','hex')),
  quarantine_migration_sha256 bytea NOT NULL CHECK(quarantine_migration_sha256=decode('1c1e7d17efa8ff84f2469d6d55b3b6c810143e5260aabf5f82d748797f2296a3','hex')),
  generation_migration_sha256 bytea NOT NULL CHECK(octet_length(generation_migration_sha256)=32),
  backup_ref text NOT NULL CHECK(length(btrim(backup_ref))>0),
  stopped_runtimes_ref text NOT NULL CHECK(length(btrim(stopped_runtimes_ref))>0),
  compatibility_ref text NOT NULL CHECK(length(btrim(compatibility_ref))>0),
  installed_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(installed_at))
);
INSERT INTO eventstore.projection_generation_compatibility(singleton,owner_service,runtime_role,
  base_schema_version,migration_revision,feature_format_version,base_schema_sha256,
  prior_migration_sha256,correction_migration_sha256,checkpoint_migration_sha256,quarantine_migration_sha256,generation_migration_sha256,
  backup_ref,stopped_runtimes_ref,compatibility_ref)
VALUES(true,:'owner_service',:'runtime_role',2,6,1,decode(:'base_schema_sha256','hex'),
  decode(:'prior_migration_sha256','hex'),decode(:'correction_migration_sha256','hex'),
  decode(:'checkpoint_migration_sha256','hex'),decode(:'quarantine_migration_sha256','hex'),decode(:'generation_migration_sha256','hex'),:'backup_ref',:'stopped_runtimes_ref',:'compatibility_ref');


-- Manifest references/digests are checked, versioned evidence supplied by the
-- typed owner adapter. SQL neither fetches their contents nor invents a global
-- stream order, history authority, comparator result or business read rows.
CREATE TABLE eventstore.projection_generations (
  projection_name text NOT NULL CHECK(length(projection_name)>0),
  generation_id uuid NOT NULL CHECK(generation_id<>'00000000-0000-0000-0000-000000000000'),
  consumer_name text NOT NULL UNIQUE CHECK(length(consumer_name)>0),
  consumer_kind text NOT NULL CHECK(consumer_kind='projection'),
  handler_digest bytea NOT NULL CHECK(octet_length(handler_digest)=32),
  contract_id text NOT NULL CHECK(length(btrim(contract_id))>0),
  contract_version bigint NOT NULL CHECK(contract_version>0),
  contract_digest bytea NOT NULL CHECK(octet_length(contract_digest)=32),
  evidence_format_version integer NOT NULL CHECK(evidence_format_version=1),
  authority_ref text NOT NULL CHECK(length(btrim(authority_ref))>0),
  scope_ref text NOT NULL CHECK(length(btrim(scope_ref))>0),
  purpose_ref text NOT NULL CHECK(length(btrim(purpose_ref))>0),
  history_manifest_ref text NOT NULL CHECK(length(btrim(history_manifest_ref))>0),
  history_manifest_sha256 bytea NOT NULL CHECK(octet_length(history_manifest_sha256)=32),
  bootstrap_manifest_ref text NOT NULL CHECK(length(btrim(bootstrap_manifest_ref))>0),
  bootstrap_manifest_sha256 bytea NOT NULL CHECK(octet_length(bootstrap_manifest_sha256)=32),
  creation_request_id uuid NOT NULL UNIQUE CHECK(creation_request_id<>'00000000-0000-0000-0000-000000000000'),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
  PRIMARY KEY(projection_name,generation_id)
);
CREATE TABLE eventstore.projection_generation_events (
  event_id uuid PRIMARY KEY CHECK(event_id<>'00000000-0000-0000-0000-000000000000'),
  projection_name text NOT NULL,
  generation_id uuid NOT NULL,
  request_id uuid NOT NULL UNIQUE CHECK(request_id<>'00000000-0000-0000-0000-000000000000'),
  previous_event_id uuid UNIQUE REFERENCES eventstore.projection_generation_events(event_id),
  action text NOT NULL CHECK(action IN ('build-progress','validated','hold','abandon','switch')),
  evidence_format_version integer NOT NULL CHECK(evidence_format_version=1),
  source_vector_ref text NOT NULL CHECK(length(btrim(source_vector_ref))>0),
  source_vector_sha256 bytea NOT NULL CHECK(octet_length(source_vector_sha256)=32),
  cursor_manifest_ref text NOT NULL CHECK(length(btrim(cursor_manifest_ref))>0),
  cursor_manifest_sha256 bytea NOT NULL CHECK(octet_length(cursor_manifest_sha256)=32),
  comparator_id text CHECK(length(btrim(comparator_id))>0),
  comparator_version bigint CHECK(comparator_version>0),
  comparison_result text CHECK(comparison_result IN ('passed','failed')),
  comparison_ref text CHECK(length(btrim(comparison_ref))>0),
  comparison_sha256 bytea CHECK(octet_length(comparison_sha256)=32),
  expected_head_epoch bigint CHECK(expected_head_epoch>0),
  expected_active_generation_id uuid CHECK(expected_active_generation_id<>'00000000-0000-0000-0000-000000000000'),
  hold_ref text CHECK(length(btrim(hold_ref))>0),
  -- Bookkeeping assigned from the real top-level transaction by the INSERT
  -- trigger, never caller intent, comparator identity or authorization.
  created_xid xid8 NOT NULL,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
  FOREIGN KEY(projection_name,generation_id) REFERENCES eventstore.projection_generations(projection_name,generation_id),
  FOREIGN KEY(projection_name,expected_active_generation_id) REFERENCES eventstore.projection_generations(projection_name,generation_id),
  CHECK(previous_event_id IS NULL OR previous_event_id<>event_id),
  CHECK(num_nonnulls(comparator_id,comparator_version,comparison_result,comparison_ref,comparison_sha256) IN (0,5)),
  CHECK((action IN ('validated','switch'))=(comparator_id IS NOT NULL)),
  CHECK(action<>'switch' OR (comparison_result='passed' AND expected_head_epoch IS NOT NULL)),
  CHECK(action IN ('switch','hold') OR expected_head_epoch IS NULL),
  CHECK(expected_head_epoch IS NOT NULL OR expected_active_generation_id IS NULL),
  CHECK((action IN ('hold','abandon'))=(hold_ref IS NOT NULL))
);
CREATE UNIQUE INDEX projection_generation_events_one_root
  ON eventstore.projection_generation_events(projection_name,generation_id) WHERE previous_event_id IS NULL;
CREATE TABLE eventstore.projection_heads (
  projection_name text PRIMARY KEY CHECK(length(projection_name)>0),
  active_generation_id uuid,
  epoch bigint NOT NULL CHECK(epoch>0),
  hold_ref text CHECK(length(btrim(hold_ref))>0),
  last_event_id uuid REFERENCES eventstore.projection_generation_events(event_id),
  last_switch_event_id uuid REFERENCES eventstore.projection_generation_events(event_id),
  last_mutation_xid xid8,
  updated_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
  FOREIGN KEY(projection_name,active_generation_id) REFERENCES eventstore.projection_generations(projection_name,generation_id)
);

CREATE FUNCTION eventstore.projection_generation_event_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE p eventstore.projection_generation_events%ROWTYPE;
BEGIN
  NEW.created_xid:=pg_current_xact_id();
  IF NEW.action='hold' AND NEW.expected_head_epoch IS NULL AND EXISTS (
    SELECT FROM eventstore.projection_heads WHERE projection_name=NEW.projection_name
      AND active_generation_id=NEW.generation_id) THEN
    RAISE EXCEPTION 'active generation hold requires matching committed head change';
  END IF;
  -- No earlier-order lock is acquired here. The adapter holds the complete
  -- prepared catalog/stream/generation fences and expected chain-tip fence.
  IF NEW.previous_event_id IS NULL THEN
    IF NEW.action NOT IN ('build-progress','hold','abandon') THEN RAISE EXCEPTION 'generation root must describe build or hold'; END IF;
  ELSE
    SELECT * INTO STRICT p FROM eventstore.projection_generation_events WHERE event_id=NEW.previous_event_id;
    IF (p.projection_name,p.generation_id) IS DISTINCT FROM (NEW.projection_name,NEW.generation_id)
      OR p.action='abandon' THEN RAISE EXCEPTION 'foreign or abandoned generation predecessor'; END IF;
    IF NEW.action='validated' AND p.action<>'build-progress' THEN
      RAISE EXCEPTION 'validation requires a completed build observation';
    END IF;
    IF NEW.action IN ('validated','switch') AND
      (NEW.source_vector_ref,NEW.source_vector_sha256,NEW.cursor_manifest_ref,NEW.cursor_manifest_sha256)
      IS DISTINCT FROM (p.source_vector_ref,p.source_vector_sha256,p.cursor_manifest_ref,p.cursor_manifest_sha256) THEN
      RAISE EXCEPTION 'generation evidence vector or cursor changed';
    END IF;
    IF NEW.action='switch' AND (p.action<>'validated' OR p.comparison_result<>'passed'
      OR (NEW.comparator_id,NEW.comparator_version,NEW.comparison_result,NEW.comparison_ref,NEW.comparison_sha256)
        IS DISTINCT FROM (p.comparator_id,p.comparator_version,p.comparison_result,p.comparison_ref,p.comparison_sha256)) THEN
      RAISE EXCEPTION 'switch requires exact passing validation';
    END IF;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER projection_generation_event_guard BEFORE INSERT ON eventstore.projection_generation_events
  FOR EACH ROW EXECUTE FUNCTION eventstore.projection_generation_event_guard();

CREATE FUNCTION eventstore.projection_head_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE e eventstore.projection_generation_events%ROWTYPE;
BEGIN
  IF TG_OP='INSERT' THEN
    NEW.last_mutation_xid:=NULL;
    IF NEW.epoch<>1 OR NEW.active_generation_id IS NOT NULL OR NEW.hold_ref IS NOT NULL
      OR NEW.last_event_id IS NOT NULL OR NEW.last_switch_event_id IS NOT NULL THEN
      RAISE EXCEPTION 'new projection head must be inactive at epoch one';
    END IF;
    RETURN NEW;
  END IF;
  -- Constraint triggers can be flushed by SET CONSTRAINTS at any time. This
  -- row provenance is assigned by this trigger from PostgreSQL's actual top-
  -- level xid8, including subtransactions; savepoint rollback restores it.
  IF OLD.last_mutation_xid=pg_current_xact_id() THEN
    RAISE EXCEPTION 'pointer receipt requires matching committed head change: one mutation per transaction';
  END IF;
  NEW.last_mutation_xid:=pg_current_xact_id();
  IF NEW.projection_name IS DISTINCT FROM OLD.projection_name OR OLD.epoch=9223372036854775807
    OR NEW.epoch<>OLD.epoch+1 OR NEW.last_event_id IS NULL OR NEW.last_event_id IS NOT DISTINCT FROM OLD.last_event_id THEN
    RAISE EXCEPTION 'projection head requires next epoch and new immutable evidence';
  END IF;
  SELECT * INTO STRICT e FROM eventstore.projection_generation_events WHERE event_id=NEW.last_event_id;
  IF e.projection_name<>OLD.projection_name OR e.expected_head_epoch IS DISTINCT FROM OLD.epoch
    OR e.expected_active_generation_id IS DISTINCT FROM OLD.active_generation_id
    OR e.created_xid<>NEW.last_mutation_xid THEN
    RAISE EXCEPTION 'projection head evidence does not match expected pointer';
  END IF;
  IF e.action='switch' THEN
    IF EXISTS (SELECT FROM eventstore.projection_generation_events held
      WHERE held.projection_name=NEW.projection_name AND held.generation_id=NEW.active_generation_id
        AND held.action='hold' AND held.expected_head_epoch IS NULL
        AND held.created_xid=NEW.last_mutation_xid) THEN
      RAISE EXCEPTION 'active generation hold requires matching committed head change';
    END IF;
    IF NEW.active_generation_id IS DISTINCT FROM e.generation_id
      OR NEW.active_generation_id IS NOT DISTINCT FROM OLD.active_generation_id
      OR NEW.last_switch_event_id IS DISTINCT FROM e.event_id OR NEW.hold_ref IS NOT NULL THEN
      RAISE EXCEPTION 'projection switch target mismatch';
    END IF;
  ELSIF e.action='hold' THEN
    IF OLD.active_generation_id IS NULL OR e.generation_id<>OLD.active_generation_id
      OR NEW.active_generation_id IS DISTINCT FROM OLD.active_generation_id
      OR NEW.last_switch_event_id IS DISTINCT FROM OLD.last_switch_event_id OR NEW.hold_ref IS DISTINCT FROM e.hold_ref THEN
      RAISE EXCEPTION 'projection hold target mismatch';
    END IF;
  ELSE RAISE EXCEPTION 'projection head requires switch or hold evidence'; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER projection_head_guard BEFORE INSERT OR UPDATE ON eventstore.projection_heads
  FOR EACH ROW EXECUTE FUNCTION eventstore.projection_head_guard();
CREATE TRIGGER projection_head_no_delete BEFORE DELETE ON eventstore.projection_heads
  FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_immutable();
CREATE TRIGGER projection_head_no_truncate BEFORE TRUNCATE ON eventstore.projection_heads
  FOR EACH STATEMENT EXECUTE FUNCTION eventstore.messaging_immutable();

-- A switch receipt (or active hold receipt) cannot commit independently of the
-- matching CAS. This deferred backstop can also run early on SET CONSTRAINTS;
-- the mutation-time guards above preserve its result under subsequent writes.
-- Unknown replies reconcile request and head, not transaction bookkeeping.
CREATE FUNCTION eventstore.projection_pointer_receipt_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE h eventstore.projection_heads%ROWTYPE;
BEGIN
  IF NEW.action='hold' AND NEW.expected_head_epoch IS NULL AND EXISTS (
    SELECT FROM eventstore.projection_heads WHERE projection_name=NEW.projection_name
      AND active_generation_id=NEW.generation_id) THEN
    RAISE EXCEPTION 'active generation hold requires matching committed head change';
  END IF;
  IF NEW.expected_head_epoch IS NOT NULL THEN
    SELECT * INTO STRICT h FROM eventstore.projection_heads WHERE projection_name=NEW.projection_name;
    IF h.last_event_id IS DISTINCT FROM NEW.event_id OR NEW.expected_head_epoch=9223372036854775807
      OR h.epoch<>NEW.expected_head_epoch+1 OR h.active_generation_id IS DISTINCT FROM NEW.generation_id
      OR h.last_mutation_xid IS DISTINCT FROM NEW.created_xid
      OR (NEW.action='switch' AND h.last_switch_event_id IS DISTINCT FROM NEW.event_id)
      OR h.hold_ref IS DISTINCT FROM NEW.hold_ref THEN
      RAISE EXCEPTION 'pointer receipt requires matching committed head change';
    END IF;
  END IF;
  RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER projection_pointer_receipt_guard AFTER INSERT ON eventstore.projection_generation_events
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION eventstore.projection_pointer_receipt_guard();

DO $immutability$
DECLARE tab text;
BEGIN
  FOREACH tab IN ARRAY ARRAY['projection_generations','projection_generation_events','projection_generation_compatibility'] LOOP
    EXECUTE format('CREATE TRIGGER immutable_content BEFORE UPDATE OR DELETE ON eventstore.%I FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_immutable()',tab);
    EXECUTE format('CREATE TRIGGER immutable_truncate BEFORE TRUNCATE ON eventstore.%I FOR EACH STATEMENT EXECUTE FUNCTION eventstore.messaging_immutable()',tab);
  END LOOP;
END
$immutability$;
REVOKE ALL ON eventstore.projection_generations,eventstore.projection_generation_events,
  eventstore.projection_heads,eventstore.projection_generation_compatibility FROM PUBLIC;
GRANT SELECT ON eventstore.projection_generation_compatibility TO :"runtime_role";
GRANT SELECT,INSERT ON eventstore.projection_generations,eventstore.projection_generation_events,eventstore.projection_heads TO :"runtime_role";
GRANT UPDATE(active_generation_id,epoch,hold_ref,last_event_id,last_switch_event_id,updated_at) ON eventstore.projection_heads TO :"runtime_role";
REVOKE ALL ON FUNCTION eventstore.projection_generation_event_guard(),eventstore.projection_head_guard(),
  eventstore.projection_pointer_receipt_guard() FROM PUBLIC;

-- Audit old and new objects without rewriting their existing ACLs.
DO $audit$
DECLARE r record; tab text; col record; allowed text[];
BEGIN
  IF NOT has_schema_privilege(current_setting('justix.install_runtime'),'eventstore','USAGE') THEN
    RAISE EXCEPTION 'missing required runtime schema USAGE';
  END IF;
  FOREACH tab IN ARRAY ARRAY['inbox','messaging_admissions','consumer_bootstraps','consumer_checkpoints',
    'consumer_gaps','consumer_gap_attempts','quarantine_evidence','quarantine_actions','projection_generations','projection_generation_events','projection_heads'] LOOP
    IF NOT has_table_privilege(current_setting('justix.install_runtime'),'eventstore.'||tab,'INSERT') THEN
      RAISE EXCEPTION 'missing required runtime INSERT on %',tab;
    END IF;
  END LOOP;
  FOREACH tab IN ARRAY ARRAY['inbox','dispatch_messages','dispatch_jobs','messaging_admissions','messaging_mode','messaging_route_compatibility',
    'consumer_bootstraps','consumer_checkpoints','consumer_gaps','consumer_gap_attempts','projection_checkpoint_compatibility',
    'quarantine_evidence','quarantine_actions','quarantine_compatibility',
      'projection_generations','projection_generation_events','projection_heads','projection_generation_compatibility'] LOOP
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
      'quarantine_evidence','quarantine_actions','quarantine_compatibility',
      'projection_generations','projection_generation_events','projection_heads','projection_generation_compatibility'] LOOP
      IF EXISTS (SELECT FROM pg_class WHERE oid=('eventstore.'||tab)::regclass AND (relowner<>current_user::regrole::oid OR relkind<>'r' OR relrowsecurity OR relforcerowsecurity)) THEN
        RAISE EXCEPTION 'unexpected mechanics table owner on %',tab;
      END IF;
      IF has_table_privilege(r.oid,'eventstore.'||tab,'UPDATE,DELETE,TRUNCATE,TRIGGER,REFERENCES,MAINTAIN') THEN
        RAISE EXCEPTION 'unexpected messaging table privileges on %',tab;
      END IF;
      IF tab IN ('projection_generation_compatibility','quarantine_compatibility','projection_checkpoint_compatibility','messaging_route_compatibility','messaging_mode','messaging_legacy_authorizations','messaging_cutovers','messaging_legacy_evidence')
        AND has_any_column_privilege(r.oid,'eventstore.'||tab,'INSERT') THEN
        RAISE EXCEPTION 'unexpected migration evidence INSERT';
      END IF;
      IF tab='outbox' AND (SELECT mode FROM eventstore.messaging_mode WHERE singleton)='custody'
        AND (has_any_column_privilege(r.oid,'eventstore.outbox','INSERT,UPDATE')) THEN
        RAISE EXCEPTION 'custody legacy runtime grants remain writable';
      END IF;
      allowed:=CASE tab
        WHEN 'projection_heads' THEN ARRAY['active_generation_id','epoch','hold_ref','last_event_id','last_switch_event_id','updated_at']
        WHEN 'consumer_checkpoints' THEN ARRAY['position','last_event_id','last_event_hash','revision','updated_at']
        WHEN 'outbox' THEN ARRAY['attempts','next_attempt_at','lease_owner','lease_until','sent_at']
        WHEN 'operations' THEN ARRAY['phase','decision','attention_required','result','error','attempts','next_attempt_at','lease_owner','lease_until','revision','updated_at']
        WHEN 'outbox_deliveries' THEN ARRAY['attempts','next_attempt_at','lease_owner','lease_until','sent_at','hold_ref']
        WHEN 'dispatch_jobs' THEN ARRAY['attempts','next_attempt_at','lease_owner','lease_until','completed_at','inbox_consumer_name','inbox_event_id','hold_ref','quarantine_ref']
        ELSE ARRAY[]::text[] END;
      FOR col IN SELECT attname FROM pg_attribute WHERE attrelid=('eventstore.'||tab)::regclass AND attnum>0 AND NOT attisdropped LOOP
        IF r.oid=current_setting('justix.install_runtime')::regrole::oid AND col.attname=ANY(allowed)
          AND NOT (tab='outbox' AND (SELECT mode FROM eventstore.messaging_mode WHERE singleton)='custody')
          AND NOT has_column_privilege(r.oid,'eventstore.'||tab,col.attname,'UPDATE') THEN
          RAISE EXCEPTION 'missing required runtime UPDATE on %.%',tab,col.attname;
        END IF;
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
