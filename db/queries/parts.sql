-- name: ListParts :many
SELECT *
FROM parts
ORDER BY code;

-- name: GetPart :one
SELECT *
FROM parts
WHERE id = $1;

-- name: CreatePart :one
INSERT INTO parts (
    plant_id, material_spec_id, code, name,
    finished_length_um, finished_width_um, finished_height_um,
    grain, allow_rotate, priority, attributes
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: ListPartRoutings :many
SELECT *
FROM part_routings
WHERE part_id = $1
ORDER BY seq;

-- name: CreatePartRouting :one
INSERT INTO part_routings (part_id, seq, operation, allowance_um, allowance_per_edge, notes)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;
