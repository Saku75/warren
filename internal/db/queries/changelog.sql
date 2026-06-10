-- name: InsertChangelogEntry :exec
INSERT INTO changelog (id, actor, action, object_type, object_id, object_label, data_before, data_after)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListChangelog :many
SELECT * FROM changelog ORDER BY id DESC LIMIT $1 OFFSET $2;

-- name: CountChangelog :one
SELECT count(*) FROM changelog;

-- name: ListChangelogForObject :many
SELECT * FROM changelog
WHERE object_type = $1 AND object_id = $2
ORDER BY id DESC
LIMIT $3 OFFSET $4;
