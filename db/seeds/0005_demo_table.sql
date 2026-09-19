-- Demo table product built from oak sheets: top, four legs, aprons and a shelf.
-- Run after 0004_demo_wood.sql (it references OAK-18). Safe to run repeatedly.
BEGIN;

INSERT INTO assemblies (plant_id, code, name, kind, material_spec_id, width_um, height_um, depth_um)
SELECT m.plant_id, 'TABLE-OAK-1200x750', 'Oak dining table 1200 × 750', 'generic', ms.id,
       1200000, 750000, 600000
FROM material_specs ms
JOIN materials m ON m.id = ms.material_id
WHERE ms.code = 'OAK-18'
ON CONFLICT (plant_id, code) DO NOTHING;

-- Origin bottom-left-front (x right, y up, z out): 1200 wide, 750 high, 600 deep.
-- Top 18, legs 60², aprons 80 high, inset 30, shelf at 40% leg height.
WITH specs AS (
    SELECT id FROM material_specs WHERE code = 'OAK-18'
)
INSERT INTO assembly_components (
    assembly_id, seq, role, kind, name, material_spec_id, quantity,
    width_um, height_um, depth_um, offset_x_um, offset_y_um, offset_z_um
)
SELECT a.id, v.seq, v.role, v.kind, v.name, specs.id, v.qty, v.w, v.h, v.d, v.ox, v.oy, v.oz
FROM assemblies a
CROSS JOIN specs
JOIN (VALUES
    (1,  'top',         'panel', 'Table top',   1, 1200000,  18000, 600000,       0, 732000,      0),
    (2,  'leg',         'beam',  'Leg',         1,   60000, 732000,  60000,   30000,      0,  30000),
    (3,  'leg',         'beam',  'Leg',         1,   60000, 732000,  60000, 1110000,      0,  30000),
    (4,  'leg',         'beam',  'Leg',         1,   60000, 732000,  60000,   30000,      0, 510000),
    (5,  'leg',         'beam',  'Leg',         1,   60000, 732000,  60000, 1110000,      0, 510000),
    (6,  'apron-long',  'beam',  'Long apron',  1, 1140000,  80000,  18000,   30000, 652000,  30000),
    (7,  'apron-long',  'beam',  'Long apron',  1, 1140000,  80000,  18000,   30000, 652000, 552000),
    (8,  'apron-short', 'beam',  'Short apron', 1,   18000,  80000, 540000,   30000, 652000,  30000),
    (9,  'apron-short', 'beam',  'Short apron', 1,   18000,  80000, 540000, 1152000, 652000,  30000),
    (10, 'shelf',       'panel', 'Shelf',       1, 1140000,  18000, 540000,   30000, 292800,  30000)
) AS v(seq, role, kind, name, qty, w, h, d, ox, oy, oz) ON true
WHERE a.code = 'TABLE-OAK-1200x750'
  AND NOT EXISTS (SELECT 1 FROM assembly_components c WHERE c.assembly_id = a.id);

COMMIT;
