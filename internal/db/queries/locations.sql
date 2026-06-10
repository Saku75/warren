-- name: CreateLocation :one
INSERT INTO locations (id, site_id, parent_id, slug, name, kind, status, tenant_id, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetLocation :one
SELECT * FROM locations WHERE id = $1;

-- name: GetLocationByScope :one
-- Resolves one slug-path segment: the location with this slug directly
-- under the given parent (NULL parent = site root).
SELECT * FROM locations
WHERE site_id = $1 AND parent_id IS NOT DISTINCT FROM $2 AND slug = $3;

-- name: ListLocationsBySite :many
SELECT * FROM locations WHERE site_id = $1 ORDER BY name, id;

-- name: CountLocationsBySite :one
SELECT count(*) FROM locations WHERE site_id = $1;

-- name: CountLocations :one
SELECT count(*) FROM locations;

-- name: ListLocationChildren :many
SELECT * FROM locations WHERE parent_id = $1 ORDER BY name, id;

-- name: GetLocationPath :one
-- The slug path from the site root to this location, e.g.
-- "cph-dc1/building-a/floor-2".
WITH RECURSIVE chain AS (
    SELECT l.id, l.parent_id, l.slug, l.site_id, 0 AS depth
    FROM locations l
    WHERE l.id = $1
    UNION ALL
    SELECT p.id, p.parent_id, p.slug, p.site_id, c.depth + 1
    FROM locations p
    JOIN chain c ON p.id = c.parent_id
)
SELECT (s.slug || '/' || string_agg(c.slug, '/' ORDER BY c.depth DESC))::text AS path
FROM chain c
JOIN sites s ON s.id = c.site_id
GROUP BY s.slug;

-- name: UpdateLocation :one
UPDATE locations
SET parent_id = $2, slug = $3, name = $4, kind = $5, status = $6,
    tenant_id = $7, description = $8, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteLocation :execrows
DELETE FROM locations WHERE id = $1;
