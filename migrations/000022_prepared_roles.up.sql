-- User decisions 2026-09-26: the platform admin prepares roles (named groups
-- of permissions) in Admin. A role is either a platform role (for JustixAuto
-- staff) or a company role, optionally limited to one company type; company
-- admins assign prepared company roles to their own employees. A company
-- employee belongs to exactly one company.

ALTER TABLE identity.roles
    ADD COLUMN scope text NOT NULL DEFAULT 'company' CHECK (scope IN ('platform', 'company')),
    -- NULL = any company type; only company roles may set it.
    ADD COLUMN company_kind text CHECK (company_kind IN ('seller', 'bank', 'mfo', 'insurance')),
    ADD CONSTRAINT roles_company_kind_scope_check CHECK (company_kind IS NULL OR scope = 'company');

UPDATE identity.roles SET scope = 'platform' WHERE system_key = 'platform_admin';

-- One user = one company: at most one active membership per user.
CREATE UNIQUE INDEX memberships_one_active_company_key
    ON identity.memberships (user_id) WHERE status = 'active';
