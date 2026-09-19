-- name: CreateJob :one
INSERT INTO cut_jobs (plant_id, status, solver, input, seed, budget_ms, created_by)
VALUES ($1, 'queued', $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetJob :one
SELECT *
FROM cut_jobs
WHERE id = $1;

-- name: ListJobs :many
SELECT id, plant_id, status, solver, seed, budget_ms, error, created_at, started_at, finished_at
FROM cut_jobs
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: MarkJobRunning :exec
UPDATE cut_jobs
SET status = 'running', started_at = now()
WHERE id = $1;

-- name: MarkJobDone :exec
UPDATE cut_jobs
SET status = 'done', result = $2, finished_at = now()
WHERE id = $1;

-- name: MarkJobFailed :exec
UPDATE cut_jobs
SET status = 'failed', error = $2, finished_at = now()
WHERE id = $1;

-- name: CancelJob :execrows
UPDATE cut_jobs
SET status = 'cancelled', finished_at = now()
WHERE id = $1 AND status IN ('queued', 'running');

-- name: ClaimNextJob :one
-- Claims the oldest queued job. FOR UPDATE SKIP LOCKED makes several workers
-- safe against each other: they never block and never claim the same row.
UPDATE cut_jobs
SET status = 'running', started_at = now()
WHERE id = (
    SELECT id
    FROM cut_jobs
    WHERE status = 'queued'
    ORDER BY created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING *;

-- name: FailStaleJobs :execrows
-- Called on worker start: jobs left in 'running' by a crash cannot be resumed,
-- so they are failed with an explanation instead of blocking the queue forever.
UPDATE cut_jobs
SET status = 'failed',
    error = 'worker stopped while this job was running',
    finished_at = now()
WHERE status = 'running'
  AND started_at < now() - make_interval(secs => sqlc.arg('stale_seconds')::int);

-- name: GetJobView :one
SELECT
    j.id,
    j.status,
    j.solver,
    j.error,
    j.created_at,
    j.started_at,
    j.finished_at,
    p.id      AS plan_id,
    p.metrics AS plan_metrics,
    j.result  AS result
FROM cut_jobs j
LEFT JOIN plans p ON p.job_id = j.id
WHERE j.id = $1;
