-- Set when an administrator assigns a password; the user must choose their
-- own before doing anything else.
ALTER TABLE identity.users ADD COLUMN password_change_required boolean NOT NULL DEFAULT false;
