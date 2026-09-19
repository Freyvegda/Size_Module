-- 0002_remnants.sql — remnant lifecycle: physical stock pieces, offcut labels,
-- plan acceptance bookkeeping.
--
-- A stock_formats row is a *catalog size* with an on-hand quantity; a
-- stock_items row is one *physical piece* (a full sheet, a bar, or an offcut
-- with a label). Remnants are stock_items with is_remnant = true and a label
-- that can be scanned on the shop floor.
--
-- Unit rule: every length is micrometers (µm) stored as BIGINT.

-- +goose Up

CREATE TABLE stock_items (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id            UUID NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    -- The catalog size this piece was cut from; NULL for manually added pieces.
    format_id           UUID REFERENCES stock_formats(id) ON DELETE SET NULL,
    -- Format code copy (or a generated code for manual pieces).
    code                TEXT NOT NULL DEFAULT '',
    label               TEXT NOT NULL,
    length_um           BIGINT NOT NULL DEFAULT 0 CHECK (length_um >= 0),
    width_um            BIGINT NOT NULL DEFAULT 0 CHECK (width_um >= 0),
    height_um           BIGINT NOT NULL DEFAULT 0 CHECK (height_um >= 0),
    is_remnant          BOOLEAN NOT NULL DEFAULT false,
    status              TEXT NOT NULL DEFAULT 'available'
                        CHECK (status IN ('available', 'reserved', 'consumed', 'retired')),
    location            TEXT NOT NULL DEFAULT '',
    -- Cost basis of this piece, so the objective can value remnants correctly.
    cost_per_unit       NUMERIC(14, 4) NOT NULL DEFAULT 0 CHECK (cost_per_unit >= 0),
    notes               TEXT NOT NULL DEFAULT '',
    -- Where the piece came from: the plan and sheet that produced it.
    parent_plan_id      UUID REFERENCES plans(id) ON DELETE SET NULL,
    parent_sheet_index  INTEGER,
    -- Where it went: the plan that consumed it.
    consumed_by_plan_id UUID REFERENCES plans(id) ON DELETE SET NULL,
    consumed_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- 1d pieces carry a length; 2d/3d pieces carry width and height.
    CHECK (length_um > 0 OR (width_um > 0 AND height_um > 0))
);

-- Labels are the shop-floor identifier; enforce uniqueness only when one is
-- set (manual pieces may be unlabelled while being registered).
CREATE UNIQUE INDEX stock_items_label_key ON stock_items (plant_id, label) WHERE label <> '';

CREATE INDEX stock_items_pool_idx ON stock_items (plant_id, is_remnant, status, created_at DESC);

-- Link a plan sheet back to the physical piece it was cut from. format-based
-- sheets keep stock_id (the problem's stock reference) but have no
-- stock_item_id; remnant sheets carry both.
ALTER TABLE plan_sheets ADD COLUMN stock_id TEXT NOT NULL DEFAULT '';
ALTER TABLE plan_sheets ADD COLUMN stock_item_id UUID REFERENCES stock_items(id) ON DELETE SET NULL;

ALTER TABLE plans ADD COLUMN accepted_at TIMESTAMPTZ;

-- +goose Down

ALTER TABLE plans DROP COLUMN accepted_at;
ALTER TABLE plan_sheets DROP COLUMN stock_item_id;
ALTER TABLE plan_sheets DROP COLUMN stock_id;
DROP TABLE IF EXISTS stock_items;
