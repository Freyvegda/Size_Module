-- 0007_rules_objective.sql — rules profiles gain an objective preset.
--
-- A rules profile already stores the manufacturing constraints (kerf, trim,
-- grain, cut mode, offcut policy) that must live in data, not code. This adds
-- the matching objective preset (weights) so a glass shop and a wood shop can
-- also differ in what "best" means without anyone hand-writing weights.

-- +goose Up

ALTER TABLE rules_profiles
    ADD COLUMN objective JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down

ALTER TABLE rules_profiles DROP COLUMN objective;
