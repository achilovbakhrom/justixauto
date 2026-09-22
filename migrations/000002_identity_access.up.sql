-- Companies: full CompanyInput from the HTTP contract; "insurer" kind renamed
-- to the contract's "insurance".
ALTER TABLE identity.companies DROP CONSTRAINT companies_kind_check;
UPDATE identity.companies SET kind = 'insurance' WHERE kind = 'insurer';
ALTER TABLE identity.companies
    ADD CONSTRAINT companies_kind_check CHECK (kind IN ('seller', 'bank', 'mfo', 'insurance')),
    ADD COLUMN legal_name  text NOT NULL DEFAULT '' CHECK (length(legal_name) <= 300),
    ADD COLUMN country_key text NOT NULL DEFAULT '' CHECK (length(country_key) <= 50),
    ADD COLUMN region_key  text NOT NULL DEFAULT '' CHECK (length(region_key) <= 50),
    ADD COLUMN email       text NOT NULL DEFAULT '' CHECK (length(email) <= 254),
    ADD COLUMN address     text NOT NULL DEFAULT '' CHECK (length(address) <= 500),
    ADD COLUMN phone       text NOT NULL DEFAULT '' CHECK (length(phone) <= 50);

-- Duplicates: normalized country (key when chosen from the list) + registration.
DROP INDEX identity.companies_country_registration_key;
CREATE UNIQUE INDEX companies_country_registration_key ON identity.companies (
    (CASE WHEN country_key <> '' THEN country_key ELSE lower(country) END),
    lower(registration_number));

CREATE TABLE identity.users (
    id            uuid        PRIMARY KEY,
    display_name  text        NOT NULL CHECK (length(display_name) BETWEEN 1 AND 200),
    email         text        NOT NULL CHECK (length(email) BETWEEN 3 AND 254),
    login         text        CHECK (length(login) BETWEEN 3 AND 100),
    password_hash text,
    status        text        NOT NULL CHECK (status IN ('pending', 'active', 'suspended')),
    status_reason text        NOT NULL DEFAULT '',
    failed_logins integer     NOT NULL DEFAULT 0,
    locked_until  timestamptz,
    version       bigint      NOT NULL CHECK (version >= 1),
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    CHECK (status <> 'active' OR (login IS NOT NULL AND password_hash IS NOT NULL))
);
CREATE UNIQUE INDEX users_email_key ON identity.users (lower(email));
CREATE UNIQUE INDEX users_login_key ON identity.users (lower(login)) WHERE login IS NOT NULL;

-- Roles are global (the same in every company the user is a member of).
-- System roles get their permissions from code; custom roles from role_permissions.
CREATE TABLE identity.roles (
    id         uuid        PRIMARY KEY,
    system_key text        UNIQUE,
    name       text        NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    version    bigint      NOT NULL CHECK (version >= 1),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX roles_name_key ON identity.roles (lower(name));

CREATE TABLE identity.role_permissions (
    role_id    uuid NOT NULL REFERENCES identity.roles ON DELETE CASCADE,
    permission text NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE identity.user_roles (
    user_id uuid NOT NULL REFERENCES identity.users ON DELETE CASCADE,
    role_id uuid NOT NULL REFERENCES identity.roles,
    PRIMARY KEY (user_id, role_id)
);

INSERT INTO identity.roles (id, system_key, name, version, created_at, updated_at) VALUES
    ('00000000-0000-4000-8000-000000000001', 'platform_admin', 'Platform administrator', 1, now(), now()),
    ('00000000-0000-4000-8000-000000000002', 'company_admin', 'Company administrator', 1, now(), now());

CREATE TABLE identity.branches (
    id         uuid        PRIMARY KEY,
    company_id uuid        NOT NULL REFERENCES identity.companies,
    name       text        NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    address    text        NOT NULL DEFAULT '' CHECK (length(address) <= 500),
    version    bigint      NOT NULL CHECK (version >= 1),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX branches_company_name_key ON identity.branches (company_id, lower(name));

CREATE TABLE identity.memberships (
    id            uuid        PRIMARY KEY,
    user_id       uuid        NOT NULL REFERENCES identity.users,
    company_id    uuid        NOT NULL REFERENCES identity.companies,
    status        text        NOT NULL CHECK (status IN ('active', 'revoked')),
    branch_access text        NOT NULL CHECK (branch_access IN ('ALL_BRANCHES', 'SELECTED_BRANCHES')),
    status_reason text        NOT NULL DEFAULT '',
    version       bigint      NOT NULL CHECK (version >= 1),
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL
);
-- One active membership per user and company; revoked ones stay as history.
CREATE UNIQUE INDEX memberships_active_key ON identity.memberships (user_id, company_id) WHERE status = 'active';

CREATE TABLE identity.membership_branches (
    membership_id uuid NOT NULL REFERENCES identity.memberships ON DELETE CASCADE,
    branch_id     uuid NOT NULL REFERENCES identity.branches,
    PRIMARY KEY (membership_id, branch_id)
);

CREATE TABLE identity.sessions (
    id                uuid        PRIMARY KEY,
    token_hash        bytea       NOT NULL UNIQUE,
    csrf_token        text        NOT NULL,
    user_id           uuid        NOT NULL REFERENCES identity.users ON DELETE CASCADE,
    active_company_id uuid        REFERENCES identity.companies,
    branch_scope_mode text        NOT NULL DEFAULT 'ALL' CHECK (branch_scope_mode IN ('ALL', 'SELECTED')),
    context_revision  bigint      NOT NULL DEFAULT 1,
    created_at        timestamptz NOT NULL,
    last_seen_at      timestamptz NOT NULL,
    expires_at        timestamptz NOT NULL,
    revoked_at        timestamptz
);
CREATE INDEX sessions_user_live_idx ON identity.sessions (user_id) WHERE revoked_at IS NULL;

CREATE TABLE identity.session_branches (
    session_id uuid NOT NULL REFERENCES identity.sessions ON DELETE CASCADE,
    branch_id  uuid NOT NULL REFERENCES identity.branches,
    PRIMARY KEY (session_id, branch_id)
);

-- Append-only record of sensitive changes: who, what, when, why, before/after.
CREATE TABLE identity.audit_events (
    id            uuid        PRIMARY KEY,
    seq           bigint      GENERATED ALWAYS AS IDENTITY UNIQUE, -- insertion order
    occurred_at   timestamptz NOT NULL,
    actor_user_id uuid,
    action        text        NOT NULL,
    resource_type text        NOT NULL,
    resource_id   uuid        NOT NULL,
    company_id    uuid,
    reason        text        NOT NULL DEFAULT '',
    details       jsonb       NOT NULL DEFAULT '{}'
);
CREATE INDEX audit_events_resource_idx ON identity.audit_events (resource_type, resource_id, seq DESC);
CREATE INDEX audit_events_actor_idx ON identity.audit_events (actor_user_id, seq DESC);
