-- Aggregates for the realized-yield dashboard. All metrics are read from the
-- JSONB scorecard stored on each plan, so the numbers are exactly what the
-- solver reported when the plan was made (or edited).

-- name: PlanKPIs :one
SELECT
    count(*)                                                                                        AS total_plans,
    count(*) FILTER (WHERE status = 'accepted')                                                     AS accepted_plans,
    coalesce(sum((metrics->>'sheetCount')::numeric) FILTER (WHERE status = 'accepted'), 0)::bigint   AS accepted_sheets,
    coalesce(sum((metrics->>'partsPlaced')::numeric) FILTER (WHERE status = 'accepted'), 0)::bigint  AS accepted_parts,
    coalesce(sum((metrics->>'remnantSheets')::numeric) FILTER (WHERE status = 'accepted'), 0)::bigint AS accepted_remnant_sheets,
    coalesce(sum((metrics->>'stockAreaM2')::numeric) FILTER (WHERE status = 'accepted'), 0)::float8 AS accepted_stock_area_m2,
    coalesce(sum((metrics->>'partAreaM2')::numeric) FILTER (WHERE status = 'accepted'), 0)::float8  AS accepted_part_area_m2,
    coalesce(sum((metrics->>'scrapAreaM2')::numeric) FILTER (WHERE status = 'accepted'), 0)::float8 AS accepted_scrap_area_m2,
    coalesce(sum((metrics->>'offcutAreaM2')::numeric) FILTER (WHERE status = 'accepted'), 0)::float8 AS accepted_offcut_area_m2,
    coalesce(sum((metrics->>'cost')::numeric) FILTER (WHERE status = 'accepted'), 0)::float8        AS accepted_cost,
    coalesce(sum((metrics->>'sheetCount')::numeric), 0)::bigint                                     AS total_sheets,
    coalesce(sum((metrics->>'partsPlaced')::numeric), 0)::bigint                                    AS total_parts_placed,
    coalesce(sum((metrics->>'remnantSheets')::numeric), 0)::bigint                                  AS total_remnant_sheets,
    coalesce(sum((metrics->>'stockAreaM2')::numeric), 0)::float8                                    AS total_stock_area_m2,
    coalesce(sum((metrics->>'partAreaM2')::numeric), 0)::float8                                     AS total_part_area_m2,
    coalesce(sum((metrics->>'scrapAreaM2')::numeric), 0)::float8                                    AS total_scrap_area_m2,
    coalesce(sum((metrics->>'offcutAreaM2')::numeric), 0)::float8                                   AS total_offcut_area_m2,
    coalesce(sum((metrics->>'cost')::numeric), 0)::float8                                           AS total_cost
FROM plans
WHERE created_at >= sqlc.arg('from') AND created_at < sqlc.arg('to');

-- name: PlanYieldSeries :many
SELECT
    id,
    created_at,
    (metrics->>'yieldPct')::float8 AS yield_pct,
    (metrics->>'wastePct')::float8 AS waste_pct,
    (metrics->>'cost')::float8     AS cost,
    (metrics->>'sheetCount')::numeric::int AS sheets,
    (metrics->>'partsPlaced')::numeric::int AS parts_placed
FROM plans
WHERE status = 'accepted'
  AND created_at >= sqlc.arg('from') AND created_at < sqlc.arg('to')
ORDER BY created_at
LIMIT sqlc.arg('limit');
