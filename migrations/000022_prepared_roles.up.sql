-- User decisions 2026-09-26: the platform admin prepares roles (a name and a
-- set of permissions) in Admin. A role's scope follows from its permissions:
-- company roles are assigned by company admins to their employees, platform
-- roles to JustixAuto staff. A company employee belongs to exactly one company.

ALTER TABLE identity.roles
    ADD COLUMN scope text NOT NULL DEFAULT 'company' CHECK (scope IN ('platform', 'company'));

UPDATE identity.roles SET scope = 'platform' WHERE system_key = 'platform_admin';

-- One user = one company: at most one active membership per user.
CREATE UNIQUE INDEX memberships_one_active_company_key
    ON identity.memberships (user_id) WHERE status = 'active';
