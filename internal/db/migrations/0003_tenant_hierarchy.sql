-- 0003: tenants become hierarchical; tenant groups are removed.
--
-- A tenant may have a parent tenant, making every level of the hierarchy
-- a real, assignable owner — a device can belong to the top-level org
-- while a VM belongs to a child org. The separate group tree this
-- replaces had non-assignable interior nodes (the NetBox quirk).
-- Pre-release: existing tenant_groups rows are dropped, not converted.
--
-- Tenant slugs stay globally unique: tenants are referenced from many
-- object types, so their refs remain short and stable; the hierarchy is
-- organizational, not an addressing scope.

ALTER TABLE tenants
    ADD COLUMN parent_id uuid REFERENCES tenants (id);

CREATE INDEX tenants_parent_idx ON tenants (parent_id);

ALTER TABLE tenants
    DROP COLUMN tenant_group_id;

DROP TABLE tenant_groups;
