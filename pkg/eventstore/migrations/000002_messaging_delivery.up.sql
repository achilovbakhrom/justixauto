\set ON_ERROR_STOP on
-- Explicit additive migration, run once as the T-008 migration owner with
-- -v owner_service=<owner> -v runtime_role=<separately provisioned runtime>.
-- Installation leaves the active mode LEGACY. It does not activate contracts,
-- stop processes, drain AMQP channels, authorize data, or perform a cutover.
-- No down migration: legacy rows and all new evidence must remain recoverable.
BEGIN;
SET LOCAL search_path = pg_catalog;
SELECT set_config('justix.install_owner', :'owner_service', true);
SELECT set_config('justix.install_runtime', :'runtime_role', true);
DO $guard$
DECLARE r pg_roles%ROWTYPE; owner_oid oid;
BEGIN
  IF current_setting('justix.install_owner') NOT IN
    ('identity','inventory','commerce','retail','financing','insurance','documents') THEN
    RAISE EXCEPTION 'unknown owner service';
  END IF;
  SELECT * INTO r FROM pg_roles WHERE rolname = current_setting('justix.install_runtime');
  IF NOT FOUND THEN RAISE EXCEPTION 'runtime role must already exist'; END IF;
  SELECT nspowner INTO owner_oid FROM pg_namespace WHERE nspname = 'eventstore';
  IF owner_oid IS NULL OR owner_oid <> current_user::regrole::oid THEN
    RAISE EXCEPTION 'T-008 schema migration owner required';
  END IF;
  IF r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls
    OR pg_has_role(r.oid, owner_oid, 'MEMBER')
    OR pg_has_role(r.oid, (SELECT datdba FROM pg_database WHERE datname=current_database()), 'MEMBER')
    OR EXISTS (SELECT FROM pg_roles p WHERE p.oid <> r.oid AND pg_has_role(r.oid,p.oid,'MEMBER')
      AND (p.rolname LIKE 'pg\_%' ESCAPE '\' OR p.rolsuper OR p.rolcreatedb OR p.rolcreaterole
        OR p.rolreplication OR p.rolbypassrls)) THEN
    RAISE EXCEPTION 'runtime role has privileged ownership or membership';
  END IF;
  IF EXISTS (SELECT FROM eventstore.events WHERE owner_service <> current_setting('justix.install_owner')) THEN
    RAISE EXCEPTION 'owner mismatch';
  END IF;
  IF (SELECT pg_get_expr(adbin,adrelid) FROM pg_attrdef d JOIN pg_attribute a
    ON a.attrelid=d.adrelid AND a.attnum=d.adnum
    WHERE d.adrelid='eventstore.events'::regclass AND a.attname='owner_service')
    IS DISTINCT FROM quote_literal(current_setting('justix.install_owner'))||'::text' THEN
    RAISE EXCEPTION 'T-008 owner default mismatch';
  END IF;
END
$guard$;

CREATE TABLE eventstore.messaging_mode (
  singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  owner_service text NOT NULL CHECK (owner_service = :'owner_service'),
  runtime_role name NOT NULL,
  mode text NOT NULL CHECK (mode IN ('legacy','custody')),
  schema_version integer NOT NULL CHECK (schema_version = 2)
);
INSERT INTO eventstore.messaging_mode VALUES (true, :'owner_service', :'runtime_role', 'legacy', 2);

