-- Pure SQL owner migration; never run automatically at application startup.
-- Stage 1: explicitly install pkg/eventstore/schema.sql using psql, connected
-- as justix_commerce to justix_commerce, owner_service=commerce and
-- runtime_role=justix_commerce_runtime (separately provisioned narrow login).
-- Stage 2: the versioned runner uses the same migration login/database and
-- public.schema_migrations (golang-migrate's bigint version/boolean dirty
-- ledger). The runner marks version 1 dirty before executing this file and
-- clean only after success. No down migration or destructive recovery exists.
BEGIN;
SET LOCAL search_path = pg_catalog;
DO $guard$
BEGIN
  IF current_database() <> 'justix_commerce' OR current_user <> 'justix_commerce' THEN
    RAISE EXCEPTION 'commerce migration requires its private database and migration owner';
  END IF;
  IF (SELECT count(*) FROM public.schema_migrations) <> 1 OR
    NOT EXISTS (SELECT FROM public.schema_migrations WHERE version=1 AND dirty) THEN
    RAISE EXCEPTION 'commerce migration requires the runner dirty version 1 ledger';
  END IF;
  IF (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
      WHERE n.nspname='eventstore' AND c.relkind='r' AND c.relname IN
      ('events','command_receipts','outbox','inbox','operations','operation_steps')) <> 6 THEN
    RAISE EXCEPTION 'install the approved shared mechanics first';
  END IF;
  IF NOT EXISTS (SELECT FROM pg_attrdef d JOIN pg_attribute a
      ON a.attrelid=d.adrelid AND a.attnum=d.adnum
      WHERE d.adrelid='eventstore.events'::regclass AND a.attname='owner_service'
        AND pg_get_expr(d.adbin,d.adrelid)='''commerce''::text') THEN
    RAISE EXCEPTION 'shared mechanics belong to another owner';
  END IF;
END
$guard$;
CREATE SCHEMA commerce_mechanics;
REVOKE ALL ON SCHEMA commerce_mechanics FROM PUBLIC;
CREATE TABLE commerce_mechanics.compatibility (
  singleton boolean PRIMARY KEY CHECK (singleton),
  owner_service text NOT NULL CHECK (owner_service='commerce'),
  mechanics_version bigint NOT NULL CHECK (mechanics_version=1)
);
INSERT INTO commerce_mechanics.compatibility VALUES (true,'commerce',1);
REVOKE ALL ON commerce_mechanics.compatibility FROM PUBLIC;
GRANT USAGE ON SCHEMA commerce_mechanics, public TO justix_commerce_runtime;
GRANT SELECT ON commerce_mechanics.compatibility, public.schema_migrations TO justix_commerce_runtime;
COMMIT;
