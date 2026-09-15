\set ON_ERROR_STOP on
-- Explicit, one-time owner migration (not an application startup script):
-- psql -X -d <owner-db> -U <migration-owner> -v owner_service=inventory \
--   -v runtime_role=justix_inventory_runtime -f pkg/eventstore/schema.sql
-- Provision runtime_role separately, without database/schema ownership, role
-- administration or membership in the migration owner. Never run the service
-- using T-005's database-owner credential. This template creates no credentials.
-- Reinstallation fails atomically; future changes require versioned migrations.
BEGIN;
SET LOCAL search_path = pg_catalog;
SELECT set_config('justix.install_owner', :'owner_service', true);
SELECT set_config('justix.install_runtime', :'runtime_role', true);
DO $guard$
DECLARE
  runtime_name text := current_setting('justix.install_runtime');
  runtime_record pg_roles%ROWTYPE;
  database_owner oid;
BEGIN
  IF current_setting('justix.install_owner') NOT IN
    ('identity','inventory','commerce','retail','financing','insurance','documents') THEN
    RAISE EXCEPTION 'unknown owner service';
  END IF;
  SELECT * INTO runtime_record FROM pg_roles WHERE rolname = runtime_name;
  IF NOT FOUND THEN RAISE EXCEPTION 'runtime role must already exist'; END IF;
  SELECT datdba INTO database_owner FROM pg_database WHERE datname = current_database();
  IF runtime_record.rolsuper OR runtime_record.rolcreatedb OR runtime_record.rolcreaterole
    OR runtime_record.rolreplication OR runtime_record.rolbypassrls
    OR pg_has_role(runtime_record.oid, database_owner, 'MEMBER')
    OR pg_has_role(runtime_record.oid, current_user, 'MEMBER') THEN
    RAISE EXCEPTION 'runtime role must be unprivileged and separate from migration/database owner';
  END IF;
  -- Role attributes alone miss privileges gained through membership (including
  -- SET ROLE through a NOINHERIT chain). Predefined cluster roles are unsuitable
  -- for a service's narrowly scoped writer identity.
  IF EXISTS (SELECT 1 FROM pg_roles r
    WHERE r.oid <> runtime_record.oid
      AND pg_has_role(runtime_record.oid, r.oid, 'MEMBER')
      AND (r.rolname LIKE 'pg\_%' ESCAPE '\' OR r.rolsuper OR r.rolcreatedb
        OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls)) THEN
    RAISE EXCEPTION 'runtime role has privileged role membership';
  END IF;
END
$guard$;

CREATE SCHEMA eventstore;
REVOKE ALL ON SCHEMA eventstore FROM PUBLIC;
GRANT USAGE ON SCHEMA eventstore TO :"runtime_role";

-- All stream identities are local to this owner database. Aggregate revisions
-- and integration positions are distinct; internal-only events have NULL
-- integration_sequence. Conditional contiguous append belongs to T-009.
CREATE TABLE eventstore.events (
  event_id uuid PRIMARY KEY,
  event_type text NOT NULL CHECK (event_type ~ '^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)+\.v[1-9][0-9]*$'),
  owner_service text NOT NULL DEFAULT :'owner_service' CHECK (owner_service = :'owner_service'),
  schema_version bigint NOT NULL CHECK (schema_version BETWEEN 1 AND 4294967295),
  aggregate_type text NOT NULL CHECK (aggregate_type ~ '^[a-z][a-z0-9-]*$'),
  aggregate_id uuid NOT NULL,
  aggregate_version bigint NOT NULL CHECK (aggregate_version > 0),
  integration_sequence bigint CHECK (integration_sequence > 0),
  company_id uuid CHECK (company_id IS NOT NULL OR :'owner_service' = 'identity'),
  occurred_at timestamptz NOT NULL CHECK (isfinite(occurred_at)),
  actor_kind text NOT NULL CHECK (actor_kind IN ('user','service','system')),
  actor_id uuid NOT NULL,
  correlation_id uuid NOT NULL,
  causation_id uuid NOT NULL,
  operation_id uuid NOT NULL,
  data jsonb NOT NULL CHECK (jsonb_typeof(data) = 'object'),
  UNIQUE (aggregate_type, aggregate_id, aggregate_version),
  UNIQUE (aggregate_type, aggregate_id, integration_sequence),
  CHECK (split_part(event_type, '.', 1) = owner_service),
  CHECK (event_type LIKE '%.' || 'v' || schema_version::text)
);

-- No raw request body: request_hash is the canonical nonsecret digest/HMAC.
-- Receipt data is a safe typed outcome, never a password/token or private body.
-- NULL company/target participates in uniqueness (global commands/creates).
CREATE TABLE eventstore.command_receipts (
  receipt_id uuid PRIMARY KEY,
  actor_id uuid NOT NULL,
  company_id uuid CHECK (company_id IS NOT NULL OR :'owner_service' = 'identity'),
  command_name text NOT NULL CHECK (length(command_name) > 0),
  target_id uuid,
  idempotency_key uuid NOT NULL,
  request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
  http_status integer NOT NULL CHECK (http_status IN (200,201,202)),
  receipt jsonb NOT NULL CHECK (jsonb_typeof(receipt) = 'object'),
  committed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  UNIQUE NULLS NOT DISTINCT (actor_id, company_id, command_name, target_id, idempotency_key)
);

