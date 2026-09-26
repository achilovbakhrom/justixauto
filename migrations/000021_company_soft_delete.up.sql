-- User decision 2026-09-26: companies are soft-deleted. A deleted company keeps
-- its row, memberships and history for audit and old records, but is hidden
-- from every list, directory and company page, and its memberships no longer
-- grant access. Its registration number no longer blocks a new registration.

ALTER TABLE identity.companies ADD COLUMN deleted_at timestamptz;

DROP INDEX identity.companies_country_registration_key;
CREATE UNIQUE INDEX companies_country_registration_key ON identity.companies (
    (CASE WHEN country_key <> '' THEN country_key ELSE lower(country) END),
    lower(registration_number))
    WHERE registration_number <> '' AND deleted_at IS NULL;
