-- Two-factor authentication is removed from the product (user decision 2026-09-24).
DROP TABLE IF EXISTS identity.mfa_challenges;
DROP TABLE IF EXISTS identity.mfa_recovery_codes;
DROP TABLE IF EXISTS identity.mfa_enrollments;
ALTER TABLE identity.sessions DROP COLUMN IF EXISTS mfa_authenticated_at;
ALTER TABLE identity.users
    DROP COLUMN IF EXISTS mfa_secret_enc,
    DROP COLUMN IF EXISTS mfa_enabled_at,
    DROP COLUMN IF EXISTS mfa_last_counter;
