-- name: CreateManufacturer :one
INSERT INTO manufacturers (id, slug, name, description)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetManufacturer :one
SELECT * FROM manufacturers WHERE id = $1;

-- name: GetManufacturerBySlug :one
SELECT * FROM manufacturers WHERE slug = $1;

-- name: ListManufacturers :many
SELECT * FROM manufacturers ORDER BY name, id LIMIT $1 OFFSET $2;

-- name: CountManufacturers :one
SELECT count(*) FROM manufacturers;

-- name: UpdateManufacturer :one
UPDATE manufacturers
SET slug = $2, name = $3, description = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteManufacturer :execrows
DELETE FROM manufacturers WHERE id = $1;

-- name: CreateDeviceRole :one
INSERT INTO device_roles (id, slug, name, color, description)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetDeviceRole :one
SELECT * FROM device_roles WHERE id = $1;

-- name: GetDeviceRoleBySlug :one
SELECT * FROM device_roles WHERE slug = $1;

-- name: ListDeviceRoles :many
SELECT * FROM device_roles ORDER BY name, id LIMIT $1 OFFSET $2;

-- name: CountDeviceRoles :one
SELECT count(*) FROM device_roles;

-- name: UpdateDeviceRole :one
UPDATE device_roles
SET slug = $2, name = $3, color = $4, description = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteDeviceRole :execrows
DELETE FROM device_roles WHERE id = $1;

-- name: CreateDeviceType :one
INSERT INTO device_types (id, manufacturer_id, slug, model, part_number, u_height, is_full_depth, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetDeviceType :one
SELECT * FROM device_types WHERE id = $1;

-- name: GetDeviceTypeByScope :one
SELECT * FROM device_types WHERE manufacturer_id = $1 AND slug = $2;

-- name: ListDeviceTypes :many
SELECT dt.*, m.slug AS manufacturer_slug, m.name AS manufacturer_name
FROM device_types dt
JOIN manufacturers m ON m.id = dt.manufacturer_id
ORDER BY m.name, dt.model, dt.id
LIMIT $1 OFFSET $2;

-- name: CountDeviceTypes :one
SELECT count(*) FROM device_types;

-- name: ListDeviceTypesByManufacturer :many
SELECT * FROM device_types WHERE manufacturer_id = $1 ORDER BY model, id;

-- name: UpdateDeviceType :one
UPDATE device_types
SET slug = $2, model = $3, part_number = $4, u_height = $5,
    is_full_depth = $6, description = $7, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteDeviceType :execrows
DELETE FROM device_types WHERE id = $1;

-- name: CreateModuleType :one
INSERT INTO module_types (id, manufacturer_id, slug, model, part_number, description)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetModuleType :one
SELECT * FROM module_types WHERE id = $1;

-- name: GetModuleTypeByScope :one
SELECT * FROM module_types WHERE manufacturer_id = $1 AND slug = $2;

-- name: ListModuleTypes :many
SELECT mt.*, m.slug AS manufacturer_slug, m.name AS manufacturer_name
FROM module_types mt
JOIN manufacturers m ON m.id = mt.manufacturer_id
ORDER BY m.name, mt.model, mt.id
LIMIT $1 OFFSET $2;

-- name: CountModuleTypes :one
SELECT count(*) FROM module_types;

-- name: ListModuleTypesByManufacturer :many
SELECT * FROM module_types WHERE manufacturer_id = $1 ORDER BY model, id;

-- name: UpdateModuleType :one
UPDATE module_types
SET slug = $2, model = $3, part_number = $4, description = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteModuleType :execrows
DELETE FROM module_types WHERE id = $1;

-- name: CreateComponentTemplate :one
INSERT INTO component_templates (id, device_type_id, module_type_id, kind, name, label, attrs, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetComponentTemplate :one
SELECT * FROM component_templates WHERE id = $1;

-- name: ListComponentTemplatesByDeviceType :many
SELECT * FROM component_templates WHERE device_type_id = $1 ORDER BY kind, name, id;

-- name: ListComponentTemplatesByModuleType :many
SELECT * FROM component_templates WHERE module_type_id = $1 ORDER BY kind, name, id;

-- name: UpdateComponentTemplate :one
UPDATE component_templates
SET name = $2, label = $3, attrs = $4, description = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteComponentTemplate :execrows
DELETE FROM component_templates WHERE id = $1;
