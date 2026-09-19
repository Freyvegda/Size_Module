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

-- name: ListMaterialSpecOptions :many
SELECT
    ms.id,
    ms.material_id,
    ms.code,
    ms.name,
    ms.thickness_um,
    ms.finish,
    ms.color,
    m.code             AS material_code,
    m.name             AS material_name,
    m.dimension_profile
FROM material_specs ms
JOIN materials m ON m.id = ms.material_id
WHERE m.is_active
ORDER BY m.code, ms.code;

-- name: CreateMaterialSpec :one
INSERT INTO material_specs (material_id, code, name, thickness_um, finish, color, attributes)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;
