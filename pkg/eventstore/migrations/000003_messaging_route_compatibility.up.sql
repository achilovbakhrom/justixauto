\set ON_ERROR_STOP on
-- Explicit forward correction; no down migration or automatic retained-data repair.
-- Run as the existing schema migration owner, after externally verified backup,
-- stopped affected processes and closed old AMQP channels. Required psql variables:
-- owner_service, runtime_role, prior_migration_sha256, correction_migration_sha256,
-- backup_ref, stopped_runtimes_ref, compatibility_ref. Digests are hex SHA-256.
-- Runner must compute/verify both exact artifacts. SQL cannot hash its own input
-- file or prove the external references. Existing cutover evidence stays intact.
BEGIN;
SET LOCAL search_path=pg_catalog;
SET LOCAL lock_timeout='5s';
SELECT set_config('justix.install_owner', :'owner_service', true);
SELECT set_config('justix.install_runtime', :'runtime_role', true);
SELECT set_config('justix.route_prior_hash', :'prior_migration_sha256', true);
SELECT set_config('justix.route_correction_hash', :'correction_migration_sha256', true);
DO $prerequisite$
DECLARE r pg_roles%ROWTYPE; owner_oid oid;
BEGIN
  SELECT nspowner INTO owner_oid FROM pg_namespace WHERE nspname='eventstore';
  IF owner_oid IS NULL OR owner_oid<>current_user::regrole::oid THEN
    RAISE EXCEPTION 'existing eventstore migration owner required';
  END IF;
  IF to_regclass('eventstore.messaging_mode') IS NULL THEN
    RAISE EXCEPTION 'installed messaging schema version 2 required';
  END IF;
  IF to_regclass('eventstore.messaging_route_compatibility') IS NOT NULL THEN
    RAISE EXCEPTION 'route compatibility marker already exists';
  END IF;
  IF current_setting('justix.route_prior_hash')<>'1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2'
    OR current_setting('justix.route_correction_hash') !~ '^[0-9a-f]{64}$' THEN
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
  -- CREATE OR REPLACE must preserve the existing invoker, search path and ACLs,
  -- not silently take ownership of or relax a differently installed function.
  IF (SELECT count(*) FROM pg_proc WHERE pronamespace='eventstore'::regnamespace
    AND proname IN ('messaging_delivery_guard','messaging_job_guard','messaging_dispatch_route')
    AND pronargs=0 AND prorettype='trigger'::regtype AND proowner=owner_oid
    AND NOT prosecdef AND proconfig=ARRAY['search_path=pg_catalog'])<>3 THEN
    RAISE EXCEPTION 'incompatible route function prerequisite';
  END IF;
END
$prerequisite$;

-- Match cutover's legacy-table-before-mode ordering. Do not hold the mode row
-- while waiting for an old writer. Every lock wait is bounded and any failure
-- rolls back all DDL and marker creation in this transaction.
LOCK TABLE eventstore.outbox, eventstore.inbox, eventstore.messaging_mode,
  eventstore.outbox_messages, eventstore.outbox_deliveries,
  eventstore.dispatch_messages, eventstore.dispatch_enrollments,
  eventstore.dispatch_jobs IN ACCESS EXCLUSIVE MODE;
