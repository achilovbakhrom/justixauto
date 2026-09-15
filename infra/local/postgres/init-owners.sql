\set ON_ERROR_STOP on

-- T-005 local-only bootstrap. The official PostgreSQL entrypoint runs this as
-- the bootstrap superuser. Passwords must be supplied through environment
-- variables; no credential has a repository default.
\set identity_password ''
\set inventory_password ''
\set commerce_password ''
\set retail_password ''
\set financing_password ''
\set insurance_password ''
\set documents_password ''
\getenv identity_password JUSTIXAUTO_IDENTITY_DB_PASSWORD
\getenv inventory_password JUSTIXAUTO_INVENTORY_DB_PASSWORD
\getenv commerce_password JUSTIXAUTO_COMMERCE_DB_PASSWORD
\getenv retail_password JUSTIXAUTO_RETAIL_DB_PASSWORD
\getenv financing_password JUSTIXAUTO_FINANCING_DB_PASSWORD
\getenv insurance_password JUSTIXAUTO_INSURANCE_DB_PASSWORD
\getenv documents_password JUSTIXAUTO_DOCUMENTS_DB_PASSWORD

SELECT
  :'identity_password' <> '' AS identity_password_present,
  :'inventory_password' <> '' AS inventory_password_present,
  :'commerce_password' <> '' AS commerce_password_present,
  :'retail_password' <> '' AS retail_password_present,
  :'financing_password' <> '' AS financing_password_present,
  :'insurance_password' <> '' AS insurance_password_present,
  :'documents_password' <> '' AS documents_password_present
\gset

-- ON_ERROR_STOP propagates this server error through psql and the official
-- entrypoint. PostgreSQL 18's psql \quit command has no exit-status argument,
-- so it must not be used as a configuration-failure signal.
\if :identity_password_present
\else
  \echo 'JUSTIXAUTO_IDENTITY_DB_PASSWORD is required and must not be empty.'
  SELECT 1 / 0 AS missing_required_owner_credential;
\endif
\if :inventory_password_present
\else
  \echo 'JUSTIXAUTO_INVENTORY_DB_PASSWORD is required and must not be empty.'
  SELECT 1 / 0 AS missing_required_owner_credential;
\endif
\if :commerce_password_present
\else
  \echo 'JUSTIXAUTO_COMMERCE_DB_PASSWORD is required and must not be empty.'
  SELECT 1 / 0 AS missing_required_owner_credential;
\endif
\if :retail_password_present
\else
  \echo 'JUSTIXAUTO_RETAIL_DB_PASSWORD is required and must not be empty.'
  SELECT 1 / 0 AS missing_required_owner_credential;
\endif
\if :financing_password_present
\else
  \echo 'JUSTIXAUTO_FINANCING_DB_PASSWORD is required and must not be empty.'
  SELECT 1 / 0 AS missing_required_owner_credential;
\endif
\if :insurance_password_present
\else
  \echo 'JUSTIXAUTO_INSURANCE_DB_PASSWORD is required and must not be empty.'
  SELECT 1 / 0 AS missing_required_owner_credential;
\endif
\if :documents_password_present
\else
  \echo 'JUSTIXAUTO_DOCUMENTS_DB_PASSWORD is required and must not be empty.'
  SELECT 1 / 0 AS missing_required_owner_credential;
\endif

CREATE TEMP TABLE justix_owner_bootstrap (
  owner_name name PRIMARY KEY,
  database_name name NOT NULL UNIQUE,
  owner_password text NOT NULL
);

INSERT INTO justix_owner_bootstrap (owner_name, database_name, owner_password)
VALUES
  ('justix_identity',  'justix_identity',  :'identity_password'),
  ('justix_inventory', 'justix_inventory', :'inventory_password'),
  ('justix_commerce',  'justix_commerce',  :'commerce_password'),
  ('justix_retail',     'justix_retail',     :'retail_password'),
  ('justix_financing', 'justix_financing', :'financing_password'),
  ('justix_insurance', 'justix_insurance', :'insurance_password'),
  ('justix_documents', 'justix_documents', :'documents_password');

SELECT format(
  'CREATE ROLE %I LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD %L',
  owner_name,
  owner_password
)
FROM justix_owner_bootstrap
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = owner_name)
ORDER BY owner_name
\gexec

-- Reassert the restricted capability set if a local data volume is reused.
SELECT format(
  'ALTER ROLE %I LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD %L',
  owner_name,
  owner_password
)
FROM justix_owner_bootstrap
ORDER BY owner_name
\gexec

SELECT format(
  'CREATE DATABASE %I OWNER %I ENCODING %L TEMPLATE template0',
  database_name,
  owner_name,
  'UTF8'
)
FROM justix_owner_bootstrap
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = database_name)
ORDER BY database_name
\gexec

SELECT format('ALTER DATABASE %I OWNER TO %I', database_name, owner_name)
FROM justix_owner_bootstrap
ORDER BY database_name
\gexec

SELECT format('REVOKE ALL PRIVILEGES ON DATABASE %I FROM PUBLIC', database_name)
FROM justix_owner_bootstrap
ORDER BY database_name
\gexec

-- These explicit pairwise revocations make the isolation invariant auditable
-- even if a reused local cluster had previously granted a service role access.
SELECT format(
  'REVOKE ALL PRIVILEGES ON DATABASE %I FROM %I',
  target.database_name,
  actor.owner_name
)
FROM justix_owner_bootstrap AS target
CROSS JOIN justix_owner_bootstrap AS actor
WHERE target.owner_name <> actor.owner_name
ORDER BY target.database_name, actor.owner_name
\gexec

SELECT format(
  'GRANT CONNECT, TEMPORARY ON DATABASE %I TO %I',
  database_name,
  owner_name
)
FROM justix_owner_bootstrap
ORDER BY database_name
\gexec

\connect justix_identity
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA public TO justix_identity;

\connect justix_inventory
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA public TO justix_inventory;

\connect justix_commerce
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA public TO justix_commerce;

\connect justix_retail
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA public TO justix_retail;

\connect justix_financing
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA public TO justix_financing;

\connect justix_insurance
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA public TO justix_insurance;

\connect justix_documents
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA public TO justix_documents;
