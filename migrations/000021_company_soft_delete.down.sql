-- Fails if a deleted company and a live one share a country/registration
-- number: resolve such pairs by hand before rolling back. Deleted companies
-- become visible again after rollback.

DROP INDEX identity.companies_country_registration_key;
CREATE UNIQUE INDEX companies_country_registration_key ON identity.companies (
    (CASE WHEN country_key <> '' THEN country_key ELSE lower(country) END),
    lower(registration_number))
    WHERE registration_number <> '';

ALTER TABLE identity.companies DROP COLUMN deleted_at;