DO $retained$
DECLARE m eventstore.messaging_mode%ROWTYPE;
BEGIN
  SELECT * INTO STRICT m FROM eventstore.messaging_mode WHERE singleton FOR UPDATE;
  IF (SELECT count(*) FROM eventstore.messaging_mode)<>1 OR m.schema_version<>2
    OR m.mode NOT IN ('legacy','custody')
    OR m.owner_service<>current_setting('justix.install_owner')
    OR m.runtime_role::text<>current_setting('justix.install_runtime') THEN
    RAISE EXCEPTION 'installed owner, runtime or schema identity mismatch';
  END IF;
  IF (SELECT pg_get_expr(adbin,adrelid) FROM pg_attrdef d JOIN pg_attribute a
      ON a.attrelid=d.adrelid AND a.attnum=d.adnum
      WHERE d.adrelid='eventstore.events'::regclass AND a.attname='owner_service')
      IS DISTINCT FROM quote_literal(m.owner_service)||'::text' THEN
    RAISE EXCEPTION 'eventstore owner default mismatch';
  END IF;
  IF EXISTS (SELECT FROM eventstore.outbox o WHERE o.exchange<>'justix.integration.v1'
    OR octet_length(o.routing_key)>255
    OR split_part(o.routing_key,'.',2) NOT IN ('identity','inventory','commerce','retail','financing','insurance','documents')
    OR o.routing_key !~ ('^'||m.owner_service||'\.'||split_part(o.routing_key,'.',2)||'\.'||m.owner_service||'(\.[a-z][a-z0-9-]*)+\.v[1-9][0-9]*$')) THEN
    RAISE EXCEPTION 'incompatible retained legacy route; separate reviewed disposition required';
  END IF;
  IF EXISTS (SELECT FROM eventstore.outbox_deliveries d JOIN eventstore.outbox_messages p USING(event_id)
    LEFT JOIN eventstore.messaging_admissions a ON a.admission_id=d.admission_id
    WHERE octet_length(d.routing_key)>255
      OR d.routing_key !~ ('^'||p.source_owner||'\.'||d.destination||'\.'||p.source_owner||'(\.[a-z][a-z0-9-]*)+\.v[1-9][0-9]*$')
      OR (d.admission_id IS NOT NULL AND NOT a.schema_allowlist ? substring(d.routing_key FROM length(p.source_owner)+length(d.destination)+3))) THEN
    RAISE EXCEPTION 'incompatible retained delivery route or schema admission';
  END IF;
  IF EXISTS (SELECT FROM eventstore.dispatch_messages p WHERE octet_length(p.routing_key)>255
    OR p.routing_key !~ ('^'||p.source_owner||'\.'||m.owner_service||'\.'||p.source_owner||'(\.[a-z][a-z0-9-]*)+\.v[1-9][0-9]*$')) THEN
    RAISE EXCEPTION 'incompatible retained custody route';
  END IF;
  IF EXISTS (SELECT FROM eventstore.dispatch_jobs j JOIN eventstore.dispatch_messages p USING(event_id)
    JOIN eventstore.messaging_admissions a ON a.admission_id=j.admission_id
    WHERE NOT a.schema_allowlist ? substring(p.routing_key FROM length(p.source_owner)+length(m.owner_service)+3)) THEN
    RAISE EXCEPTION 'incompatible retained job schema admission';
  END IF;
END
$retained$;

-- Reject hazardous default ACLs before creating anything; do not mask a default
-- grant by revoking it on the new marker. Global and schema-specific defaults,
-- PUBLIC and all runtime SET ROLE-reachable grantees are considered.
DO $defaults$
BEGIN
  IF EXISTS (SELECT FROM pg_default_acl d CROSS JOIN LATERAL aclexplode(d.defaclacl) a
    WHERE d.defaclrole=current_user::regrole::oid
      AND d.defaclnamespace IN (0,'eventstore'::regnamespace::oid)
      AND ((a.grantee=0 AND d.defaclobjtype='r') OR
        (a.grantee<>0 AND pg_has_role(current_setting('justix.install_runtime'),a.grantee,'MEMBER')
          AND ((d.defaclobjtype='r' AND (a.privilege_type<>'SELECT' OR a.is_grantable))
            OR d.defaclobjtype='f')))) THEN
    RAISE EXCEPTION 'unexpected runtime or PUBLIC default privileges';
  END IF;
END
$defaults$;

CREATE TABLE eventstore.messaging_route_compatibility (
  singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
  owner_service text NOT NULL CHECK(owner_service=:'owner_service'),
  runtime_role name NOT NULL CHECK(runtime_role=:'runtime_role'::name),
  base_schema_version integer NOT NULL CHECK(base_schema_version=2),
  migration_revision integer NOT NULL CHECK(migration_revision=3),
  route_format_version integer NOT NULL CHECK(route_format_version=1),
  prior_migration_sha256 bytea NOT NULL CHECK(prior_migration_sha256=decode('1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2','hex')),
  correction_migration_sha256 bytea NOT NULL CHECK(octet_length(correction_migration_sha256)=32),
  backup_ref text NOT NULL CHECK(length(btrim(backup_ref))>0),
  stopped_runtimes_ref text NOT NULL CHECK(length(btrim(stopped_runtimes_ref))>0),
  compatibility_ref text NOT NULL CHECK(length(btrim(compatibility_ref))>0),
  installed_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(installed_at))
);
INSERT INTO eventstore.messaging_route_compatibility(singleton,owner_service,runtime_role,
  base_schema_version,migration_revision,route_format_version,prior_migration_sha256,
  correction_migration_sha256,backup_ref,stopped_runtimes_ref,compatibility_ref)
VALUES(true,:'owner_service',:'runtime_role',2,3,1,decode(:'prior_migration_sha256','hex'),
  decode(:'correction_migration_sha256','hex'),:'backup_ref',:'stopped_runtimes_ref',:'compatibility_ref');
