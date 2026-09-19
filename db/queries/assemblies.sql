-- name: ListAssemblies :many
SELECT *
FROM assemblies
ORDER BY code;

-- name: GetAssembly :one
SELECT *
FROM assemblies
WHERE id = $1;

-- name: ListAssemblyComponents :many
SELECT *
FROM assembly_components
WHERE assembly_id = $1
ORDER BY seq;

-- name: ListAllAssemblyComponents :many
SELECT *
FROM assembly_components
ORDER BY assembly_id, seq;

-- name: CreateAssembly :one
INSERT INTO assemblies (plant_id, material_spec_id, code, name, kind, width_um, height_um, depth_um, attributes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: CreateAssemblyComponent :one
INSERT INTO assembly_components (
    assembly_id, seq, role, kind, name, material_spec_id, quantity,
    width_um, height_um, depth_um,
    offset_x_um, offset_y_um, offset_z_um, attributes
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;
