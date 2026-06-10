-- name: CreateSite :one
INSERT INTO sites (id, slug, name, status, tenant_id, site_group_id, facility, time_zone, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetSite :one
SELECT * FROM sites WHERE id = $1;

-- name: GetSiteBySlug :one
SELECT * FROM sites WHERE slug = $1;

-- name: ListSites :many
-- Sites with tenant and group display fields for lists.
SELECT s.*,
       t.slug AS tenant_slug, t.name AS tenant_name,
       g.slug AS group_slug, g.name AS group_name
FROM sites s
LEFT JOIN tenants t ON t.id = s.tenant_id
LEFT JOIN site_groups g ON g.id = s.site_group_id
ORDER BY s.name, s.id
LIMIT $1 OFFSET $2;

-- name: CountSites :one
SELECT count(*) FROM sites;

-- name: UpdateSite :one
UPDATE sites
SET slug = $2, name = $3, status = $4, tenant_id = $5, site_group_id = $6,
    facility = $7, time_zone = $8, description = $9, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteSite :execrows
DELETE FROM sites WHERE id = $1;
