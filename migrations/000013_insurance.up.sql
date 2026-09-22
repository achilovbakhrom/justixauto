CREATE SCHEMA IF NOT EXISTS insurance;

-- An own-installment sale sent to an insurer for a decision. One per sale.
CREATE TABLE insurance.applications (
    id                 uuid        PRIMARY KEY,
    seller_company_id  uuid        NOT NULL,
    insurer_company_id uuid        NOT NULL,
    deal_id            uuid        NOT NULL UNIQUE,
    status             text        NOT NULL CHECK (status IN ('draft', 'submitted', 'review', 'needs-info', 'approved', 'declined')),
    note               text        NOT NULL DEFAULT '',
    snapshot           jsonb,      -- sale facts frozen at submission
    version            bigint      NOT NULL CHECK (version >= 1),
    created_by         uuid        NOT NULL,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL,
    submitted_at       timestamptz,
    decided_at         timestamptz
);
CREATE INDEX applications_insurer_idx ON insurance.applications (insurer_company_id, status) WHERE status <> 'draft';

-- The shared history both sides see: requests, responses, decisions.
CREATE TABLE insurance.messages (
    id             uuid        PRIMARY KEY,
    seq            bigint      GENERATED ALWAYS AS IDENTITY UNIQUE,
    application_id uuid        NOT NULL REFERENCES insurance.applications,
    kind           text        NOT NULL CHECK (kind IN ('submitted', 'taken', 'request', 'response', 'approved', 'declined')),
    request_id     uuid,
    note           text        NOT NULL DEFAULT '',
    company_id     uuid        NOT NULL,
    actor_user_id  uuid        NOT NULL,
    created_at     timestamptz NOT NULL
);
CREATE INDEX messages_application_idx ON insurance.messages (application_id, seq);
