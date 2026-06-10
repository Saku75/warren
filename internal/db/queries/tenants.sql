-- name: CreateTenant :one
INSERT INTO tenants (id, slug, name, description, tenant_group_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetTenant :one
SELECT * FROM tenants WHERE id = $1;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = $1;

-- name: ListTenants :many
-- Tenants with their group's display fields for lists.
SELECT t.*, g.slug AS group_slug, g.name AS group_name
FROM tenants t
LEFT JOIN tenant_groups g ON g.id = t.tenant_group_id
ORDER BY t.name, t.id
LIMIT $1 OFFSET $2;

-- name: CountTenants :one
SELECT count(*) FROM tenants;

-- name: UpdateTenant :one
UPDATE tenants
SET slug = $2, name = $3, description = $4, tenant_group_id = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteTenant :execrows
DELETE FROM tenants WHERE id = $1;
