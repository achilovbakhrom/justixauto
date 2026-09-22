CREATE SCHEMA IF NOT EXISTS identity;

CREATE TABLE identity.companies (
    id                  uuid        PRIMARY KEY,
    kind                text        NOT NULL CHECK (kind IN ('seller', 'bank', 'mfo', 'insurer')),
    name                text        NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    country             text        NOT NULL CHECK (length(country) BETWEEN 1 AND 100),
    region              text        NOT NULL DEFAULT '' CHECK (length(region) <= 100),
    registration_number text        NOT NULL CHECK (length(registration_number) BETWEEN 1 AND 64),
    status              text        NOT NULL CHECK (status IN ('draft', 'active', 'suspended')),
    status_reason       text        NOT NULL DEFAULT '',
    version             bigint      NOT NULL CHECK (version >= 1),
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL
);

-- Duplicate companies are detected by country + registration number.
CREATE UNIQUE INDEX companies_country_registration_key
    ON identity.companies (lower(country), registration_number);
