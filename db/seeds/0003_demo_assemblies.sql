-- Demo assembly: a window defined as an overall size plus subparts (four frame
-- beams and a glass panel). Safe to run repeatedly: the assembly is inserted
-- with ON CONFLICT DO NOTHING and components only when none exist yet.
BEGIN;

INSERT INTO assemblies (plant_id, code, name, kind, width_um, height_um, depth_um)
SELECT p.id, 'WIN-1200x1400', 'Demo window 1200 × 1400', 'window', 1200000, 1400000, 70000
FROM plants p WHERE p.code = 'DEMO'
ON CONFLICT (plant_id, code) DO NOTHING;

-- Window geometry (all micrometers): overall 1200 × 1400, frame section 70,
-- frame depth 70, glass 24 thick inset 10 inside the frame.
-- Origin is the bottom-left-front corner: x right, y up, z out of the wall.
WITH specs AS (
    SELECT
        (SELECT id FROM material_specs WHERE code = 'GLASS-CLEAR-6') AS glass,
        (SELECT id FROM material_specs WHERE code = 'ALU-6060-T6') AS alu
)
INSERT INTO assembly_components (
    assembly_id, seq, role, kind, name, material_spec_id,
    width_um, height_um, depth_um, offset_x_um, offset_y_um, offset_z_um
)
SELECT
    a.id, v.seq, v.role, v.kind, v.name,
    CASE v.kind WHEN 'beam' THEN specs.alu ELSE specs.glass END,
    v.w, v.h, v.d, v.ox, v.oy, v.oz
FROM assemblies a
CROSS JOIN specs
JOIN (VALUES
    (1, 'frame-bottom', 'beam',  'Bottom frame', 1200000,  70000, 70000,       0,       0, 0),
    (2, 'frame-top',    'beam',  'Top frame',    1200000,  70000, 70000,       0, 1330000, 0),
    (3, 'frame-left',   'beam',  'Left frame',     70000, 1260000, 70000,       0,   70000, 0),
    (4, 'frame-right',  'beam',  'Right frame',    70000, 1260000, 70000, 1130000,   70000, 0),
    (5, 'glass',        'panel', 'Glass pane',   1040000, 1240000, 24000,   80000,   80000, 23000)
) AS v(seq, role, kind, name, w, h, d, ox, oy, oz) ON true
WHERE a.code = 'WIN-1200x1400'
  AND NOT EXISTS (SELECT 1 FROM assembly_components c WHERE c.assembly_id = a.id);

COMMIT;
