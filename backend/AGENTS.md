# backend/ — Go API + optimizer engine

Module: `github.com/size-module/backend` (Go 1.27). One process serves the REST
API **and** hosts the pure-Go optimizer. PostgreSQL is optional at startup:
without it the optimizer, solver list and demo endpoints still work, master-data
endpoints answer `503 database_unavailable`, and optimization runs are simply
not archived.

## Layout

| Path | Purpose |
|---|---|
| `cmd/cutoptics/main.go` | entry point: flags (`-demo`, `-demo-bar`, `-addr`), config, registry, optional pgx pool, chi router, graceful shutdown |
| `cmd/cutoptics/bench.go` | the `bench` subcommand: benchmark harness + golden regression gate (exit 1 on regression) |
| `api/openapi.yaml` | hand-written OpenAPI 3.1 contract — the source of truth for request/response schemas |
| `internal/modules/` | HTTP-facing feature modules: `catalog`, `parts`, `jobs`, `stock`, `plans`, `campaigns`, `kpis` (+ the products/`assemblies` module) |
| `internal/optimizer/` | pure Go solver library (no DB, no HTTP) |
| `internal/platform/` | config, HTTP server, http helpers, postgres pool/store, sqlc-generated queries, id |
| `testdata/benchmarks/` | committed benchmark instances (`glass-mixed`, `wood-panels`, `metal-bars`) + `golden.json` |

## Request path

`main.go` builds `httpserver.Deps{Config, Registry, Jobs, Catalog, Parts, DBHealth}`
and `httpserver.New` mounts, in this order:

- `GET /healthz`, `GET /api/v1/healthz`, `GET /api/v1/meta`
- jobs module (sync): `POST /api/v1/optimize`, `GET /api/v1/solvers`, `GET /api/v1/demo/plan`, `GET /api/v1/demo/bar-plan`
- jobs module (async queue): `POST /api/v1/jobs`, `GET /api/v1/jobs/{id}`, `POST /api/v1/jobs/{id}/cancel`, `GET /api/v1/jobs/{id}/events` (SSE)
- catalog module: `GET/POST /api/v1/materials`, `GET/POST /api/v1/material-specs`, `GET/POST /api/v1/stock-formats`
- parts module: `GET/POST /api/v1/parts`
- assemblies module: `GET/POST /api/v1/assemblies`, `GET /api/v1/assemblies/{id}` (products with subparts)
- stock module: `GET/POST /api/v1/stock-items`, `GET/PATCH /api/v1/stock-items/{id}`
- plans module: `GET /api/v1/plans`, `GET /api/v1/plans/{id}`, `POST /api/v1/plans/{id}/accept`, `POST /api/v1/plans/{id}/edit`, `POST /api/v1/plans/{id}/reoptimize`, `GET /api/v1/plans/{id}/exports?format=csv|svg|dxf|pdf`
- kpis module: `GET /api/v1/kpis?days=90` (or `from`/`to`) — realized/pipeline material and cost aggregates plus a trend series
- campaigns module: `GET/POST /api/v1/campaigns`, `GET/PATCH /api/v1/campaigns/{id}`, `POST /api/v1/campaigns/{id}/items`, `DELETE /api/v1/campaigns/{id}/items/{itemID}`, `POST /api/v1/campaigns/{id}/run-next`

Optimize flow (sync): `jobs.Service.Run` → `optimizer.Solve` (picks the solver from the
registry, or honours `?solver=`) → `validator.Validate` → optional archive via
the `jobs.Store` interface (implemented by `platform/postgres.Store.SaveRun` in
one transaction) → JSON response. `?dryRun=1` skips archiving (used by the
frontend solver comparison). `?includeRemnants=1` appends the plant's available
labelled remnants (optionally filtered by `materialSpecId`) before solving;
`preferRemnants` in the rules then orders them first and the objective does not
charge remnant sheets as new stock.

Async flow: `POST /api/v1/jobs` snapshots the problem into `cut_jobs`;
`jobs.Worker` (`CUTOPTICS_WORKERS`, default 2) claims it with
`FOR UPDATE SKIP LOCKED`, publishes progress to the `platform/events` hub,
streams it over SSE and archives the plan with `Store.Complete`. Stale running
jobs are failed on worker start.

## Conventions and gotchas

- **Units**: every length is `int64` micrometers in the API, DB and solver
  (`internal/optimizer/core/types.go`). Floats are only cost, percent, m².
- **No hand-written API types**: handlers decode into structs from
  `internal/modules/*` (input DTOs) and encode `core` structs directly. Keep
  `api/openapi.yaml` in sync when you touch an endpoint.
- **Generated code**: `internal/platform/db/*.sql.go`, `models.go`, `db.go` are
  sqlc output. Change `db/queries/*.sql` and run `db/scripts/generate.ps1`.
- **Store interfaces**: each module declares the small interface it needs
  (`catalog.Store`, `parts.Store`, `jobs.Store`). `platform/postgres.Store`
  implements them; a nil store is legal and means "DB down".
- **Errors**: use `platform/httpx` (`JSON`, `Error`, `DecodeJSON`) so error
  codes stay uniform.
- `main.go` keeps running when PostgreSQL is unreachable — `DBHealth` just
  reports it.

## Commands (run from `backend/`)

```powershell
go run ./cmd/cutoptics                 # API on :8080, no infrastructure required
go run ./cmd/cutoptics -demo           # print the 2D demo plan as JSON
go run ./cmd/cutoptics -demo-bar       # print the 1D demo plan as JSON
go test ./...                          # solver tests + golden benchmark gate
go test -short ./...                   # skips the golden gate
go build -ldflags "-X github.com/size-module/backend/internal/platform/config.Version=1.2.3"

go run ./cmd/cutoptics bench -dir testdata/benchmarks -random 6 -check
```

Environment: `CUTOPTICS_ADDR` (`:8080`), `DATABASE_URL` (default
`postgres://cutoptics:cutoptics@localhost:5433/cutoptics?sslmode=disable`),
`CUTOPTICS_ENV` (`dev`), `CUTOPTICS_CORS_ORIGINS`, `CUTOPTICS_WORKERS` (`2`,
asynchronous job workers in this process).

## Related

- `internal/optimizer/AGENTS.md` — solver engine details (read before touching solvers).
- `internal/modules/AGENTS.md` — module-by-module API notes.
- `internal/platform/AGENTS.md` — infrastructure packages.
- `../db/AGENTS.md` — schema/queries behind the generated db package.
