-- Pure SQL owner migration; never run automatically at application startup.
-- Stage 1: explicitly install pkg/eventstore/schema.sql using psql, connected
-- as justix_inventory to justix_inventory, owner_service=inventory and
-- runtime_role=justix_inventory_runtime (separately provisioned narrow login).
-- Stage 2: the versioned runner uses the same migration login/database and
-- public.schema_migrations (golang-migrate's bigint version/boolean dirty
-- ledger). The runner marks version 1 dirty before executing this file and
-- clean only after success. No down migration or destructive recovery exists.
BEGIN;
SET LOCAL search_path = pg_catalog;
DO $guard$
BEGIN
  IF current_database() <> 'justix_inventory' OR current_user <> 'justix_inventory' THEN
    RAISE EXCEPTION 'inventory migration requires its private database and migration owner';
  END IF;
  IF (SELECT count(*) FROM public.schema_migrations) <> 1 OR
    NOT EXISTS (SELECT FROM public.schema_migrations WHERE version=1 AND dirty) THEN
    RAISE EXCEPTION 'inventory migration requires the runner dirty version 1 ledger';
  END IF;
  IF (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
      WHERE n.nspname='eventstore' AND c.relkind='r' AND c.relname IN
      ('events','command_receipts','outbox','inbox','operations','operation_steps')) <> 6 THEN
    RAISE EXCEPTION 'install the approved shared mechanics first';
  END IF;
  IF NOT EXISTS (SELECT FROM pg_attrdef d JOIN pg_attribute a
      ON a.attrelid=d.adrelid AND a.attnum=d.adnum
      WHERE d.adrelid='eventstore.events'::regclass AND a.attname='owner_service'
        AND pg_get_expr(d.adbin,d.adrelid)='''inventory''::text') THEN
    RAISE EXCEPTION 'shared mechanics belong to another owner';
  END IF;
END
$guard$;
CREATE SCHEMA inventory_mechanics;
REVOKE ALL ON SCHEMA inventory_mechanics FROM PUBLIC;
CREATE TABLE inventory_mechanics.compatibility (
  singleton boolean PRIMARY KEY CHECK (singleton),
  owner_service text NOT NULL CHECK (owner_service='inventory'),
  mechanics_version bigint NOT NULL CHECK (mechanics_version=1)
);
INSERT INTO inventory_mechanics.compatibility VALUES (true,'inventory',1);
REVOKE ALL ON inventory_mechanics.compatibility FROM PUBLIC;
GRANT USAGE ON SCHEMA inventory_mechanics, public TO justix_inventory_runtime;
GRANT SELECT ON inventory_mechanics.compatibility, public.schema_migrations TO justix_inventory_runtime;
COMMIT;
