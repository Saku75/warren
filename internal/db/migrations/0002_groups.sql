-- 0002: tenant groups and site groups.
--
-- Both are pure nestable trees with parent-scoped slugs (path-addressed,
-- design doc §4.3). Site groups absorb NetBox's separate Region and
-- SiteGroup models: one tree, distinguished by a kind label (§5.3).

CREATE TABLE tenant_groups (
    id          uuid PRIMARY KEY,
    parent_id   uuid REFERENCES tenant_groups (id),
    slug        text        NOT NULL,
    name        text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (parent_id, slug)
);

CREATE INDEX tenant_groups_parent_idx ON tenant_groups (parent_id);

ALTER TABLE tenants
    ADD COLUMN tenant_group_id uuid REFERENCES tenant_groups (id);

CREATE INDEX tenants_group_idx ON tenants (tenant_group_id);

CREATE TABLE site_groups (
    id          uuid PRIMARY KEY,
    parent_id   uuid REFERENCES site_groups (id),
    slug        text        NOT NULL,
    name        text        NOT NULL,
    kind        text        NOT NULL DEFAULT 'group'
                CHECK (kind IN ('region', 'group')),
    description text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (parent_id, slug)
);

CREATE INDEX site_groups_parent_idx ON site_groups (parent_id);

ALTER TABLE sites
    ADD COLUMN site_group_id uuid REFERENCES site_groups (id);

CREATE INDEX sites_group_idx ON sites (site_group_id);