CREATE TRIGGER immutable_content BEFORE UPDATE OR DELETE ON eventstore.messaging_route_compatibility
  FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_immutable();
REVOKE ALL ON eventstore.messaging_route_compatibility FROM PUBLIC;
GRANT SELECT ON eventstore.messaging_route_compatibility TO :"runtime_role";

CREATE OR REPLACE FUNCTION eventstore.messaging_delivery_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE m eventstore.outbox_messages%ROWTYPE; a eventstore.messaging_admissions%ROWTYPE;
BEGIN
  IF TG_OP='UPDATE' THEN
    IF (to_jsonb(NEW)-ARRAY['attempts','next_attempt_at','lease_owner','lease_until','sent_at','hold_ref'])
      IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['attempts','next_attempt_at','lease_owner','lease_until','sent_at','hold_ref'])
      OR (OLD.sent_at IS NOT NULL AND NEW.sent_at IS DISTINCT FROM OLD.sent_at) THEN
      RAISE EXCEPTION 'delivery content or confirmed status is immutable' USING ERRCODE='23514';
    END IF;
  END IF;
  SELECT * INTO STRICT m FROM eventstore.outbox_messages WHERE event_id=NEW.event_id;
  IF octet_length(NEW.routing_key)>255 OR NEW.routing_key !~
    ('^'||m.source_owner||'\.'||NEW.destination||'\.'||m.source_owner||'(\.[a-z][a-z0-9-]*)+\.v[1-9][0-9]*$') THEN
    RAISE EXCEPTION 'invalid admitted destination route' USING ERRCODE='23514';
  END IF;
  IF NEW.admission_id IS NOT NULL THEN
    SELECT * INTO STRICT a FROM eventstore.messaging_admissions WHERE admission_id=NEW.admission_id;
    IF a.namespace<>'source-recipient' OR a.action<>'admit' OR a.subject<>NEW.destination
      OR (a.source_owner,a.aggregate_type,a.aggregate_id) IS DISTINCT FROM (m.source_owner,m.aggregate_type,m.aggregate_id)
      OR m.integration_sequence<=a.start_after OR (a.end_inclusive IS NOT NULL AND m.integration_sequence>a.end_inclusive) THEN
      RAISE EXCEPTION 'delivery admission mismatch' USING ERRCODE='23514';
    END IF;
    IF NOT a.schema_allowlist ? substring(NEW.routing_key FROM length(m.source_owner)+length(NEW.destination)+3) THEN
      RAISE EXCEPTION 'delivery schema is not admitted' USING ERRCODE='23514';
    END IF;
  ELSE
    -- Historical authority-unavailable obligations cannot lose their hold via a
    -- scheduling UPDATE. Re-admission/disposition needs separately owned work.
    IF NOT EXISTS (SELECT FROM eventstore.messaging_legacy_authorizations legacy_authority
      WHERE legacy_authority.event_id=NEW.legacy_event_id AND legacy_authority.destination=NEW.destination
        AND legacy_authority.admission_id IS NULL AND legacy_authority.hold_ref=NEW.hold_ref) THEN
      RAISE EXCEPTION 'legacy obligation must retain its evidenced hold' USING ERRCODE='23514';
    END IF;
  END IF;
  IF TG_OP='UPDATE' AND NEW.sent_at IS DISTINCT FROM OLD.sent_at AND NEW.hold_ref IS NOT NULL THEN
    RAISE EXCEPTION 'held delivery cannot be confirmed' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END
$$;

