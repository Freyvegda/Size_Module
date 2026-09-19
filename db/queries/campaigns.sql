-- Campaign planning: ordered jobs sharing one stock budget.

-- name: CreateCampaign :one
INSERT INTO campaigns (plant_id, code, name, rules, objective, budget_ms, seed, stock, initial_stock)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetCampaign :one
SELECT *
FROM campaigns
WHERE id = $1;

-- name: GetCampaignForUpdate :one
SELECT *
FROM campaigns
WHERE id = $1
FOR UPDATE;

-- name: ListCampaigns :many
SELECT *
FROM campaigns
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateCampaignMeta :one
UPDATE campaigns
SET name = COALESCE(sqlc.narg('name')::text, name),
    status = COALESCE(sqlc.narg('status')::text, status),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateCampaignStock :one
-- Stores the remaining budget after an item ran and moves the campaign to
-- active (or completed when no pending items remain).
UPDATE campaigns
SET stock = $2,
    status = $3,
    completed_at = CASE WHEN $3 = 'completed' THEN now() ELSE completed_at END,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CreateCampaignItem :one
INSERT INTO campaign_items (campaign_id, seq, name, parts, due_date, priority)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListCampaignItems :many
SELECT *
FROM campaign_items
WHERE campaign_id = $1
ORDER BY seq;

-- name: GetCampaignItem :one
SELECT *
FROM campaign_items
WHERE id = $1;

-- name: NextCampaignItem :one
SELECT *
FROM campaign_items
WHERE campaign_id = $1 AND status = 'pending'
ORDER BY seq
LIMIT 1;

-- name: CountPendingCampaignItems :one
SELECT count(*)::bigint AS pending
FROM campaign_items
WHERE campaign_id = $1 AND status = 'pending';

-- name: CompleteCampaignItem :one
UPDATE campaign_items
SET status = $2, plan_id = $3, job_id = $4, error = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteCampaignItem :execrows
DELETE FROM campaign_items
WHERE id = $1 AND campaign_id = $2 AND status = 'pending';
