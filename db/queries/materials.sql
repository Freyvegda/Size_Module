-- name: ListMaterials :many
SELECT *
FROM materials
WHERE is_active
ORDER BY code;

-- name: GetMaterial :one
SELECT *
FROM materials
WHERE id = $1;

-- name: CreateMaterial :one
INSERT INTO materials (plant_id, code, name, dimension_profile, attributes)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListMaterialSpecs :many
SELECT *
FROM material_specs
ORDER BY code;

-- name: CreateMaterialSpec :one
INSERT INTO material_specs (material_id, code, name, thickness_um, finish, color, attributes)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;
