-- One active hold per vehicle across all deals (wholesale and retail): the
-- global arbiter against selling a VIN twice. No expiry.
CREATE TABLE inventory.reservations (
    id          uuid        PRIMARY KEY,
    vehicle_id  uuid        NOT NULL REFERENCES inventory.vehicle_units,
    company_id  uuid        NOT NULL,
    holder_type text        NOT NULL CHECK (holder_type IN ('commerce-order', 'retail-deal')),
    holder_id   uuid        NOT NULL,
    status      text        NOT NULL CHECK (status IN ('held', 'released', 'finalized')),
    reason      text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL,
    closed_at   timestamptz
);
CREATE UNIQUE INDEX reservations_held_vehicle_key ON inventory.reservations (vehicle_id) WHERE status = 'held';
CREATE INDEX reservations_holder_idx ON inventory.reservations (holder_type, holder_id);

-- Concrete VINs allocated to order lines.
CREATE TABLE commerce.order_allocations (
    id          uuid        PRIMARY KEY,
    order_id    uuid        NOT NULL REFERENCES commerce.orders,
    line_id     uuid        NOT NULL,
    vehicle_id  uuid        NOT NULL,
    vin         text        NOT NULL,
    status      text        NOT NULL CHECK (status IN ('allocated', 'shipped', 'delivered', 'rejected', 'released')),
    shipment_id uuid,
    created_at  timestamptz NOT NULL
);
-- A vehicle counts once per order while live; rejected ones may be re-allocated.
CREATE UNIQUE INDEX order_allocations_live_key ON commerce.order_allocations (order_id, vehicle_id)
    WHERE status IN ('allocated', 'shipped', 'delivered');

CREATE TABLE commerce.shipments (
    id          uuid        PRIMARY KEY,
    order_id    uuid        NOT NULL REFERENCES commerce.orders,
    route       text        NOT NULL CHECK (route IN ('factory', 'foreign-direct', 'in-transit', 'local')),
    status      text        NOT NULL CHECK (status IN ('in-transit', 'received')),
    version     bigint      NOT NULL CHECK (version >= 1),
    created_by  uuid        NOT NULL,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL
);

-- Route facts recorded by the responsible party; no free status editing.
CREATE TABLE commerce.shipment_milestones (
    id             uuid        PRIMARY KEY,
    shipment_id    uuid        NOT NULL REFERENCES commerce.shipments,
    milestone_type text        NOT NULL,
    occurred_at    timestamptz NOT NULL,
    location       text        NOT NULL,
    note           text        NOT NULL DEFAULT '',
    recorded_by    uuid        NOT NULL,
    company_id     uuid        NOT NULL,
    recorded_at    timestamptz NOT NULL
);
CREATE INDEX shipment_milestones_idx ON commerce.shipment_milestones (shipment_id, occurred_at);
