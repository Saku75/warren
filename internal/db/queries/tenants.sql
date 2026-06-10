-- name: CreateTenant :one
INSERT INTO tenants (id, slug, name, description)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetTenant :one
SELECT * FROM tenants WHERE id = $1;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = $1;

-- name: ListTenants :many
SELECT * FROM tenants ORDER BY name, id LIMIT $1 OFFSET $2;

-- name: CountTenants :one
SELECT count(*) FROM tenants;

-- name: UpdateTenant :one
UPDATE tenants
SET slug = $2, name = $3, description = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteTenant :execrows
DELETE FROM tenants WHERE id = $1;
