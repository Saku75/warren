-- name: CreateRack :one
INSERT INTO racks (id, site_id, location_id, slug, name, status, u_height, tenant_id, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetRack :one
SELECT * FROM racks WHERE id = $1;

-- name: GetRackByScope :one
SELECT * FROM racks WHERE site_id = $1 AND slug = $2;

-- name: ListRacks :many
SELECT r.*, s.slug AS site_slug, s.name AS site_name
FROM racks r JOIN sites s ON s.id = r.site_id
ORDER BY s.name, r.name, r.id LIMIT $1 OFFSET $2;

-- name: CountRacks :one
SELECT count(*) FROM racks;

-- name: UpdateRack :one
UPDATE racks
SET slug = $2, name = $3, status = $4, u_height = $5, location_id = $6,
    tenant_id = $7, description = $8, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteRack :execrows
DELETE FROM racks WHERE id = $1;

-- name: CreateDevice :one
INSERT INTO devices (id, site_id, location_id, rack_id, position, face, device_type_id, role_id, tenant_id, slug, name, status, serial, asset_tag, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: GetDevice :one
SELECT * FROM devices WHERE id = $1;

-- name: GetDeviceByScope :one
SELECT * FROM devices WHERE site_id = $1 AND slug = $2;

-- name: ListDevices :many
SELECT d.*, s.slug AS site_slug, s.name AS site_name,
       dt.model AS type_model, r.name AS role_name, r.color AS role_color
FROM devices d
JOIN sites s ON s.id = d.site_id
JOIN device_types dt ON dt.id = d.device_type_id
JOIN device_roles r ON r.id = d.role_id
ORDER BY s.name, d.name, d.id LIMIT $1 OFFSET $2;

-- name: CountDevices :one
SELECT count(*) FROM devices;

-- name: ListDevicesByRack :many
SELECT d.*, dt.model AS type_model, dt.u_height AS type_u_height
FROM devices d JOIN device_types dt ON dt.id = d.device_type_id
WHERE d.rack_id = $1 ORDER BY d.position DESC NULLS LAST, d.name;

-- name: UpdateDevice :one
UPDATE devices
SET slug = $2, name = $3, status = $4, location_id = $5, rack_id = $6,
    position = $7, face = $8, role_id = $9, tenant_id = $10, serial = $11,
    asset_tag = $12, description = $13, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteDevice :execrows
DELETE FROM devices WHERE id = $1;

-- name: CreateComponent :one
INSERT INTO components (id, device_id, module_id, kind, name, label, attrs, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetComponent :one
SELECT * FROM components WHERE id = $1;

-- name: ListComponentsByDevice :many
SELECT * FROM components WHERE device_id = $1 ORDER BY kind, name, id;

-- name: DeleteComponent :execrows
DELETE FROM components WHERE id = $1;

-- name: CreateModule :one
INSERT INTO modules (id, device_id, bay_id, module_type_id, serial, description)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetModule :one
SELECT * FROM modules WHERE id = $1;

-- name: GetModuleByBay :one
SELECT * FROM modules WHERE bay_id = $1;

-- name: ListModulesByDevice :many
SELECT m.*, mt.model AS type_model, b.name AS bay_name
FROM modules m
JOIN module_types mt ON mt.id = m.module_type_id
JOIN components b ON b.id = m.bay_id
WHERE m.device_id = $1 ORDER BY b.name, m.id;

-- name: DeleteModule :execrows
DELETE FROM modules WHERE id = $1;