-- Byte identity/routing is immutable to the runtime. Only relay scheduling
-- columns may change. No FK to events: integration serialization is a separate
-- owner-approved contract, inserted in the same transaction by T-011.
CREATE TABLE eventstore.outbox (
  event_id uuid PRIMARY KEY,
  aggregate_type text NOT NULL CHECK (aggregate_type ~ '^[a-z][a-z0-9-]*$'),
  aggregate_id uuid NOT NULL,
  integration_sequence bigint NOT NULL CHECK (integration_sequence > 0),
  envelope bytea NOT NULL CHECK (octet_length(envelope) > 0),
  envelope_hash bytea NOT NULL CHECK (envelope_hash = sha256(envelope)),
  exchange text NOT NULL CHECK (length(exchange) > 0),
  routing_key text NOT NULL CHECK (length(routing_key) > 0),
  attempts bigint NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  lease_owner uuid,
  lease_until timestamptz,
  sent_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  UNIQUE (aggregate_type, aggregate_id, integration_sequence),
  CHECK ((lease_owner IS NULL) = (lease_until IS NULL))
);
CREATE INDEX outbox_due ON eventstore.outbox (next_attempt_at, event_id) WHERE sent_at IS NULL;

-- Consumer names distinguish projection generations/process subscribers. Inbox
-- uniqueness alone does not establish ordered delivery or exactly-once effects.
CREATE TABLE eventstore.inbox (
  consumer_name text NOT NULL CHECK (length(consumer_name) > 0),
  event_id uuid NOT NULL,
  envelope_hash bytea NOT NULL CHECK (octet_length(envelope_hash) = 32),
  processed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (consumer_name, event_id)
);

CREATE TABLE eventstore.operations (
  operation_id uuid PRIMARY KEY,
  kind text NOT NULL CHECK (length(kind) > 0),
  aggregate_id uuid NOT NULL,
  actor_id uuid NOT NULL,
  company_id uuid CHECK (company_id IS NOT NULL OR :'owner_service' = 'identity'),
  admission_ref uuid NOT NULL,
  intent_revision bigint NOT NULL CHECK (intent_revision >= 0),
  request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
  phase text NOT NULL CHECK (length(phase) > 0),
  decision text NOT NULL DEFAULT 'undecided' CHECK (decision IN ('undecided','commit','abort')),
  attention_required boolean NOT NULL DEFAULT false,
  result jsonb CHECK (jsonb_typeof(result) = 'object'),
  error jsonb CHECK (jsonb_typeof(error) = 'object'),
  attempts bigint NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  lease_owner uuid,
  lease_until timestamptz,
  revision bigint NOT NULL CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  CHECK ((lease_owner IS NULL) = (lease_until IS NULL))
);
CREATE INDEX operations_due ON eventstore.operations (next_attempt_at, operation_id);
CREATE INDEX operations_scope ON eventstore.operations (company_id, actor_id, operation_id);

-- A durable commit/abort decision cannot be reversed, including by resetting to
-- undecided. Leases and timestamps have no effect on this business decision.
CREATE FUNCTION eventstore.preserve_operation_decision() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $body$
BEGIN
  IF OLD.decision <> 'undecided' AND NEW.decision <> OLD.decision THEN
    RAISE EXCEPTION 'operation decision is immutable' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END
$body$;
REVOKE ALL ON FUNCTION eventstore.preserve_operation_decision() FROM PUBLIC;
CREATE TRIGGER preserve_operation_decision BEFORE UPDATE OF decision
ON eventstore.operations FOR EACH ROW EXECUTE FUNCTION eventstore.preserve_operation_decision();

-- Completed step receipts are append-only. Pending work stays in operations;
-- insertion of each step outcome and its local effects is one owner transaction.
CREATE TABLE eventstore.operation_steps (
  operation_id uuid NOT NULL REFERENCES eventstore.operations(operation_id),
  step text NOT NULL CHECK (length(step) > 0),
  request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
  result jsonb NOT NULL CHECK (jsonb_typeof(result) = 'object'),
  completed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (operation_id, step)
);

REVOKE ALL ON ALL TABLES IN SCHEMA eventstore FROM PUBLIC;
GRANT SELECT, INSERT ON eventstore.events, eventstore.command_receipts,
  eventstore.outbox, eventstore.inbox, eventstore.operations, eventstore.operation_steps TO :"runtime_role";
GRANT UPDATE (attempts, next_attempt_at, lease_owner, lease_until, sent_at)
  ON eventstore.outbox TO :"runtime_role";
GRANT UPDATE (phase, decision, attention_required, result, error, attempts,
  next_attempt_at, lease_owner, lease_until, revision, updated_at)
  ON eventstore.operations TO :"runtime_role";
-- Deliberately no DELETE/TRUNCATE, schema CREATE, table ownership, automatic
-- retention/purge, expiry-driven release, or broad default privileges.
DO $permissions$
DECLARE
  role_record record;
  table_name text;
BEGIN
  -- Existing ALTER DEFAULT PRIVILEGES or ordinary role memberships must not
  -- silently broaden this installation's grants. Inspect every role reachable
  -- with SET ROLE, not only currently inherited privileges.
  FOR role_record IN SELECT oid FROM pg_roles
    WHERE pg_has_role(current_setting('justix.install_runtime'), oid, 'MEMBER') LOOP
    IF has_schema_privilege(role_record.oid, 'eventstore', 'CREATE') THEN
      RAISE EXCEPTION 'runtime role can create schema objects';
    END IF;
    FOREACH table_name IN ARRAY ARRAY['events','command_receipts','outbox','inbox','operations','operation_steps'] LOOP
      IF has_table_privilege(role_record.oid, 'eventstore.' || table_name, 'DELETE,TRUNCATE,TRIGGER,REFERENCES')
        OR has_table_privilege(role_record.oid, 'eventstore.' || table_name, 'UPDATE') THEN
        RAISE EXCEPTION 'runtime role has unexpected table privileges on %', table_name;
      END IF;
    END LOOP;
  END LOOP;
END
$permissions$;
COMMIT;
