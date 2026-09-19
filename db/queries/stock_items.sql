-- Physical stock pieces: full sheets from a format and labelled remnants.
-- A stock_formats row is a catalog size; a stock_items row is one piece.

-- name: ListStockItems :many
SELECT
    si.*,
    sf.code           AS format_code,
    ms.code           AS spec_code,
    m.code            AS material_code,
    m.dimension_profile
FROM stock_items si
LEFT JOIN stock_formats sf ON sf.id = si.format_id
LEFT JOIN material_specs ms ON ms.id = sf.material_spec_id
LEFT JOIN materials m ON m.id = ms.material_id
WHERE si.plant_id = $1
  AND (sqlc.narg('status')::text IS NULL OR si.status = sqlc.narg('status')::text)
  AND (sqlc.narg('is_remnant')::boolean IS NULL OR si.is_remnant = sqlc.narg('is_remnant')::boolean)
ORDER BY si.is_remnant DESC, si.created_at DESC, si.id;

-- name: GetStockItem :one
SELECT
    si.*,
    sf.code           AS format_code,
    ms.code           AS spec_code,
    m.code            AS material_code,
    m.dimension_profile
FROM stock_items si
LEFT JOIN stock_formats sf ON sf.id = si.format_id
LEFT JOIN material_specs ms ON ms.id = sf.material_spec_id
LEFT JOIN materials m ON m.id = ms.material_id
WHERE si.id = $1;

-- name: CreateStockItem :one
INSERT INTO stock_items (
    plant_id, format_id, code, label, length_um, width_um, height_um,
    is_remnant, status, location, cost_per_unit, notes,
    parent_plan_id, parent_sheet_index
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: UpdateStockItem :one
UPDATE stock_items
SET label     = COALESCE(sqlc.narg('label')::text, label),
    location  = COALESCE(sqlc.narg('location')::text, location),
    status    = COALESCE(sqlc.narg('status')::text, status),
    notes     = COALESCE(sqlc.narg('notes')::text, notes),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ConsumeStockItem :execrows
-- Marks a physical piece used by a plan. Only pieces still in the pool can be
-- consumed, so a double accept cannot consume the same remnant twice.
UPDATE stock_items
SET status = 'consumed', consumed_by_plan_id = $2, consumed_at = now(), updated_at = now()
WHERE id = $1 AND status IN ('available', 'reserved');

-- name: ListAvailableRemnants :many
-- The remnant-first input for a solve: available, labelled leftovers for a
-- plant, optionally limited to one material spec.
SELECT si.*
FROM stock_items si
LEFT JOIN stock_formats sf ON sf.id = si.format_id
WHERE si.plant_id = $1
  AND si.status = 'available'
  AND si.is_remnant
  AND (sqlc.narg('material_spec_id')::uuid IS NULL
       OR sf.material_spec_id = sqlc.narg('material_spec_id')::uuid)
ORDER BY si.created_at, si.id;
