-- name: CreateSite :one
INSERT INTO sites (id, slug, name, status, tenant_id, facility, time_zone, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetSite :one
SELECT * FROM sites WHERE id = $1;

-- name: GetSiteBySlug :one
SELECT * FROM sites WHERE slug = $1;

-- name: ListSites :many
-- Sites with their tenant's display fields for lists and detail pages.
SELECT s.*, t.slug AS tenant_slug, t.name AS tenant_name
FROM sites s
LEFT JOIN tenants t ON t.id = s.tenant_id
ORDER BY s.name, s.id
LIMIT $1 OFFSET $2;

-- name: CountSites :one
SELECT count(*) FROM sites;

-- name: UpdateSite :one
UPDATE sites
SET slug = $2, name = $3, status = $4, tenant_id = $5, facility = $6,
    time_zone = $7, description = $8, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteSite :execrows
DELETE FROM sites WHERE id = $1;
