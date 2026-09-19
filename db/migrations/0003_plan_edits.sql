-- 0003_plan_edits.sql — interactive plan editing: per-placement locks and
-- version lineage for edited/re-solved plans.
--
-- Edits never mutate an existing plan in place: a save or re-solve creates the
-- next version row and archives the source. Locked placements are the pieces a
-- planner pinned; a re-solve must keep them exactly where they are.

-- +goose Up

ALTER TABLE placements ADD COLUMN locked BOOLEAN NOT NULL DEFAULT false;

-- The plan an edited/re-solved version was derived from.
ALTER TABLE plans ADD COLUMN parent_plan_id UUID REFERENCES plans(id) ON DELETE SET NULL;

CREATE INDEX plans_parent_idx ON plans (parent_plan_id);

-- +goose Down

DROP INDEX plans_parent_idx;
ALTER TABLE plans DROP COLUMN parent_plan_id;
ALTER TABLE placements DROP COLUMN locked;
