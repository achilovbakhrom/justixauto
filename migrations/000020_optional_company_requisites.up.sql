-- User decision 2026-09-26: creating a company only requires the company
-- name and the first administrator's login (plus password and its
-- confirmation, without which sign-in is impossible). Country, registration
-- number and e-mail become optional requisites that arrive later from a
-- government-source integration; the registration number is never entered in
-- a form. Duplicate detection by country/registration therefore only applies
-- once a registration number is known.

ALTER TABLE identity.companies DROP CONSTRAINT companies_country_check;
ALTER TABLE identity.companies
    ADD CONSTRAINT companies_country_check CHECK (length(country) <= 100);

ALTER TABLE identity.companies DROP CONSTRAINT companies_registration_number_check;
ALTER TABLE identity.companies
    ADD CONSTRAINT companies_registration_number_check CHECK (length(registration_number) <= 64);

DROP INDEX identity.companies_country_registration_key;
CREATE UNIQUE INDEX companies_country_registration_key ON identity.companies (
    (CASE WHEN country_key <> '' THEN country_key ELSE lower(country) END),
    lower(registration_number))
    WHERE registration_number <> '';

-- Users: the first administrator's e-mail is optional too, so an empty
-- e-mail must neither fail the length check nor collide with any other
-- empty-e-mail user.
ALTER TABLE identity.users DROP CONSTRAINT users_email_check;
ALTER TABLE identity.users
    ADD CONSTRAINT users_email_check CHECK (email = '' OR length(email) BETWEEN 3 AND 254);

DROP INDEX identity.users_email_key;
CREATE UNIQUE INDEX users_email_key ON identity.users (lower(email)) WHERE email <> '';
