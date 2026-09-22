-- A retail sale of one VIN to a customer. The vehicle is held in inventory
-- from creation until delivery or cancellation.
CREATE TABLE retail.deals (
    id                     uuid        PRIMARY KEY,
    company_id             uuid        NOT NULL,
    branch_id              uuid        NOT NULL,
    customer_id            uuid        NOT NULL REFERENCES retail.customers,
    lead_id                uuid        REFERENCES retail.leads,
    vehicle_id             uuid        NOT NULL,
    payment_scheme         text        NOT NULL CHECK (payment_scheme IN ('cash', 'own-installment', 'partner-finance')),
    price_minor            text        NOT NULL,
    currency               text        NOT NULL,
    status                 text        NOT NULL CHECK (status IN ('reserved', 'delivered', 'cancelled')),
    contract_signed_on     date,
    contract_reference     text        NOT NULL DEFAULT '',
    registered_on          date,
    plate_number           text        NOT NULL DEFAULT '',
    registration_reference text        NOT NULL DEFAULT '',
    delivered_at           timestamptz,
    status_reason          text        NOT NULL DEFAULT '',
    version                bigint      NOT NULL CHECK (version >= 1),
    created_by             uuid        NOT NULL,
    created_at             timestamptz NOT NULL,
    updated_at             timestamptz NOT NULL
);
-- Local claims: one active sale per vehicle and per lead (inventory holds the global one).
CREATE UNIQUE INDEX deals_active_vehicle_key ON retail.deals (vehicle_id) WHERE status = 'reserved';
CREATE UNIQUE INDEX deals_active_lead_key ON retail.deals (lead_id) WHERE status = 'reserved';
CREATE INDEX deals_company_idx ON retail.deals (company_id, branch_id, status);

-- Retail invoices are records of the deal (e.g. registration), not B2B invoices.
CREATE TABLE retail.invoices (
    id                 uuid        PRIMARY KEY,
    deal_id            uuid        NOT NULL REFERENCES retail.deals,
    company_id         uuid        NOT NULL,
    purpose            text        NOT NULL CHECK (purpose IN ('vehicle-payment', 'first-installment', 'registration')),
    amount_minor       text        NOT NULL,
    currency           text        NOT NULL,
    recipient_snapshot text        NOT NULL,
    due_date           date,
    status             text        NOT NULL CHECK (status IN ('issued', 'void')),
    version            bigint      NOT NULL CHECK (version >= 1),
    created_at         timestamptz NOT NULL
);
CREATE UNIQUE INDEX retail_invoices_purpose_key ON retail.invoices (deal_id, purpose) WHERE status = 'issued';

CREATE TABLE retail.payment_evidence (
    id                 uuid        PRIMARY KEY,
    invoice_id         uuid        NOT NULL REFERENCES retail.invoices,
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
CREATE INDEX retail_evidence_invoice_idx ON retail.payment_evidence (invoice_id);