-- An evidence reference is not a grant. Owner adapters validate authority,
-- schemas, purpose and effective non-overlapping ranges under the T-009 stream
-- fence. These immutable records preserve what that validation admitted.
CREATE TABLE eventstore.messaging_admissions (
  admission_id uuid PRIMARY KEY,
  namespace text NOT NULL CHECK (namespace IN ('source-recipient','local-consumer')),
  source_owner text NOT NULL CHECK (source_owner IN ('identity','inventory','commerce','retail','financing','insurance','documents')),
  aggregate_type text NOT NULL CHECK (aggregate_type ~ '^[a-z][a-z0-9-]*$'),
  aggregate_id uuid NOT NULL,
  subject text NOT NULL CHECK (length(subject) > 0),
  action text NOT NULL CHECK (action IN ('admit','close','hold','resume')),
  prior_admission_id uuid REFERENCES eventstore.messaging_admissions(admission_id),
  contract_id text NOT NULL CHECK (length(contract_id) > 0),
  contract_version bigint NOT NULL CHECK (contract_version BETWEEN 1 AND 4294967295),
  contract_digest bytea NOT NULL CHECK (octet_length(contract_digest) = 32),
  schema_allowlist jsonb NOT NULL CHECK (jsonb_typeof(schema_allowlist) = 'array' AND jsonb_array_length(schema_allowlist) > 0),
  authority_ref text NOT NULL CHECK (length(authority_ref) > 0),
  scope_ref text NOT NULL CHECK (length(scope_ref) > 0),
  purpose text NOT NULL CHECK (length(purpose) > 0),
  bootstrap_ref text NOT NULL CHECK (length(bootstrap_ref) > 0),
  start_after bigint NOT NULL CHECK (start_after >= 0),
  end_inclusive bigint CHECK (end_inclusive >= start_after),
  consumer_kind text CHECK (consumer_kind IN ('projection','process')),
  generation text,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(created_at)),
  CHECK ((action = 'admit') = (prior_admission_id IS NULL)),
  CHECK (action <> 'close' OR end_inclusive IS NOT NULL),
  CHECK ((namespace = 'source-recipient' AND source_owner = :'owner_service'
      AND subject IN ('identity','inventory','commerce','retail','financing','insurance','documents')
      AND consumer_kind IS NULL AND generation IS NULL)
    OR (namespace = 'local-consumer' AND consumer_kind IS NOT NULL AND generation IS NOT NULL AND length(generation) > 0))
);
CREATE INDEX messaging_admissions_stream ON eventstore.messaging_admissions
  (namespace,source_owner,aggregate_type,aggregate_id,subject,start_after);

