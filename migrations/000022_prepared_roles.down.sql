DROP INDEX identity.memberships_one_active_company_key;

ALTER TABLE identity.roles
    DROP CONSTRAINT roles_company_kind_scope_check,
    DROP COLUMN company_kind,
    DROP COLUMN scope;
