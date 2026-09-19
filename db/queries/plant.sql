-- name: GetDefaultPlant :one
SELECT *
FROM plants
ORDER BY created_at
LIMIT 1;

-- name: CreatePlant :one
INSERT INTO plants (code, name, timezone)
VALUES ($1, $2, $3)
RETURNING *;
