-- Inventory owns vehicle models, physical vehicles (VIN), warehouses, receipt
-- batches and placements. Company IDs refer to identity.companies by ID only.
CREATE SCHEMA IF NOT EXISTS inventory;

CREATE TABLE inventory.vehicle_models (
    id                   uuid        PRIMARY KEY,
    make                 text        NOT NULL CHECK (length(make) BETWEEN 1 AND 100),
    model                text        NOT NULL CHECK (length(model) BETWEEN 1 AND 100),
    variant              text        NOT NULL CHECK (length(variant) BETWEEN 1 AND 100),
    current_spec_version integer     NOT NULL CHECK (current_spec_version >= 1),
    version              bigint      NOT NULL CHECK (version >= 1),
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL
);
CREATE UNIQUE INDEX vehicle_models_identity_key ON inventory.vehicle_models (lower(make), lower(model), lower(variant));

-- Immutable specification versions; vehicles point to the exact version.
CREATE TABLE inventory.model_specifications (
    model_id       uuid        NOT NULL REFERENCES inventory.vehicle_models,
    spec_version   integer     NOT NULL CHECK (spec_version >= 1),
    year           integer     NOT NULL CHECK (year BETWEEN 1900 AND 2100),
    body_type      text        NOT NULL CHECK (length(body_type) BETWEEN 1 AND 50),
    exterior_color text        NOT NULL CHECK (length(exterior_color) BETWEEN 1 AND 50),
    interior_color text        NOT NULL CHECK (length(interior_color) BETWEEN 1 AND 50),
    powertrain     text        NOT NULL CHECK (length(powertrain) BETWEEN 1 AND 50),
    drivetrain     text        NOT NULL CHECK (length(drivetrain) BETWEEN 1 AND 50),
    created_at     timestamptz NOT NULL,
    created_by     uuid        NOT NULL,
    PRIMARY KEY (model_id, spec_version)
);

CREATE TABLE inventory.warehouses (
    id          uuid        PRIMARY KEY,
    company_id  uuid        NOT NULL,
    branch_id   uuid,
    name        text        NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    country     text        NOT NULL CHECK (length(country) BETWEEN 1 AND 100),
    country_key text        NOT NULL DEFAULT '',
    region      text        NOT NULL DEFAULT '',
    region_key  text        NOT NULL DEFAULT '',
    city        text        NOT NULL CHECK (length(city) BETWEEN 1 AND 100),
    address     text        NOT NULL CHECK (length(address) BETWEEN 1 AND 500),
    capacity    integer     NOT NULL CHECK (capacity > 0),
    version     bigint      NOT NULL CHECK (version >= 1),
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL
);
CREATE UNIQUE INDEX warehouses_company_name_key ON inventory.warehouses (company_id, lower(name));
-- At most one primary warehouse per branch.
CREATE UNIQUE INDEX warehouses_branch_key ON inventory.warehouses (company_id, branch_id) WHERE branch_id IS NOT NULL;

-- A receipt of N homogeneous vehicles; VINs may be entered later.
CREATE TABLE inventory.receipt_batches (
    id                 uuid        PRIMARY KEY,
    company_id         uuid        NOT NULL,
    warehouse_id       uuid        NOT NULL REFERENCES inventory.warehouses,
    model_id           uuid        NOT NULL,
    spec_version       integer     NOT NULL,
    confirmed_quantity integer     NOT NULL CHECK (confirmed_quantity > 0),
    identified_count   integer     NOT NULL CHECK (identified_count >= 0),
    unidentified_count integer     NOT NULL CHECK (unidentified_count >= 0),
    received_at        timestamptz NOT NULL,
    created_by         uuid        NOT NULL,
    version            bigint      NOT NULL CHECK (version >= 1),
    created_at         timestamptz NOT NULL,
    FOREIGN KEY (model_id, spec_version) REFERENCES inventory.model_specifications,
    CHECK (identified_count + unidentified_count = confirmed_quantity)
);
CREATE INDEX receipt_batches_warehouse_idx ON inventory.receipt_batches (warehouse_id) WHERE unidentified_count > 0;

-- One physical vehicle per globally unique VIN.
CREATE TABLE inventory.vehicle_units (
    id                   uuid        PRIMARY KEY,
    vin                  text        NOT NULL UNIQUE CHECK (vin ~ '^[A-HJ-NPR-Z0-9]{17}$'),
    model_id             uuid        NOT NULL,
    spec_version         integer     NOT NULL,
    owner_company_id     uuid,
    custodian_company_id uuid,
    version              bigint      NOT NULL CHECK (version >= 1),
    created_at           timestamptz NOT NULL,
    FOREIGN KEY (model_id, spec_version) REFERENCES inventory.model_specifications
);

-- Where a vehicle physically is: at most one current placement.
CREATE TABLE inventory.placements (
    vehicle_id       uuid        PRIMARY KEY REFERENCES inventory.vehicle_units,
    warehouse_id     uuid        NOT NULL REFERENCES inventory.warehouses,
    receipt_batch_id uuid        REFERENCES inventory.receipt_batches,
    placed_at        timestamptz NOT NULL
);
CREATE INDEX placements_warehouse_idx ON inventory.placements (warehouse_id);

-- Append-only history of what happened to vehicles and stock.
CREATE TABLE inventory.facts (
    id               uuid        PRIMARY KEY,
    seq              bigint      GENERATED ALWAYS AS IDENTITY UNIQUE,
    company_id       uuid        NOT NULL,
    fact_type        text        NOT NULL,
    vehicle_id       uuid,
    warehouse_id     uuid,
    receipt_batch_id uuid,
    actor_user_id    uuid        NOT NULL,
    occurred_at      timestamptz NOT NULL,
    recorded_at      timestamptz NOT NULL,
    reason           text        NOT NULL DEFAULT '',
    details          jsonb       NOT NULL DEFAULT '{}'
);
CREATE INDEX facts_vehicle_idx ON inventory.facts (vehicle_id, seq) WHERE vehicle_id IS NOT NULL;
CREATE INDEX facts_warehouse_idx ON inventory.facts (warehouse_id, seq) WHERE warehouse_id IS NOT NULL;
