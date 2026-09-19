# db/ — PostgreSQL schema, queries and tooling

Self-contained database workspace: goose migrations, sqlc source queries, demo
seeds, SQL smoke tests and PowerShell scripts. sqlc generates Go code straight
into `backend/internal/platform/db` (see `sqlc.yaml`).

## Layout

| Path | Purpose |
|---|---|
| `migrations/` | `0001_init.sql` and `0002_remnants.sql` (goose Up/Down) — 15 tables |
| `queries/*.sql` | ~40 named sqlc queries (`jobs`, `materials`, `parts`, `plans`, `plant`, `stock_formats`, `stock_items`) |
| `seeds/` | `0001_demo_data.sql` (plant, glass/alu materials, stock, parts, routing, rules profile, machine) + `0002_demo_remnants.sql` (local pool of labelled leftovers) |
| `tests/smoke.sql` | table tests (`\set ON_ERROR_STOP on`, DO blocks, rolled-back writes) |
| `scripts/*.ps1` | day-to-day commands (below) |
| `sqlc.yaml` | sqlc v2 config: schema = migrations, output = `../backend/internal/platform/db`, pgx/v5, UUID → google/uuid, numeric → float64 |

Tables: `plants`, `users`, `materials`, `material_specs`, `machines`,
`rules_profiles`, `stock_formats`, `parts`, `part_routings`, `cut_jobs`,
`plans`, `plan_sheets`, `placements`, `audit_log`, `stock_items`.

The remnant lifecycle adds `stock_items` (physical pieces with label, status,
lineage and prorated cost) plus `plan_sheets.stock_id` / `stock_item_id` and
`plans.accepted_at`.

## Working rules

- **Units**: all length/dimension columns are `BIGINT` micrometers. JSONB is
  used for flexible attributes/rules/metrics; keep the indexed core fields.
- **Changing generated code**: edit `queries/*.sql` (each query has a
  `-- name:` comment), then run `scripts/generate.ps1`. Never edit
  `backend/internal/platform/db/*` by hand.
- **Schema change**: `scripts/new-migration.ps1 -Name add_x`, edit the pair,
  `scripts/migrate.ps1`, then regenerate queries if affected.
- **Seeds** must stay idempotent (`ON CONFLICT DO NOTHING`, wrapped in
  `BEGIN/COMMIT`) so `seed.ps1` can run repeatedly.
- Ports come from `db/.env` (`DATABASE_URL`, default host port **5433**) and must
  match `deploy/compose/.env` (`POSTGRES_PORT`).

## Scripts (PowerShell, run from anywhere)

| Script | Purpose |
|---|---|
| `up.ps1` / `down.ps1 [-Volumes]` | start / stop PostgreSQL + Adminer via docker compose |
| `migrate.ps1 [-Status] [-Down] [-To N]` | apply/inspect goose migrations |
| `new-migration.ps1 -Name add_remnants` | create the next up/down pair |
| `seed.ps1` | apply everything in `seeds/` in name order |
| `psql.ps1` / `psql.ps1 -File some.sql` | interactive psql / run a SQL file |
| `test.ps1` | run the table tests in `tests/` |
| `generate.ps1` | `sqlc generate` into the backend |
| `reset.ps1 [-Force]` | drop schema, migrate, seed |

`common.ps1` is the shared helper (compose file discovery, DB URL, dirs) — it is
dot-sourced, not meant to be run directly.
