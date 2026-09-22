-- A supplier's invoice for an order: total and payment schedule copied from
-- the order terms when issued. Amounts are exact minor units (decimal text).
CREATE TABLE commerce.invoices (
    id                  uuid        PRIMARY KEY,
    order_id            uuid        NOT NULL REFERENCES commerce.orders,
    supplier_company_id uuid        NOT NULL,
    buyer_company_id    uuid        NOT NULL,
    total_minor         text        NOT NULL,
    currency            text        NOT NULL,
    schedule            jsonb       NOT NULL,
    status              text        NOT NULL CHECK (status IN ('issued', 'void')),
    void_reason         text        NOT NULL DEFAULT '',
    version             bigint      NOT NULL CHECK (version >= 1),
    created_by          uuid        NOT NULL,
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL
);
CREATE UNIQUE INDEX invoices_active_order_key ON commerce.invoices (order_id) WHERE status = 'issued';

-- The buyer's assertion of an external payment; the supplier decides.
-- Accepting records a fact only: no money moves through the platform.
CREATE TABLE commerce.payment_evidence (
    id                 uuid        PRIMARY KEY,
    invoice_id         uuid        NOT NULL REFERENCES commerce.invoices,
    amount_minor       text        NOT NULL,
    currency           text        NOT NULL,
    paid_on            date        NOT NULL,
    external_reference text        NOT NULL,
    status             text        NOT NULL CHECK (status IN ('submitted', 'accepted', 'rejected')),
    decision_reason    text        NOT NULL DEFAULT '',
    submitted_by       uuid        NOT NULL,
    decided_by         uuid,
    version            bigint      NOT NULL CHECK (version >= 1),
    created_at         timestamptz NOT NULL,
    decided_at         timestamptz
);
CREATE INDEX payment_evidence_invoice_idx ON commerce.payment_evidence (invoice_id);
