-- name: CreateSiteGroup :one
INSERT INTO site_groups (id, parent_id, slug, name, kind, description)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetSiteGroup :one
SELECT * FROM site_groups WHERE id = $1;

-- name: GetSiteGroupByScope :one
SELECT * FROM site_groups
WHERE parent_id IS NOT DISTINCT FROM $1 AND slug = $2;

-- name: ListSiteGroups :many
SELECT * FROM site_groups ORDER BY name, id;

-- name: ListSiteGroupChildren :many
SELECT * FROM site_groups WHERE parent_id = $1 ORDER BY name, id;

-- name: GetSiteGroupPath :one
WITH RECURSIVE chain AS (
    SELECT g.id, g.parent_id, g.slug, 0 AS depth
    FROM site_groups g
    WHERE g.id = $1
    UNION ALL
    SELECT p.id, p.parent_id, p.slug, c.depth + 1
    FROM site_groups p
    JOIN chain c ON p.id = c.parent_id
)
SELECT string_agg(c.slug, '/' ORDER BY c.depth DESC)::text AS path
FROM chain c;

-- name: UpdateSiteGroup :one
UPDATE site_groups
SET parent_id = $2, slug = $3, name = $4, kind = $5, description = $6, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteSiteGroup :execrows
DELETE FROM site_groups WHERE id = $1;

-- name: ListSitesByGroup :many
SELECT * FROM sites WHERE site_group_id = $1 ORDER BY name, id;
