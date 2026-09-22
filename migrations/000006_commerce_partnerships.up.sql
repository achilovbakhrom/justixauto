CREATE SCHEMA IF NOT EXISTS commerce;

-- A B2B relationship between two companies. Only an active partnership opens
-- offers, prices, stock and new deals; ended ones keep their history.
CREATE TABLE commerce.partnerships (
    id                   uuid        PRIMARY KEY,
    requester_company_id uuid        NOT NULL,
    recipient_company_id uuid        NOT NULL,
    status               text        NOT NULL CHECK (status IN ('requested', 'active', 'declined', 'withdrawn', 'ended')),
    status_reason        text        NOT NULL DEFAULT '',
    requested_by         uuid        NOT NULL,
    version              bigint      NOT NULL CHECK (version >= 1),
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL,
    activated_at         timestamptz,
    closed_at            timestamptz,
    CHECK (requester_company_id <> recipient_company_id)
);
-- At most one open (requested or active) partnership per pair of companies.
CREATE UNIQUE INDEX partnerships_open_pair_key ON commerce.partnerships (
    least(requester_company_id, recipient_company_id), greatest(requester_company_id, recipient_company_id))
    WHERE status IN ('requested', 'active');
CREATE INDEX partnerships_requester_idx ON commerce.partnerships (requester_company_id);
CREATE INDEX partnerships_recipient_idx ON commerce.partnerships (recipient_company_id);

-- Append-only history of commerce decisions (who, what, when, why).
CREATE TABLE commerce.events (
    id            uuid        PRIMARY KEY,
    seq           bigint      GENERATED ALWAYS AS IDENTITY UNIQUE,
    company_id    uuid        NOT NULL,
    event_type    text        NOT NULL,
    resource_type text        NOT NULL,
    resource_id   uuid        NOT NULL,
    actor_user_id uuid        NOT NULL,
    occurred_at   timestamptz NOT NULL,
    reason        text        NOT NULL DEFAULT '',
    details       jsonb       NOT NULL DEFAULT '{}'
);
CREATE INDEX events_resource_idx ON commerce.events (resource_type, resource_id, seq);
