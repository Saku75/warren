-- name: CreateTenantGroup :one
INSERT INTO tenant_groups (id, parent_id, slug, name, description)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetTenantGroup :one
SELECT * FROM tenant_groups WHERE id = $1;

-- name: GetTenantGroupByScope :one
SELECT * FROM tenant_groups
WHERE parent_id IS NOT DISTINCT FROM $1 AND slug = $2;

-- name: ListTenantGroups :many
SELECT * FROM tenant_groups ORDER BY name, id;

-- name: ListTenantGroupChildren :many
SELECT * FROM tenant_groups WHERE parent_id = $1 ORDER BY name, id;

-- name: GetTenantGroupPath :one
WITH RECURSIVE chain AS (
    SELECT g.id, g.parent_id, g.slug, 0 AS depth
    FROM tenant_groups g
    WHERE g.id = $1
    UNION ALL
    SELECT p.id, p.parent_id, p.slug, c.depth + 1
    FROM tenant_groups p
    JOIN chain c ON p.id = c.parent_id
)
SELECT string_agg(c.slug, '/' ORDER BY c.depth DESC)::text AS path
FROM chain c;

-- name: UpdateTenantGroup :one
UPDATE tenant_groups
SET parent_id = $2, slug = $3, name = $4, description = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteTenantGroup :execrows
DELETE FROM tenant_groups WHERE id = $1;

-- name: ListTenantsByGroup :many
SELECT * FROM tenants WHERE tenant_group_id = $1 ORDER BY name, id;
