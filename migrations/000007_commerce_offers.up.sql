-- A supplier's offer. Each version is an immutable snapshot of the commercial
-- terms; publishing makes one exact version visible to partners. Publishing
-- reserves no stock.
CREATE TABLE commerce.offers (
    id                   uuid        PRIMARY KEY,
    supplier_company_id  uuid        NOT NULL,
    status               text        NOT NULL CHECK (status IN ('draft', 'published', 'withdrawn')),
    published_version_id uuid,
    status_reason        text        NOT NULL DEFAULT '',
    version              bigint      NOT NULL CHECK (version >= 1),
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL,
    CHECK ((status = 'published') = (published_version_id IS NOT NULL) OR status = 'withdrawn')
);
CREATE INDEX offers_supplier_idx ON commerce.offers (supplier_company_id);
CREATE INDEX offers_published_idx ON commerce.offers (supplier_company_id) WHERE status = 'published';

CREATE TABLE commerce.offer_versions (
    id            uuid        PRIMARY KEY,
    offer_id      uuid        NOT NULL REFERENCES commerce.offers,
    number        integer     NOT NULL CHECK (number >= 1),
    terms         jsonb       NOT NULL,
    audience_mode text        NOT NULL CHECK (audience_mode IN ('all-active', 'selected')),
    audience_ids  jsonb       NOT NULL DEFAULT '[]',
    created_by    uuid        NOT NULL,
    created_at    timestamptz NOT NULL,
    published_at  timestamptz,
    UNIQUE (offer_id, number)
);
ALTER TABLE commerce.offers ADD FOREIGN KEY (published_version_id) REFERENCES commerce.offer_versions;
