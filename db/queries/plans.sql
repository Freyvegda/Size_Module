-- name: CreatePlan :one
INSERT INTO plans (plant_id, job_id, parent_plan_id, version, status, name, solver, solver_version, seed, rules, metrics, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetPlan :one
SELECT *
FROM plans
WHERE id = $1;

-- name: GetPlanForUpdate :one
SELECT *
FROM plans
WHERE id = $1
FOR UPDATE;

-- name: ListPlans :many
SELECT *
FROM plans
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: AcceptPlan :one
-- Freezes a plan: only draft or approved plans can be accepted, and the guard
-- makes a second accept a no-op the caller can detect (no rows).
UPDATE plans
SET status = 'accepted', accepted_at = now(), updated_at = now()
WHERE id = $1 AND status IN ('draft', 'approved')
RETURNING *;

-- name: CreatePlanSheet :one
INSERT INTO plan_sheets (plan_id, sheet_index, stock_code, stock_id, stock_item_id, label, width_um, height_um, offcuts, cut_steps)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: ArchivePlan :execrows
-- Retires a plan version that has been superseded by an edit or re-solve.
UPDATE plans
SET status = 'archived', updated_at = now()
WHERE id = $1 AND status IN ('draft', 'approved');

-- name: ListPlanSheets :many
SELECT *
FROM plan_sheets
WHERE plan_id = $1
ORDER BY sheet_index;

-- name: CreatePlacement :one
INSERT INTO placements (plan_sheet_id, part_id, part_code, x_um, y_um, w_um, h_um, rotated, locked, seq)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: ListPlacements :many
SELECT *
FROM placements
WHERE plan_sheet_id = $1
ORDER BY seq;

-- name: InsertAuditLog :exec
INSERT INTO audit_log (plant_id, actor, action, entity, entity_id, payload)
VALUES ($1, $2, $3, $4, $5, $6);