-- Populated only by the migration authority after reviewing each historical
-- route. NULL admission means historical authority is unavailable: preserve an
-- explicit hold, never invent an admission or make it retryable by default.
CREATE TABLE eventstore.messaging_legacy_authorizations (
  event_id uuid PRIMARY KEY REFERENCES eventstore.outbox(event_id),
  destination text NOT NULL CHECK (destination IN ('identity','inventory','commerce','retail','financing','insurance','documents')),
  admission_id uuid REFERENCES eventstore.messaging_admissions(admission_id),
  hold_ref text CHECK (length(hold_ref) > 0),
  authority_ref text NOT NULL CHECK (length(authority_ref) > 0),
  CHECK ((admission_id IS NULL) = (hold_ref IS NOT NULL))
);
CREATE TABLE eventstore.messaging_cutovers (
  cutover_id uuid PRIMARY KEY,
  schema_hash bytea NOT NULL CHECK (octet_length(schema_hash)=32),
  prior_schema_hash bytea NOT NULL CHECK (octet_length(prior_schema_hash)=32),
  backup_ref text NOT NULL CHECK (length(backup_ref)>0),
  stopped_runtimes_ref text NOT NULL CHECK (length(stopped_runtimes_ref)>0),
  broker_and_checkpoint_ref text NOT NULL CHECK (length(broker_and_checkpoint_ref)>0),
  compatibility_ref text NOT NULL CHECK (length(compatibility_ref)>0),
  legacy_rows bigint NOT NULL CHECK (legacy_rows>=0),
  legacy_inbox_rows bigint NOT NULL CHECK (legacy_inbox_rows>=0),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE eventstore.messaging_legacy_evidence (
  event_id uuid PRIMARY KEY REFERENCES eventstore.messaging_legacy_authorizations(event_id),
  cutover_id uuid NOT NULL REFERENCES eventstore.messaging_cutovers(cutover_id),
  original_row jsonb NOT NULL CHECK (jsonb_typeof(original_row)='object')
);

CREATE TABLE eventstore.outbox_messages (
  event_id uuid PRIMARY KEY,
  source_owner text NOT NULL CHECK (source_owner = :'owner_service'),
  aggregate_type text NOT NULL CHECK (aggregate_type ~ '^[a-z][a-z0-9-]*$'),
  aggregate_id uuid NOT NULL,
  integration_sequence bigint NOT NULL CHECK (integration_sequence > 0),
  envelope bytea NOT NULL CHECK (octet_length(envelope)>0),
  envelope_hash bytea NOT NULL CHECK (envelope_hash=sha256(envelope)),
  recipients jsonb NOT NULL CHECK (jsonb_typeof(recipients)='array'),
  plan_authority_ref text NOT NULL CHECK (length(plan_authority_ref)>0),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(created_at)),
  UNIQUE (source_owner,aggregate_type,aggregate_id,integration_sequence)
);
CREATE TABLE eventstore.outbox_deliveries (
  event_id uuid NOT NULL REFERENCES eventstore.outbox_messages(event_id),
  destination text NOT NULL CHECK (destination IN ('identity','inventory','commerce','retail','financing','insurance','documents')),
  admission_id uuid REFERENCES eventstore.messaging_admissions(admission_id),
  legacy_event_id uuid REFERENCES eventstore.messaging_legacy_evidence(event_id),
  exchange text NOT NULL CHECK (exchange='justix.integration.v1'),
  routing_key text NOT NULL CHECK (length(routing_key)>0),
  attempts bigint NOT NULL DEFAULT 0 CHECK (attempts>=0),
  next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(next_attempt_at)),
  lease_owner uuid,
  lease_until timestamptz CHECK (isfinite(lease_until)),
  sent_at timestamptz CHECK (isfinite(sent_at)),
  hold_ref text CHECK (length(hold_ref)>0),
  PRIMARY KEY (event_id,destination),
  CHECK ((admission_id IS NOT NULL) <> (legacy_event_id IS NOT NULL)),
  CHECK (legacy_event_id IS NULL OR legacy_event_id=event_id),
  CHECK ((lease_owner IS NULL)=(lease_until IS NULL))
);
CREATE INDEX outbox_deliveries_due ON eventstore.outbox_deliveries (next_attempt_at,event_id,destination)
  WHERE sent_at IS NULL AND hold_ref IS NULL;

