-- Demo remnants: leftovers from earlier plans, registered as physical pieces
-- with shop-floor labels. Safe to run repeatedly (ON CONFLICT DO NOTHING).
BEGIN;

INSERT INTO stock_items (plant_id, format_id, code, label, width_um, height_um, is_remnant, status, location, cost_per_unit, notes)
SELECT sf.plant_id, sf.id, sf.code, 'OFF-DEMO-01', 1600000, 1000000, true, 'available', 'Rack A', 12.50,
       'demo glass remnant, 1600 x 1000 mm'
FROM stock_formats sf
WHERE sf.code = 'SHEET-2440x1220'
ON CONFLICT DO NOTHING;

INSERT INTO stock_items (plant_id, format_id, code, label, width_um, height_um, is_remnant, status, location, cost_per_unit, notes)
SELECT sf.plant_id, sf.id, sf.code, 'OFF-DEMO-02', 900000, 400000, true, 'available', 'Rack A', 3.39,
       'demo glass remnant, 900 x 400 mm'
FROM stock_formats sf
WHERE sf.code = 'SHEET-2440x1220'
ON CONFLICT DO NOTHING;

INSERT INTO stock_items (plant_id, format_id, code, label, length_um, width_um, is_remnant, status, location, cost_per_unit, notes)
SELECT sf.plant_id, sf.id, sf.code, 'OFF-DEMO-BAR', 2200000, 60000, true, 'available', 'Rack B', 5.41,
       'demo aluminium bar remnant, 2200 mm'
FROM stock_formats sf
WHERE sf.code = 'BAR-6000'
ON CONFLICT DO NOTHING;

COMMIT;
