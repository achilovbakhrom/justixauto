DROP TABLE IF EXISTS identity.audit_events;
DROP TABLE IF EXISTS identity.session_branches;
DROP TABLE IF EXISTS identity.sessions;
DROP TABLE IF EXISTS identity.membership_branches;
DROP TABLE IF EXISTS identity.memberships;
DROP TABLE IF EXISTS identity.branches;
DROP TABLE IF EXISTS identity.user_roles;
DROP TABLE IF EXISTS identity.role_permissions;
DROP TABLE IF EXISTS identity.roles;
DROP TABLE IF EXISTS identity.users;

DROP INDEX identity.companies_country_registration_key;
ALTER TABLE identity.companies
    DROP COLUMN legal_name, DROP COLUMN country_key, DROP COLUMN region_key,
    DROP COLUMN email, DROP COLUMN address, DROP COLUMN phone,
    DROP CONSTRAINT companies_kind_check;
UPDATE identity.companies SET kind = 'insurer' WHERE kind = 'insurance';
ALTER TABLE identity.companies
    ADD CONSTRAINT companies_kind_check CHECK (kind IN ('seller', 'bank', 'mfo', 'insurer'));
CREATE UNIQUE INDEX companies_country_registration_key
    ON identity.companies (lower(country), registration_number);