CREATE TABLE eventstore.dispatch_messages (
  event_id uuid PRIMARY KEY,
  source_owner text NOT NULL CHECK (source_owner IN ('identity','inventory','commerce','retail','financing','insurance','documents')),
  aggregate_type text NOT NULL CHECK (aggregate_type ~ '^[a-z][a-z0-9-]*$'),
  aggregate_id uuid NOT NULL,
  integration_sequence bigint NOT NULL CHECK (integration_sequence>0),
  envelope bytea NOT NULL CHECK (octet_length(envelope)>0),
  envelope_hash bytea NOT NULL CHECK (envelope_hash=sha256(envelope)),
  exchange text NOT NULL CHECK (exchange='justix.integration.v1'),
  routing_key text NOT NULL CHECK (length(routing_key)>0),
  membership_version text NOT NULL CHECK (length(membership_version)>0),
  admission_ref text NOT NULL CHECK (length(admission_ref)>0),
  bootstrap_ref text NOT NULL CHECK (length(bootstrap_ref)>0),
  initial_enrollment_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(created_at)),
  UNIQUE(source_owner,aggregate_type,aggregate_id,integration_sequence)
);
CREATE TABLE eventstore.dispatch_enrollments (
  event_id uuid NOT NULL REFERENCES eventstore.dispatch_messages(event_id),
  enrollment_id uuid NOT NULL,
  membership_version text NOT NULL CHECK (length(membership_version)>0),
  cutover_ref text NOT NULL CHECK (length(cutover_ref)>0),
  consumers jsonb NOT NULL CHECK (jsonb_typeof(consumers)='array'),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(created_at)),
  PRIMARY KEY(event_id,enrollment_id)
);
ALTER TABLE eventstore.dispatch_messages ADD CONSTRAINT dispatch_initial_enrollment
  FOREIGN KEY(event_id,initial_enrollment_id) REFERENCES eventstore.dispatch_enrollments(event_id,enrollment_id)
  DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE eventstore.dispatch_jobs (
  consumer_name text NOT NULL CHECK (length(consumer_name)>0),
  event_id uuid NOT NULL,
  enrollment_id uuid NOT NULL,
  admission_id uuid NOT NULL REFERENCES eventstore.messaging_admissions(admission_id),
  consumer_kind text NOT NULL CHECK (consumer_kind IN ('projection','process')),
  generation text NOT NULL CHECK (length(generation)>0),
  attempts bigint NOT NULL DEFAULT 0 CHECK (attempts>=0),
  next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(next_attempt_at)),
  lease_owner uuid,
  lease_until timestamptz CHECK (isfinite(lease_until)),
  completed_at timestamptz CHECK (isfinite(completed_at)),
  inbox_consumer_name text,
  inbox_event_id uuid,
  hold_ref text CHECK (length(hold_ref)>0),
  quarantine_ref text CHECK (length(quarantine_ref)>0),
  PRIMARY KEY(consumer_name,event_id),
  FOREIGN KEY(event_id,enrollment_id) REFERENCES eventstore.dispatch_enrollments(event_id,enrollment_id),
  FOREIGN KEY(inbox_consumer_name,inbox_event_id) REFERENCES eventstore.inbox(consumer_name,event_id),
  CHECK ((lease_owner IS NULL)=(lease_until IS NULL)),
  CHECK ((completed_at IS NULL AND inbox_consumer_name IS NULL AND inbox_event_id IS NULL)
    OR (completed_at IS NOT NULL AND inbox_consumer_name IS NOT NULL AND inbox_event_id IS NOT NULL
      AND inbox_consumer_name=consumer_name AND inbox_event_id=event_id))
);
CREATE INDEX dispatch_jobs_due ON eventstore.dispatch_jobs (consumer_name,next_attempt_at,event_id)
  WHERE completed_at IS NULL AND hold_ref IS NULL AND quarantine_ref IS NULL;

CREATE FUNCTION eventstore.messaging_immutable() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION 'messaging evidence is immutable' USING ERRCODE='23514'; END
$$;
CREATE FUNCTION eventstore.messaging_mode_transition() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
  IF OLD.mode<>'legacy' OR NEW.mode<>'custody'
    OR (to_jsonb(OLD)-'mode') IS DISTINCT FROM (to_jsonb(NEW)-'mode')
    OR (SELECT count(*) FROM eventstore.messaging_cutovers)<>1 THEN
    RAISE EXCEPTION 'only an evidenced forward custody transition is permitted' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END
$$;
CREATE TRIGGER mode_transition BEFORE UPDATE ON eventstore.messaging_mode
  FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_mode_transition();
CREATE TRIGGER retain_mode BEFORE DELETE ON eventstore.messaging_mode
  FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_immutable();
