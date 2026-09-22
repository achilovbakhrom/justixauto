DROP TABLE IF EXISTS platform.idempotency_keys;
DROP SCHEMA IF EXISTS platform;
DROP TABLE IF EXISTS identity.mfa_challenges;
DROP TABLE IF EXISTS identity.mfa_recovery_codes;
DROP TABLE IF EXISTS identity.mfa_enrollments;
ALTER TABLE identity.sessions DROP COLUMN mfa_authenticated_at;
ALTER TABLE identity.users DROP COLUMN mfa_secret_enc, DROP COLUMN mfa_enabled_at, DROP COLUMN mfa_last_counter;
