-- Restores the empty 000003 MFA schema; enrolled authenticators are not recoverable.
ALTER TABLE identity.users
    ADD COLUMN mfa_secret_enc   bytea,
    ADD COLUMN mfa_enabled_at   timestamptz,
    ADD COLUMN mfa_last_counter bigint NOT NULL DEFAULT 0;

ALTER TABLE identity.sessions ADD COLUMN mfa_authenticated_at timestamptz;

CREATE TABLE identity.mfa_enrollments (
    id           uuid        PRIMARY KEY,
    user_id      uuid        NOT NULL REFERENCES identity.users ON DELETE CASCADE,
    secret_enc   bytea       NOT NULL,
    created_at   timestamptz NOT NULL,
    expires_at   timestamptz NOT NULL,
    confirmed_at timestamptz
);

CREATE TABLE identity.mfa_recovery_codes (
    user_id   uuid        NOT NULL REFERENCES identity.users ON DELETE CASCADE,
    code_hash bytea       NOT NULL,
    used_at   timestamptz,
    PRIMARY KEY (user_id, code_hash)
);

CREATE TABLE identity.mfa_challenges (
    id          uuid        PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES identity.users ON DELETE CASCADE,
    token_hash  bytea       NOT NULL,
    attempts    integer     NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL,
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz
);
