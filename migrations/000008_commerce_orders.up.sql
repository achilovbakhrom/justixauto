-- Request for quotation: a buyer asks a partner supplier for models and quantities.
CREATE TABLE commerce.rfqs (
    id                  uuid        PRIMARY KEY,
    buyer_company_id    uuid        NOT NULL,
    supplier_company_id uuid        NOT NULL,
    offer_version_id    uuid        REFERENCES commerce.offer_versions,
    lines               jsonb       NOT NULL,
    status              text        NOT NULL CHECK (status IN ('draft', 'sent', 'negotiating', 'accepted', 'declined', 'cancelled')),
    status_reason       text        NOT NULL DEFAULT '',
    version             bigint      NOT NULL CHECK (version >= 1),
    created_by          uuid        NOT NULL,
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL,
    CHECK (buyer_company_id <> supplier_company_id)
);
CREATE INDEX rfqs_buyer_idx ON commerce.rfqs (buyer_company_id);
CREATE INDEX rfqs_supplier_idx ON commerce.rfqs (supplier_company_id) WHERE status <> 'draft';

-- The supplier's numbered, immutable answers. The digest pins exact terms.
CREATE TABLE commerce.quotations (
    id         uuid        PRIMARY KEY,
    rfq_id     uuid        NOT NULL REFERENCES commerce.rfqs,
    number     integer     NOT NULL CHECK (number >= 1),
    terms      jsonb       NOT NULL,
    digest     text        NOT NULL,
    created_by uuid        NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (rfq_id, number)
);

-- A purchase order with the terms captured when it was created.
CREATE TABLE commerce.orders (
    id                  uuid        PRIMARY KEY,
    buyer_company_id    uuid        NOT NULL,
    supplier_company_id uuid        NOT NULL,
    source              text        NOT NULL CHECK (source IN ('rfq', 'offer')),
    rfq_id              uuid        REFERENCES commerce.rfqs,
    quotation_id        uuid        UNIQUE REFERENCES commerce.quotations, -- one order per accepted quotation
    offer_version_id    uuid        REFERENCES commerce.offer_versions,
    terms               jsonb       NOT NULL, -- current agreed terms (original or latest accepted addendum)
    status              text        NOT NULL CHECK (status IN ('awaiting-supplier', 'accepted', 'fulfilling', 'completed', 'cancelled')),
    status_reason       text        NOT NULL DEFAULT '',
    version             bigint      NOT NULL CHECK (version >= 1),
    created_by          uuid        NOT NULL,
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL
);
CREATE INDEX orders_buyer_idx ON commerce.orders (buyer_company_id);
CREATE INDEX orders_supplier_idx ON commerce.orders (supplier_company_id);

-- Changes to an accepted order, agreed by both parties. The original terms
-- stay in the order history; each addendum is immutable.
CREATE TABLE commerce.order_addenda (
    id                  uuid        PRIMARY KEY,
    order_id            uuid        NOT NULL REFERENCES commerce.orders,
    number              integer     NOT NULL CHECK (number >= 1),
    terms               jsonb       NOT NULL,
    reason              text        NOT NULL,
    proposed_by_company uuid        NOT NULL,
    status              text        NOT NULL CHECK (status IN ('proposed', 'accepted', 'rejected')),
    decision_reason     text        NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL,
    decided_at          timestamptz,
    UNIQUE (order_id, number)
);
-- At most one open proposal per order.
CREATE UNIQUE INDEX order_addenda_open_key ON commerce.order_addenda (order_id) WHERE status = 'proposed';
