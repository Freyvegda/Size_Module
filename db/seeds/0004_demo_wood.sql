-- Demo wood family with two types and sheet stock, so a product (e.g. a table)
-- can be designed against real material. Safe to run repeatedly
-- (ON CONFLICT DO NOTHING).
BEGIN;

INSERT INTO materials (plant_id, code, name, dimension_profile)
SELECT p.id, 'WOOD', 'Wood', '2d' FROM plants p WHERE p.code = 'DEMO'
ON CONFLICT (plant_id, code) DO NOTHING;

INSERT INTO material_specs (material_id, code, name, thickness_um, finish, color)
SELECT m.id, 'OAK-18', 'Oak 18 mm', 18000, 'sanded', 'natural'
FROM materials m WHERE m.code = 'WOOD'
ON CONFLICT (material_id, code) DO NOTHING;

INSERT INTO material_specs (material_id, code, name, thickness_um, finish, color)
SELECT m.id, 'PINE-25', 'Pine 25 mm', 25000, 'sanded', 'natural'
FROM materials m WHERE m.code = 'WOOD'
ON CONFLICT (material_id, code) DO NOTHING;

INSERT INTO stock_formats (plant_id, material_spec_id, code, width_um, height_um, on_hand_qty, cost_per_unit)
SELECT m.plant_id, ms.id, 'SHEET-OAK-2440x1220', 2440000, 1220000, 12, 48.00
FROM material_specs ms
JOIN materials m ON m.id = ms.material_id
WHERE ms.code = 'OAK-18'
ON CONFLICT (plant_id, code) DO NOTHING;

INSERT INTO stock_formats (plant_id, material_spec_id, code, width_um, height_um, on_hand_qty, cost_per_unit)
SELECT m.plant_id, ms.id, 'SHEET-PINE-2440x1220', 2440000, 1220000, 20, 32.00
FROM material_specs ms
JOIN materials m ON m.id = ms.material_id
WHERE ms.code = 'PINE-25'
ON CONFLICT (plant_id, code) DO NOTHING;

COMMIT;