CREATE FUNCTION eventstore.messaging_admission_shape() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE schema_name jsonb; prior eventstore.messaging_admissions%ROWTYPE;
BEGIN
  FOR schema_name IN SELECT value FROM jsonb_array_elements(NEW.schema_allowlist) LOOP
    IF jsonb_typeof(schema_name)<>'string' OR (schema_name #>> '{}') !~
      ('^'||NEW.source_owner||'(\.[a-z][a-z0-9-]*)+\.v[1-9][0-9]*$') THEN
      RAISE EXCEPTION 'invalid admission schema allowlist' USING ERRCODE='23514';
    END IF;
  END LOOP;
  IF jsonb_array_length(NEW.schema_allowlist)<>(SELECT count(DISTINCT value) FROM jsonb_array_elements(NEW.schema_allowlist)) THEN
    RAISE EXCEPTION 'duplicate admission schemas' USING ERRCODE='23514';
  END IF;
  IF NEW.prior_admission_id IS NOT NULL THEN
    SELECT * INTO STRICT prior FROM eventstore.messaging_admissions WHERE admission_id=NEW.prior_admission_id;
    IF (prior.namespace,prior.source_owner,prior.aggregate_type,prior.aggregate_id,prior.subject)
      IS DISTINCT FROM (NEW.namespace,NEW.source_owner,NEW.aggregate_type,NEW.aggregate_id,NEW.subject) THEN
      RAISE EXCEPTION 'admission evidence belongs to a different stream or subject' USING ERRCODE='23514';
    END IF;
  END IF;
  RETURN NEW;
END
$$;
CREATE TRIGGER admission_shape BEFORE INSERT ON eventstore.messaging_admissions
  FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_admission_shape();
CREATE FUNCTION eventstore.messaging_mode_guard() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE active_mode text;
BEGIN
  -- A row lock also fences cutover against a transaction that is still using
  -- legacy tables. Application composition additionally checks mode at startup.
  SELECT mode INTO active_mode FROM eventstore.messaging_mode WHERE singleton FOR SHARE;
  IF TG_ARGV[0] <> active_mode THEN
    RAISE EXCEPTION 'messaging mode mismatch: requires %', TG_ARGV[0] USING ERRCODE='23514';
  END IF;
  IF TG_TABLE_NAME='inbox' AND active_mode='custody'
    AND current_setting('justix.messaging_mode',true) IS DISTINCT FROM 'custody' THEN
    RAISE EXCEPTION 'custody inbox requires explicit transaction adapter' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END
$$;
CREATE FUNCTION eventstore.messaging_inbox_guard() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE active_mode text;
BEGIN
  SELECT mode INTO active_mode FROM eventstore.messaging_mode WHERE singleton FOR SHARE;
  IF active_mode='custody' AND current_setting('justix.messaging_mode',true) IS DISTINCT FROM 'custody' THEN
    RAISE EXCEPTION 'custody inbox requires explicit transaction adapter' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END
$$;
CREATE TRIGGER legacy_outbox_mode BEFORE INSERT OR UPDATE ON eventstore.outbox
  FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_mode_guard('legacy');
CREATE TRIGGER inbox_mode BEFORE INSERT ON eventstore.inbox
  FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_inbox_guard();

-- Deferred checks see all children committed in the same transaction. UUID
-- admission references transitively freeze immutable contract/digest evidence.
-- Arrays, rather than JSON objects keyed by recipient, preserve duplicates so
-- a malformed plan cannot silently collapse two admissions into one child.
CREATE FUNCTION eventstore.messaging_complete() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE expected jsonb; actual jsonb; eid uuid; enrollment uuid;
BEGIN
  eid := NEW.event_id;
  IF TG_TABLE_NAME IN ('outbox_messages','outbox_deliveries') THEN
    SELECT coalesce(jsonb_agg(x ORDER BY x->>'destination'),'[]') INTO expected
      FROM eventstore.outbox_messages m, jsonb_array_elements(m.recipients) x WHERE m.event_id=eid;
    SELECT coalesce(jsonb_agg(jsonb_strip_nulls(jsonb_build_object('destination',destination,
      'admission_id',admission_id,'legacy_event_id',legacy_event_id)) ORDER BY destination),'[]') INTO actual
      FROM eventstore.outbox_deliveries WHERE event_id=eid;
  ELSE
    IF TG_TABLE_NAME='dispatch_messages' THEN enrollment:=NEW.initial_enrollment_id;
    ELSE enrollment:=NEW.enrollment_id; END IF;
    SELECT coalesce(jsonb_agg(x ORDER BY x->>'consumer_name'),'[]') INTO expected
      FROM eventstore.dispatch_enrollments e, jsonb_array_elements(e.consumers) x
      WHERE e.event_id=eid AND e.enrollment_id=enrollment;
    SELECT coalesce(jsonb_agg(jsonb_build_object('consumer_name',consumer_name,'admission_id',admission_id)
      ORDER BY consumer_name),'[]') INTO actual FROM eventstore.dispatch_jobs
      WHERE event_id=eid AND enrollment_id=enrollment;
    IF EXISTS (SELECT FROM eventstore.dispatch_messages m JOIN eventstore.dispatch_enrollments e
      ON e.event_id=m.event_id AND e.enrollment_id=m.initial_enrollment_id
      WHERE m.event_id=eid AND m.membership_version<>e.membership_version) THEN
      RAISE EXCEPTION 'initial membership mismatch' USING ERRCODE='23514';
    END IF;
  END IF;
  IF expected IS DISTINCT FROM actual THEN
    RAISE EXCEPTION 'incomplete or conflicting messaging children' USING ERRCODE='23514';
  END IF;
  RETURN NULL;
END
$$;

CREATE FUNCTION eventstore.messaging_delivery_guard() RETURNS trigger
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
  IF NEW.routing_key !~ ('^'||m.source_owner||'\.'||NEW.destination||'\.[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)*\.v[1-9][0-9]*$') THEN
    RAISE EXCEPTION 'invalid admitted destination route' USING ERRCODE='23514';
  END IF;
  IF NEW.admission_id IS NOT NULL THEN
    SELECT * INTO STRICT a FROM eventstore.messaging_admissions WHERE admission_id=NEW.admission_id;
    IF a.namespace<>'source-recipient' OR a.action<>'admit' OR a.subject<>NEW.destination
      OR (a.source_owner,a.aggregate_type,a.aggregate_id) IS DISTINCT FROM (m.source_owner,m.aggregate_type,m.aggregate_id)
      OR m.integration_sequence<=a.start_after OR (a.end_inclusive IS NOT NULL AND m.integration_sequence>a.end_inclusive) THEN
      RAISE EXCEPTION 'delivery admission mismatch' USING ERRCODE='23514';
    END IF;
    IF NOT a.schema_allowlist ? (m.source_owner||substring(NEW.routing_key FROM length(m.source_owner)+length(NEW.destination)+2)) THEN
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

CREATE FUNCTION eventstore.messaging_job_guard() RETURNS trigger
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
  IF NOT a.schema_allowlist ? (m.source_owner||substring(m.routing_key FROM length(m.source_owner)+length(split_part(m.routing_key,'.',2))+2)) THEN
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
CREATE FUNCTION eventstore.messaging_dispatch_route() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE owner_name text;
BEGIN
  SELECT owner_service INTO owner_name FROM eventstore.messaging_mode WHERE singleton;
  IF NEW.routing_key !~ ('^'||NEW.source_owner||'\.'||owner_name||'\.[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)*\.v[1-9][0-9]*$') THEN
    RAISE EXCEPTION 'invalid custody destination route' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END
$$;

DO $triggers$
DECLARE tab text;
BEGIN
  FOREACH tab IN ARRAY ARRAY['messaging_admissions','messaging_legacy_authorizations','messaging_cutovers',
    'messaging_legacy_evidence','outbox_messages','dispatch_messages','dispatch_enrollments'] LOOP
    EXECUTE format('CREATE TRIGGER immutable_content BEFORE UPDATE OR DELETE ON eventstore.%I FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_immutable()',tab);
  END LOOP;
  FOREACH tab IN ARRAY ARRAY['outbox_messages','outbox_deliveries','dispatch_messages','dispatch_enrollments','dispatch_jobs'] LOOP
    EXECUTE format('CREATE TRIGGER custody_mode BEFORE INSERT OR UPDATE ON eventstore.%I FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_mode_guard(''custody'')',tab);
    EXECUTE format('CREATE CONSTRAINT TRIGGER complete_children AFTER INSERT ON eventstore.%I DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_complete()',tab);
  END LOOP;
  FOREACH tab IN ARRAY ARRAY['outbox_deliveries','dispatch_jobs'] LOOP
    EXECUTE format('CREATE TRIGGER retain_work BEFORE DELETE ON eventstore.%I FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_immutable()',tab);
  END LOOP;
END
$triggers$;
CREATE TRIGGER delivery_content BEFORE INSERT OR UPDATE ON eventstore.outbox_deliveries
  FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_delivery_guard();
CREATE TRIGGER job_content BEFORE INSERT OR UPDATE ON eventstore.dispatch_jobs
  FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_job_guard();
CREATE TRIGGER dispatch_route BEFORE INSERT ON eventstore.dispatch_messages
  FOR EACH ROW EXECUTE FUNCTION eventstore.messaging_dispatch_route();

-- This is an explicit migration-owner operation, SECURITY INVOKER, revoked from
-- PUBLIC/runtime. Evidence references must identify externally verified backups,
-- stopped processes/channels, inbox/checkpoints/backlog and compatibility checks.
-- SQL cannot prove that a referenced backup exists or a broker channel is closed.
CREATE FUNCTION eventstore.activate_messaging_custody(
  cutover uuid, new_schema_hash bytea, old_schema_hash bytea, backup text,
  stopped_runtimes text, broker_and_checkpoints text, compatibility text) RETURNS void
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE mode_row eventstore.messaging_mode%ROWTYPE; o record;
BEGIN
  -- Acquire table locks first, in legacy write order, before the mode-row fence.
  -- No old append/relay or direct inbox transaction can straddle this snapshot.
  LOCK TABLE eventstore.outbox, eventstore.inbox IN ACCESS EXCLUSIVE MODE;
  SELECT * INTO STRICT mode_row FROM eventstore.messaging_mode WHERE singleton FOR UPDATE;
  IF mode_row.mode<>'legacy' THEN RAISE EXCEPTION 'cutover requires legacy mode'; END IF;
  IF EXISTS (SELECT FROM eventstore.outbox legacy_row LEFT JOIN eventstore.messaging_legacy_authorizations a USING(event_id)
    WHERE a.event_id IS NULL OR legacy_row.exchange<>'justix.integration.v1'
      OR legacy_row.routing_key !~ ('^'||mode_row.owner_service||'\.'||a.destination||'\.[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)*\.v[1-9][0-9]*$')) THEN
    RAISE EXCEPTION 'unrecognized or unauthorized legacy route';
  END IF;
  INSERT INTO eventstore.messaging_cutovers VALUES(cutover,new_schema_hash,old_schema_hash,backup,
    stopped_runtimes,broker_and_checkpoints,compatibility,(SELECT count(*) FROM eventstore.outbox),
    (SELECT count(*) FROM eventstore.inbox),clock_timestamp());
  INSERT INTO eventstore.messaging_legacy_evidence SELECT event_id,cutover,to_jsonb(legacy_row) FROM eventstore.outbox legacy_row;
  UPDATE eventstore.messaging_mode SET mode='custody' WHERE singleton;
  FOR o IN SELECT legacy.*,a.destination,a.admission_id,a.hold_ref,a.authority_ref
    FROM eventstore.outbox legacy JOIN eventstore.messaging_legacy_authorizations a USING(event_id) LOOP
    INSERT INTO eventstore.outbox_messages VALUES(o.event_id,mode_row.owner_service,o.aggregate_type,
      o.aggregate_id,o.integration_sequence,o.envelope,o.envelope_hash,
      jsonb_build_array(jsonb_strip_nulls(jsonb_build_object('destination',o.destination,
        'admission_id',o.admission_id,'legacy_event_id',CASE WHEN o.admission_id IS NULL THEN o.event_id END))),
      o.authority_ref,o.created_at);
    INSERT INTO eventstore.outbox_deliveries(event_id,destination,admission_id,legacy_event_id,
      exchange,routing_key,attempts,next_attempt_at,sent_at,hold_ref)
      VALUES(o.event_id,o.destination,o.admission_id,CASE WHEN o.admission_id IS NULL THEN o.event_id END,
        o.exchange,o.routing_key,o.attempts,o.next_attempt_at,o.sent_at,o.hold_ref);
  END LOOP;
  -- Revoke both table and column grants: revoking table UPDATE alone leaves
  -- T-008's enumerated scheduling-column grants intact.
  EXECUTE format('REVOKE INSERT, UPDATE ON eventstore.outbox FROM %I',mode_row.runtime_role);
  EXECUTE format('REVOKE UPDATE (attempts,next_attempt_at,lease_owner,lease_until,sent_at) ON eventstore.outbox FROM %I',mode_row.runtime_role);
  FOR o IN SELECT oid FROM pg_roles WHERE pg_has_role(mode_row.runtime_role,oid,'MEMBER') LOOP
    IF has_table_privilege(o.oid,'eventstore.outbox','INSERT,UPDATE,DELETE,TRUNCATE,TRIGGER,REFERENCES')
      OR has_any_column_privilege(o.oid,'eventstore.outbox','INSERT,UPDATE,REFERENCES') THEN
      RAISE EXCEPTION 'legacy runtime grants remain writable';
    END IF;
  END LOOP;
END
$$;

REVOKE ALL ON eventstore.messaging_mode,eventstore.messaging_admissions,eventstore.messaging_legacy_authorizations,
  eventstore.messaging_cutovers,eventstore.messaging_legacy_evidence,eventstore.outbox_messages,eventstore.outbox_deliveries,
  eventstore.dispatch_messages,eventstore.dispatch_enrollments,eventstore.dispatch_jobs FROM PUBLIC;
GRANT SELECT ON eventstore.messaging_mode,eventstore.messaging_legacy_authorizations,eventstore.messaging_cutovers,
  eventstore.messaging_legacy_evidence TO :"runtime_role";
GRANT SELECT,INSERT ON eventstore.messaging_admissions,eventstore.outbox_messages,eventstore.outbox_deliveries,
  eventstore.dispatch_messages,eventstore.dispatch_enrollments,eventstore.dispatch_jobs TO :"runtime_role";
GRANT UPDATE(attempts,next_attempt_at,lease_owner,lease_until,sent_at,hold_ref)
  ON eventstore.outbox_deliveries TO :"runtime_role";
GRANT UPDATE(attempts,next_attempt_at,lease_owner,lease_until,completed_at,inbox_consumer_name,inbox_event_id,hold_ref,quarantine_ref)
  ON eventstore.dispatch_jobs TO :"runtime_role";
REVOKE ALL ON FUNCTION eventstore.messaging_immutable(),eventstore.messaging_mode_guard(),
  eventstore.messaging_mode_transition(),eventstore.messaging_admission_shape(),
  eventstore.messaging_inbox_guard(),eventstore.messaging_complete(),eventstore.messaging_delivery_guard(),
  eventstore.messaging_job_guard(),eventstore.messaging_dispatch_route(),
  eventstore.activate_messaging_custody(uuid,bytea,bytea,text,text,text,text) FROM PUBLIC;

-- Audit every SET ROLE reachable identity, including NOINHERIT and default ACLs.
-- Column-level grants must be audited separately from broad table UPDATE.
DO $audit$
DECLARE r record; tab text; col record; allowed text[];
BEGIN
  FOR r IN SELECT oid FROM pg_roles WHERE pg_has_role(current_setting('justix.install_runtime'),oid,'MEMBER') LOOP
    IF has_schema_privilege(r.oid,'eventstore','CREATE') THEN RAISE EXCEPTION 'runtime schema CREATE'; END IF;
    FOREACH tab IN ARRAY ARRAY['messaging_mode','messaging_admissions','messaging_legacy_authorizations',
      'messaging_cutovers','messaging_legacy_evidence','outbox_messages','outbox_deliveries',
      'dispatch_messages','dispatch_enrollments','dispatch_jobs'] LOOP
      IF has_table_privilege(r.oid,'eventstore.'||tab,'UPDATE,DELETE,TRUNCATE,TRIGGER,REFERENCES') THEN
        RAISE EXCEPTION 'unexpected messaging table privileges on %',tab;
      END IF;
      IF tab IN ('messaging_mode','messaging_legacy_authorizations','messaging_cutovers','messaging_legacy_evidence')
        AND has_any_column_privilege(r.oid,'eventstore.'||tab,'INSERT') THEN
        RAISE EXCEPTION 'unexpected migration evidence INSERT';
      END IF;
      allowed:=CASE tab
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
    IF has_function_privilege(r.oid,'eventstore.activate_messaging_custody(uuid,bytea,bytea,text,text,text,text)','EXECUTE') THEN
      RAISE EXCEPTION 'runtime may activate migration';
    END IF;
  END LOOP;
END
$audit$;
COMMIT;
