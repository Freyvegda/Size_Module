-- Rules profiles: named, reusable constraint + objective presets. They are the
-- only place vertical specifics live, so every column here is data, not code.

-- name: ListRulesProfiles :many
SELECT id, plant_id, code, name, rules, objective, is_default, created_at, updated_at
FROM rules_profiles
ORDER BY is_default DESC, code;

-- name: GetRulesProfile :one
SELECT id, plant_id, code, name, rules, objective, is_default, created_at, updated_at
FROM rules_profiles
WHERE id = $1;

-- name: CreateRulesProfile :one
INSERT INTO rules_profiles (plant_id, code, name, rules, objective, is_default)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, plant_id, code, name, rules, objective, is_default, created_at, updated_at;

-- name: UpdateRulesProfile :one
UPDATE rules_profiles
SET name       = $2,
    rules      = $3,
    objective  = $4,
    is_default = $5,
    updated_at = now()
WHERE id = $1
RETURNING id, plant_id, code, name, rules, objective, is_default, created_at, updated_at;

-- name: ClearDefaultRulesProfiles :exec
UPDATE rules_profiles
SET is_default = false, updated_at = now()
WHERE plant_id = $1 AND id <> $2 AND is_default;
