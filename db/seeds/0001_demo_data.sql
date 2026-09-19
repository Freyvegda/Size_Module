-- Demo data for local development. Safe to run repeatedly (ON CONFLICT DO NOTHING).
BEGIN;

INSERT INTO plants (code, name, timezone)
VALUES ('DEMO', 'Demo Plant', 'UTC')
ON CONFLICT (code) DO NOTHING;

-- Materials ---------------------------------------------------------------
INSERT INTO materials (plant_id, code, name, dimension_profile)
SELECT p.id, 'GLASS', 'Glass', '2d' FROM plants p WHERE p.code = 'DEMO'
ON CONFLICT (plant_id, code) DO NOTHING;

INSERT INTO materials (plant_id, code, name, dimension_profile)
SELECT p.id, 'ALU', 'Aluminium profiles', '1d' FROM plants p WHERE p.code = 'DEMO'
ON CONFLICT (plant_id, code) DO NOTHING;

-- Specs -------------------------------------------------------------------
INSERT INTO material_specs (material_id, code, name, thickness_um, finish, color)
SELECT m.id, 'GLASS-CLEAR-6', 'Clear float 6 mm', 6000, 'clear', 'clear'
FROM materials m WHERE m.code = 'GLASS'
ON CONFLICT (material_id, code) DO NOTHING;

INSERT INTO material_specs (material_id, code, name, thickness_um, finish, color)
SELECT m.id, 'ALU-6060-T6', 'Aluminium 6060 T6', 0, 'mill', 'silver'
FROM materials m WHERE m.code = 'ALU'
ON CONFLICT (material_id, code) DO NOTHING;

-- Stock formats -----------------------------------------------------------
INSERT INTO stock_formats (plant_id, material_spec_id, code, width_um, height_um, on_hand_qty, cost_per_unit)
SELECT m.plant_id, ms.id, 'SHEET-3210x2250', 3210000, 2250000, 40, 62.50
FROM material_specs ms
JOIN materials m ON m.id = ms.material_id
WHERE ms.code = 'GLASS-CLEAR-6'
ON CONFLICT (plant_id, code) DO NOTHING;

INSERT INTO stock_formats (plant_id, material_spec_id, code, width_um, height_um, on_hand_qty, cost_per_unit)
SELECT m.plant_id, ms.id, 'SHEET-2440x1220', 2440000, 1220000, 25, 28.00
FROM material_specs ms
JOIN materials m ON m.id = ms.material_id
WHERE ms.code = 'GLASS-CLEAR-6'
ON CONFLICT (plant_id, code) DO NOTHING;

INSERT INTO stock_formats (plant_id, material_spec_id, code, length_um, width_um, on_hand_qty, cost_per_unit)
SELECT m.plant_id, ms.id, 'BAR-6000', 6000000, 60000, 30, 14.75
FROM material_specs ms
JOIN materials m ON m.id = ms.material_id
WHERE ms.code = 'ALU-6060-T6'
ON CONFLICT (plant_id, code) DO NOTHING;

-- Parts -------------------------------------------------------------------
INSERT INTO parts (plant_id, material_spec_id, code, name, finished_width_um, finished_height_um, grain, allow_rotate, priority)
SELECT m.plant_id, ms.id, 'WINDOW-1200x1400', 'Window pane 1200 x 1400', 1200000, 1400000, 'none', true, 1
FROM material_specs ms
JOIN materials m ON m.id = ms.material_id
WHERE ms.code = 'GLASS-CLEAR-6'
ON CONFLICT (plant_id, code) DO NOTHING;

INSERT INTO parts (plant_id, material_spec_id, code, name, finished_width_um, finished_height_um, grain, allow_rotate, priority)
SELECT m.plant_id, ms.id, 'WINDOW-800x1000', 'Window pane 800 x 1000', 800000, 1000000, 'none', true, 1
FROM material_specs ms
JOIN materials m ON m.id = ms.material_id
WHERE ms.code = 'GLASS-CLEAR-6'
ON CONFLICT (plant_id, code) DO NOTHING;

INSERT INTO parts (plant_id, material_spec_id, code, name, finished_width_um, finished_height_um, grain, allow_rotate, priority)
SELECT m.plant_id, ms.id, 'SHELF-500x250', 'Shelf 500 x 250', 500000, 250000, 'none', true, 2
FROM material_specs ms
JOIN materials m ON m.id = ms.material_id
WHERE ms.code = 'GLASS-CLEAR-6'
ON CONFLICT (plant_id, code) DO NOTHING;

-- Cut sizes: window panes are ground 2 mm per edge before tempering.
INSERT INTO part_routings (part_id, seq, operation, allowance_um, allowance_per_edge, notes)
SELECT pt.id, 1, 'edge grinding', 2000, true, '2 mm per edge'
FROM parts pt WHERE pt.code IN ('WINDOW-1200x1400', 'WINDOW-800x1000')
ON CONFLICT (part_id, seq) DO NOTHING;

-- Rules profile -----------------------------------------------------------
INSERT INTO rules_profiles (plant_id, code, name, rules, is_default)
SELECT p.id, 'GLASS-DEFAULT', 'Glass: guillotine, 4 mm kerf', '{
    "kerf": 4000,
    "trim": 10000,
    "allowRotate": true,
    "grainMode": "none",
    "cutMode": "guillotine",
    "offcutMinW": 300000,
    "offcutMinH": 300000,
    "minPartDim": 50000,
    "oversAllowedPct": 0
}'::jsonb, true
FROM plants p WHERE p.code = 'DEMO'
ON CONFLICT (plant_id, code) DO NOTHING;

-- Machines ----------------------------------------------------------------
INSERT INTO machines (plant_id, code, name, kind)
SELECT p.id, 'SAW-01', 'Panel saw 1', 'panel_saw' FROM plants p WHERE p.code = 'DEMO'
ON CONFLICT (plant_id, code) DO NOTHING;

COMMIT;
