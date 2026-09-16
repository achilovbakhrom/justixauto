-- Pure SQL owner migration; never run automatically at application startup.
-- Stage 1: explicitly install pkg/eventstore/schema.sql using psql, connected
-- as justix_insurance to justix_insurance, owner_service=insurance and
-- runtime_role=justix_insurance_runtime (separately provisioned narrow login).
-- Stage 2: the versioned runner uses the same migration login/database and
-- public.schema_migrations (golang-migrate's bigint version/boolean dirty
-- ledger). The runner marks version 1 dirty before executing this file and
-- clean only after success. No down migration or destructive recovery exists.
BEGIN;
SET LOCAL search_path = pg_catalog;
DO $guard$
BEGIN
  IF current_database() <> 'justix_insurance' OR current_user <> 'justix_insurance' THEN
    RAISE EXCEPTION 'insurance migration requires its private database and migration owner';
  END IF;
  IF (SELECT count(*) FROM public.schema_migrations) <> 1 OR
    NOT EXISTS (SELECT FROM public.schema_migrations WHERE version=1 AND dirty) THEN
    RAISE EXCEPTION 'insurance migration requires the runner dirty version 1 ledger';
  END IF;
  IF (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
      WHERE n.nspname='eventstore' AND c.relkind='r' AND c.relname IN
      ('events','command_receipts','outbox','inbox','operations','operation_steps')) <> 6 THEN
    RAISE EXCEPTION 'install the approved shared mechanics first';
  END IF;
  IF NOT EXISTS (SELECT FROM pg_attrdef d JOIN pg_attribute a
      ON a.attrelid=d.adrelid AND a.attnum=d.adnum
      WHERE d.adrelid='eventstore.events'::regclass AND a.attname='owner_service'
        AND pg_get_expr(d.adbin,d.adrelid)='''insurance''::text') THEN
    RAISE EXCEPTION 'shared mechanics belong to another owner';
  END IF;
END
$guard$;
CREATE SCHEMA insurance_mechanics;
REVOKE ALL ON SCHEMA insurance_mechanics FROM PUBLIC;
CREATE TABLE insurance_mechanics.compatibility (
  singleton boolean PRIMARY KEY CHECK (singleton),
  owner_service text NOT NULL CHECK (owner_service='insurance'),
  mechanics_version bigint NOT NULL CHECK (mechanics_version=1)
);
INSERT INTO insurance_mechanics.compatibility VALUES (true,'insurance',1);
REVOKE ALL ON insurance_mechanics.compatibility FROM PUBLIC;
GRANT USAGE ON SCHEMA insurance_mechanics, public TO justix_insurance_runtime;
GRANT SELECT ON insurance_mechanics.compatibility, public.schema_migrations TO justix_insurance_runtime;
COMMIT;
