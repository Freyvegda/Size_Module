-- name: ListStockFormats :many
SELECT
    sf.id,
    sf.plant_id,
    sf.material_spec_id,
    sf.code,
    sf.length_um,
    sf.width_um,
    sf.height_um,
    sf.on_hand_qty,
    sf.cost_per_unit,
    sf.is_active,
    ms.code        AS spec_code,
    ms.thickness_um,
    ms.finish,
    m.code         AS material_code,
    m.name         AS material_name,
    m.dimension_profile
FROM stock_formats sf
JOIN material_specs ms ON ms.id = sf.material_spec_id
JOIN materials m ON m.id = ms.material_id
WHERE sf.is_active
ORDER BY m.code, ms.code, sf.code;

-- name: GetStockFormat :one
SELECT *
FROM stock_formats
WHERE id = $1;

-- name: GetStockFormatDetail :one
SELECT
    sf.id,
    sf.plant_id,
    sf.material_spec_id,
    sf.code,
    sf.length_um,
    sf.width_um,
    sf.height_um,
    sf.on_hand_qty,
    sf.cost_per_unit,
    sf.is_active,
    ms.code        AS spec_code,
    ms.thickness_um,
    ms.finish,
    m.code         AS material_code,
    m.name         AS material_name,
    m.dimension_profile
FROM stock_formats sf
JOIN material_specs ms ON ms.id = sf.material_spec_id
JOIN materials m ON m.id = ms.material_id
WHERE sf.id = $1;

-- name: CreateStockFormat :one
INSERT INTO stock_formats (plant_id, material_spec_id, code, length_um, width_um, height_um, on_hand_qty, cost_per_unit)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateStockOnHand :exec
UPDATE stock_formats
SET on_hand_qty = on_hand_qty + sqlc.arg('delta'), updated_at = now()
WHERE id = sqlc.arg('id');

-- name: TakeStockOnHand :exec
-- Consumes one physical sheet from a format without ever going below zero.
UPDATE stock_formats
SET on_hand_qty = GREATEST(on_hand_qty - 1, 0), updated_at = now()
WHERE id = $1;