CREATE OR REPLACE FUNCTION eventstore.messaging_job_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE m eventstore.dispatch_messages%ROWTYPE; a eventstore.messaging_admissions%ROWTYPE;
BEGIN
  IF TG_OP='UPDATE' THEN
    IF (to_jsonb(NEW)-ARRAY['attempts','next_attempt_at','lease_owner','lease_until','completed_at','inbox_consumer_name','inbox_event_id','hold_ref','quarantine_ref'])
      IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['attempts','next_attempt_at','lease_owner','lease_until','completed_at','inbox_consumer_name','inbox_event_id','hold_ref','quarantine_ref'])
      OR (OLD.completed_at IS NOT NULL AND
        (NEW.completed_at,NEW.inbox_consumer_name,NEW.inbox_event_id) IS DISTINCT FROM
        (OLD.completed_at,OLD.inbox_consumer_name,OLD.inbox_event_id)) THEN
      RAISE EXCEPTION 'job content or completion is immutable' USING ERRCODE='23514';
    END IF;
  END IF;
  SELECT * INTO STRICT m FROM eventstore.dispatch_messages WHERE event_id=NEW.event_id;
  SELECT * INTO STRICT a FROM eventstore.messaging_admissions WHERE admission_id=NEW.admission_id;
  IF a.namespace<>'local-consumer' OR a.action<>'admit' OR a.subject<>NEW.consumer_name
    OR (a.consumer_kind,a.generation) IS DISTINCT FROM (NEW.consumer_kind,NEW.generation)
    OR (a.source_owner,a.aggregate_type,a.aggregate_id) IS DISTINCT FROM (m.source_owner,m.aggregate_type,m.aggregate_id)
    OR m.integration_sequence<=a.start_after OR (a.end_inclusive IS NOT NULL AND m.integration_sequence>a.end_inclusive) THEN
    RAISE EXCEPTION 'job admission mismatch' USING ERRCODE='23514';
  END IF;
  IF NOT a.schema_allowlist ? substring(m.routing_key FROM length(m.source_owner)+length(split_part(m.routing_key,'.',2))+3) THEN
    RAISE EXCEPTION 'job schema is not admitted' USING ERRCODE='23514';
  END IF;
  IF NEW.completed_at IS NOT NULL AND NOT EXISTS (SELECT FROM eventstore.inbox i
    WHERE i.consumer_name=NEW.consumer_name AND i.event_id=NEW.event_id AND i.envelope_hash=m.envelope_hash) THEN
    RAISE EXCEPTION 'completion requires matching inbox identity and bytes' USING ERRCODE='23514';
  END IF;
  IF NEW.completed_at IS NOT NULL AND (TG_OP='INSERT' OR OLD.completed_at IS NULL)
    AND (NEW.hold_ref IS NOT NULL OR NEW.quarantine_ref IS NOT NULL) THEN
    RAISE EXCEPTION 'held or quarantined job cannot complete' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END
$$;
CREATE OR REPLACE FUNCTION eventstore.messaging_dispatch_route() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE owner_name text;
BEGIN
  SELECT owner_service INTO owner_name FROM eventstore.messaging_mode WHERE singleton;
  IF octet_length(NEW.routing_key)>255 OR NEW.routing_key !~
    ('^'||NEW.source_owner||'\.'||owner_name||'\.'||NEW.source_owner||'(\.[a-z][a-z0-9-]*)+\.v[1-9][0-9]*$') THEN
    RAISE EXCEPTION 'invalid custody destination route' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END
$$;

-- Preserve installed function ACLs; only the new marker receives a grant.
DO $audit$
DECLARE r record; tab text; col record; allowed text[];
BEGIN
  FOR r IN SELECT oid FROM pg_roles WHERE pg_has_role(current_setting('justix.install_runtime'),oid,'MEMBER') LOOP
    IF has_database_privilege(r.oid,current_database(),'CREATE') OR has_schema_privilege(r.oid,'eventstore','CREATE') THEN RAISE EXCEPTION 'runtime schema CREATE'; END IF;
    FOREACH tab IN ARRAY ARRAY['events','command_receipts','outbox','inbox','operations','operation_steps',
      'messaging_route_compatibility','messaging_mode','messaging_admissions','messaging_legacy_authorizations',
      'messaging_cutovers','messaging_legacy_evidence','outbox_messages','outbox_deliveries',
      'dispatch_messages','dispatch_enrollments','dispatch_jobs'] LOOP
      IF has_table_privilege(r.oid,'eventstore.'||tab,'UPDATE,DELETE,TRUNCATE,TRIGGER,REFERENCES,MAINTAIN') THEN
        RAISE EXCEPTION 'unexpected messaging table privileges on %',tab;
      END IF;
      IF tab IN ('messaging_route_compatibility','messaging_mode','messaging_legacy_authorizations','messaging_cutovers','messaging_legacy_evidence')
        AND has_any_column_privilege(r.oid,'eventstore.'||tab,'INSERT') THEN
        RAISE EXCEPTION 'unexpected migration evidence INSERT';
      END IF;
      IF tab='outbox' AND (SELECT mode FROM eventstore.messaging_mode WHERE singleton)='custody'
        AND (has_any_column_privilege(r.oid,'eventstore.outbox','INSERT,UPDATE')) THEN
        RAISE EXCEPTION 'custody legacy runtime grants remain writable';
      END IF;
      allowed:=CASE tab
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
END
$audit$;
COMMIT;
