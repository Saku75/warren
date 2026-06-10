-- name: CreateTenant :one
INSERT INTO tenants (id, parent_id, slug, name, description)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetTenant :one
SELECT * FROM tenants WHERE id = $1;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = $1;

-- name: ListTenants :many
-- Tenants with their parent's display fields for lists.
SELECT t.*, p.slug AS parent_slug, p.name AS parent_name
FROM tenants t
LEFT JOIN tenants p ON p.id = t.parent_id
ORDER BY t.name, t.id
LIMIT $1 OFFSET $2;

-- name: ListTenantsTree :many
-- Every tenant, for tree assembly.
SELECT * FROM tenants ORDER BY name, id;

-- name: ListTenantChildren :many
SELECT * FROM tenants WHERE parent_id = $1 ORDER BY name, id;

-- name: CountTenants :one
SELECT count(*) FROM tenants;

-- name: UpdateTenant :one
UPDATE tenants
SET parent_id = $2, slug = $3, name = $4, description = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteTenant :execrows
DELETE FROM tenants WHERE id = $1;
