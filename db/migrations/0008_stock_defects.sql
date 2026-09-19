-- 0008_stock_defects.sql — unusable regions inside a physical piece.
--
-- A remnant with a knot, a crack or a scratch is not useless: the good area
-- around the defect can still be cut. Storing the defect rectangles lets the
-- solvers avoid them (they already accept core.StockItem.Defects) and lets the
-- validator prove no piece was placed over one.
--
-- Rectangles use the same coordinate system and micrometer units as placements:
-- origin at the piece's top-left corner, x right, y down.

-- +goose Up

ALTER TABLE stock_items
    ADD COLUMN defects JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down

ALTER TABLE stock_items DROP COLUMN defects;
