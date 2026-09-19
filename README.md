# Size Module

A material optimization platform: it decides how to divide stock material into
products with the least waste, and it keeps the leftovers in circulation.

The first vertical slice is a **cut optimizer**: give it an order (parts) and
stock (sheets, panels, bars) and it returns a validated cutting plan with 2D and
3D visualisations, a waste breakdown and shop-floor cut steps.

```
frontend/   React 19 + TypeScript + Tailwind/shadcn + Redux Toolkit + three.js
backend/    Go: HTTP API + optimizer engine (pure Go, unit tested)
db/         PostgreSQL schema, sqlc queries, seeds, table tests, scripts
deploy/     docker-compose for local infrastructure
docs/       Domain glossary and notes
```

## What works today

| Piece | Status |
|---|---|
| 2D beam search solver (`beam-2d`) | explores guillotine cut trees; 26.8% mean waste vs 35.0% for the shelf baseline on the benchmark set |
| Local search solver (`polish-2d`) | dissolves, merges and repacks sheets within the time budget; 24.5% mean waste, 9→7 and 7→6 sheets on two hard instances |
| 2D shelf solver (`shelf-2d`) | fast baseline: strips, kerf/trim/grain/rotation, reusable offcuts, cut sequences |
| Column generation solver (`cg-1d`) | Gilmore–Gomory with DP pricing and an LP lower bound; 26 bars vs 28 for the baseline on `bars-varied` (optimal), 41/50 vs 36/50 pieces when stock is short |
| Two-stage column generation (`cg-2d`) | strip + height pricing for guillotine sheets; 26.3% mean waste vs 29.2% for the beam, at ~2 ms per instance, and it minimises stock **cost** |
| Free-cutting solver (`maxrects-2d`) | MaxRects for `cutMode: free` (CNC, laser, waterjet); ties on sheet count but leaves more fragmented remnants — measured, not assumed |
| 1D First-Fit-Decreasing solver (`ffd-1d`) | fast baseline for bars, profiles and tubes with offcuts |
| Portfolios (`best-1d`, `best-2d`) | run every strategy for the dimension and cut mode within the budget, keep the best objective score |
| Guillotine cut-tree validator | proves a layout can actually be cut; rejects pinwheel layouts |
| Plan validator | overlaps, bounds, kerf, grain, demand limits, guillotine feasibility |
| Explainer | waste breakdown (trim/kerf/scrap/offcut), area lower bound, notes |
| Benchmark harness | committed + generated instances, golden regression gate, `go test` enforcement |
| REST API | health, meta, solvers, optimize (`solver=`, `dryRun=`), demo plans, materials, stock formats, parts |
| Async job queue | `POST /api/v1/jobs` → Postgres queue (`FOR UPDATE SKIP LOCKED`), worker pool, `GET /jobs/{id}`, cancel, **SSE progress** with best-so-far plans |
| Physical stock pool | `stock_items` distinguishes catalog sizes from pieces; `/api/v1/stock-items` registers labelled remnants, retires them, and keeps lineage to the plan that produced them |
| Remnant-first allocation | `?includeRemnants=1` loads available labelled remnants into the problem; constructive solvers use them before fresh sheets and the objective does not charge remnant sheets as new stock |
| Plan acceptance | `POST /api/v1/plans/{id}/accept` consumes pieces, decrements on-hand quantities, registers labelled offcut remnants and writes an audit entry in one transaction |
| Archived plan API | `GET /api/v1/plans` and `GET /api/v1/plans/{id}` return a stored plan in the same shape as a fresh solve, with per-placement ids, lock flags and the rules |
| Interactive plan editing | the viewer drags/rotates/locks/deletes placements; the server re-validates and stores each edit as a new plan version (`POST /plans/{id}/edit`), archiving the source |
| Locked re-solve | `pinned-2d` / `pinned-1d` keep locked placements exactly in place and fill the free regions around them with the rest of the demand (`POST /plans/{id}/reoptimize`) |
| Exports | CSV cut list (pieces, ordered cuts, offcuts), SVG review drawing, DXF R12 for CAD/CAM and a printable PDF — `GET /plans/{id}/exports?format=csv\|svg\|dxf\|pdf` |
| Costing | every result carries a `cost` breakdown: new material, remnants taken, offcut credit, net cost, cost per part and per m² (`internal/optimizer/costing`) |
| Realized-yield KPIs | `GET /api/v1/kpis?days=90` aggregates plan scorecards (realized = accepted plans, created = pipeline) with a trend series; the dashboard shows KPI cards and a yield chart |
| Campaign planning | ordered jobs share one stock budget: running an item consumes sheets, returns its offcuts as `CMP-…` remnants and archives a draft plan; `POST /campaigns/{id}/run-next`, stock-budget UI at `/campaigns` |
| Rules profiles | named constraint + objective presets stored in the database; `/rules` editor; `?rulesProfileId=` on runs and campaigns so glass/wood differ in data, not code |
| Defect maps | unusable regions on a physical piece (knots, cracks, scratches); solvers avoid them and the validator rejects any plan that covers one |
| Cut-stage limit | `maxCutStages` is enforced on the guillotine cut tree (staged search + `too_many_stages` violation) |
| PostgreSQL schema + seeds | materials, specs, stock formats, parts, routings, jobs, plans, placements, audit |
| Job archive | every non-dry `POST /optimize` persists job + plan + sheets + placements in one transaction |
| Web app | dashboard, plan viewer (2D orthographic / 3D exploded with three.js, 1D bar tracks), solver picker + comparison, materials, parts, stock, jobs (sync run + async queue with live progress), settings |

See [`docs/solver.md`](docs/solver.md) for how the solvers work and how quality
is gated.

Not implemented yet (planned): authentication/roles, CSV imports,
multi-plant, machine-control formats, irregular nesting / 3D bin
packing, OpenAPI-driven codegen.

## Quickstart

Prerequisites: Go (1.24+), Node 22+, pnpm, Docker.

```powershell
# 1. Infrastructure (PostgreSQL on host port 5433 + Adminer on 8081)
.\db\scripts\up.ps1

# 2. Schema and demo data
.\db\scripts\migrate.ps1
.\db\scripts\seed.ps1

# 3. Verify the tables
.\db\scripts\test.ps1

# 4. API (http://localhost:8080)
cd backend
go run ./cmd/cutoptics

# 5. Web app (http://localhost:5173) — in a second terminal
cd frontend
pnpm dev
```

The optimizer also runs without any infrastructure:

```powershell
cd backend
go run ./cmd/cutoptics -demo        # 2D demo plan as JSON
go run ./cmd/cutoptics -demo-bar    # 1D demo plan as JSON
go test ./...                       # solver tests + benchmark golden gate

# Benchmark every solver on committed + generated instances
go run ./cmd/cutoptics bench -dir testdata/benchmarks -random 6 -check
```

## Async jobs (queue + live progress)

`POST /api/v1/optimize` answers synchronously and is fine for interactive runs.
Long jobs belong on the queue, which needs PostgreSQL:

```powershell
# Queue a job (returns 202 with the job id immediately)
curl.exe -s -X POST "http://localhost:8080/api/v1/jobs?solver=polish-2d" `
  -H "Content-Type: application/json" -d "@problem.json"

# Watch it: snapshot, running, progress, done (or failed / cancelled)
curl.exe -s -N "http://localhost:8080/api/v1/jobs/<id>/events"

curl.exe -s "http://localhost:8080/api/v1/jobs/<id>"          # status + plan id + metrics
curl.exe -s -X POST "http://localhost:8080/api/v1/jobs/<id>/cancel"
```

Workers run inside the API process (`CUTOPTICS_WORKERS`, default 2) and claim
jobs with `FOR UPDATE SKIP LOCKED`, so several processes can share one database.
Every finished job is archived as a plan with its sheets and placements; jobs
left running by a crash are failed on the next worker start. The Jobs page in
the web app submits to this queue and shows the live progress bar.

> Port note: the compose file publishes PostgreSQL on **5433** because a native
> PostgreSQL installation commonly owns 5432. Change `POSTGRES_PORT` in
> `deploy/compose/.env` and `DATABASE_URL` in `db/.env` together if you want a
> different port.

## Remnants (leftovers in circulation)

A **stock format** is a catalog size; a **stock item** is one physical piece. The
stock pool is what closes the loop:

```powershell
# What is on the shelves (labelled pieces and remnants)
curl.exe -s "http://localhost:8080/api/v1/stock-items?status=available"

# Ask a run to use the pool before fresh sheets (sync or queued)
curl.exe -s -X POST "http://localhost:8080/api/v1/optimize?includeRemnants=1" `
  -H "Content-Type: application/json" -d "@problem.json"

# Accept a plan: consume used pieces, decrement on-hand, label the offcuts
curl.exe -s -X POST "http://localhost:8080/api/v1/plans/<plan-id>/accept"
```

Acceptance is one transaction: the plan is frozen (`accepted`), the physical
pieces it used become `consumed`, catalog `on_hand_qty` goes down (never below
zero), every reusable offcut is registered as a labelled remnant
(`OFF-<plan>-<sheet>-<offcut>`, with prorated cost and lineage) and an
`audit_log` entry is written. `metrics.remnantSheets` in any result says how many
sheets came from leftovers instead of new stock.

## Editing plans and exports

Plans are immutable versions. The viewer's **Edit layout** mode drags, rotates,
locks and deletes placements; saving calls `POST /plans/{id}/edit`, which
re-validates the layout (bounds, trim, kerf, guillotine feasibility, demand) and
stores it as version `n+1` — the source is archived. **Re-solve with locks**
(`POST /plans/{id}/reoptimize`) keeps every locked placement exactly where it is
and packs the remaining demand into the free regions around them.

```powershell
# Move one placement and lock it, saving a new version
curl.exe -X POST "http://localhost:8080/api/v1/plans/<id>/edit" `
  -H "Content-Type: application/json" `
  -d '{\"operations\":[{\"placementId\":\"<placement-id>\",\"x\":700000,\"y\":600000,\"locked\":true}]}'

# Re-solve around the locks, or download the plan
curl.exe -X POST "http://localhost:8080/api/v1/plans/<id>/reoptimize" -d "{}"
curl.exe -o plan.csv "http://localhost:8080/api/v1/plans/<id>/exports?format=csv"
```

Exports: `csv` (shop-floor cut list), `svg` (review drawing), `dxf` (R12, mm,
layers SHEET/PARTS/OFFCUT/TEXT) and `pdf` (one page per sheet).

## Costing and realized-yield KPIs

Every optimization result (sync, queued or archived plan) carries a `cost`
breakdown in the same currency as `stock_formats.cost_per_unit`:

```json
"cost": {
  "newMaterialCost": 250,   "remnantCost": 0,
  "totalStockCost": 250,    "offcutCredit": 86.96,
  "netCost": 163.04,        "partCost": 139.58,
  "trimCost": 3.77,         "kerfCost": 0.72,   "scrapCost": 23.46,
  "costPerPart": 7.41,      "costPerM2": 10.11
}
```

A sheet from a catalog format costs its full price; a remnant costs its prorated
value; part/trim/kerf/scrap areas carry their share of the sheet cost; the
offcut credit values leftovers that stay in stock. **Net cost = material taken −
offcut credit** — the material the order really consumed.

The dashboard reads `GET /api/v1/kpis?days=90`: `realized` aggregates accepted
plans (what the shop committed to), `created` every plan in the window, and
`series` is the yield/waste trend. All numbers come from the stored plan
scorecards, so the dashboard and a plan's own scorecard can never disagree.

## Campaign planning

A campaign is an ordered set of jobs sharing one stock budget — how a shop
batches a week of orders through the saw:

```powershell
# Create a campaign with a budget (formats and/or the plant's remnants)
curl.exe -X POST "http://localhost:8080/api/v1/campaigns" -H "Content-Type: application/json" `
  -d '{"name":"Week 38","useRemnants":true,"stock":[{"id":"<format-id>","code":"SHEET-3210x2250","width":3210000,"height":2250000,"quantity":4,"costPerUnit":62.5}]}'

# Add the jobs, in the order they should run
curl.exe -X POST "http://localhost:8080/api/v1/campaigns/<id>/items" -H "Content-Type: application/json" `
  -d '{"name":"Order 1042","dueDate":"2026-09-25","parts":[{"id":"p1","code":"PANE-600x400","width":600000,"height":400000,"quantity":6,"allowRotate":true}]}'

# Plan the next item against what is left
curl.exe -X POST "http://localhost:8080/api/v1/campaigns/<id>/run-next" -d "{}"
```

Each run solves the earliest pending item, archives it as a job plus **draft**
plan and stores the reduced budget: used sheets leave, offcuts re-enter as
labelled `CMP-…` remnants with prorated cost, and the campaign becomes `active`
(first run) and `completed` (nothing pending). The budget is a planning sandbox;
accepting a plan is still what changes the plant's real stock. The Campaigns
page (`/campaigns`) shows the remaining budget, the ordered items, due dates and
a button to open each item's plan.

## The DB folder

`db/` is self-contained: schema, queries and the commands to work with them.

| Script | Purpose |
|---|---|
| `up.ps1` / `down.ps1 [-Volumes]` | start / stop PostgreSQL and Adminer |
| `migrate.ps1 [-Status] [-Down] [-To N]` | apply goose migrations or inspect them |
| `new-migration.ps1 -Name add_remnants` | create the next up/down migration pair |
| `seed.ps1` | apply everything in `db/seeds` in name order |
| `psql.ps1` / `psql.ps1 -File some.sql` | interactive shell or run a SQL file |
| `test.ps1` | run the table tests in `db/tests` |
| `generate.ps1` | run `sqlc generate` into `backend/internal/platform/db` |
| `reset.ps1 [-Force]` | drop schema, migrate, seed |

Adding a query: write it in `db/queries/*.sql` with a `-- name:` comment, run
`db/scripts/generate.ps1`, and use the generated function from a module store.

## Architecture in one paragraph

`internal/optimizer` is a pure Go library with a single `Solver` interface
(`core/solver.go`) and a registry. Problems and solutions are plain structs with
**all lengths in integer micrometers**, so every layer is exact: the API, the
database, the solvers and the tests. Modules (`catalog`, `parts`, `jobs`) depend
on small store interfaces; `internal/platform/postgres` implements them with
sqlc-generated queries. The API starts even when PostgreSQL is down — the
optimizer keeps working and results simply are not archived.

## Roadmap (next milestones)

1. ~~Beam search over cut trees, keeping `shelf-2d` as the baseline~~ — done,
   with a golden benchmark gate.
2. ~~Iterated local search that uses the time budget~~ — done (`polish-2d`).
3. ~~Column generation with DP pricing for 1D homogeneous orders~~ — done
   (`cg-1d`, with an LP lower bound in the plan notes).
4. ~~Column generation for 2D guillotine patterns and a free-cutting MaxRects
   solver~~ — done (`cg-2d` two-stage pricing with demand rationing,
   `maxrects-2d` for `cutMode: free`).
5. ~~Async jobs: Postgres-backed queue (`FOR UPDATE SKIP LOCKED`), SSE progress
   with best-so-far plans~~ — done (`POST /api/v1/jobs`, worker pool, SSE stream,
   cancel, stale-job recovery; the Jobs page shows live progress).
6. ~~Remnant lifecycle: physical sheets, offcut labels, remnant-first
   allocation~~ — done (`stock_items`, `/api/v1/stock-items`, `preferRemnants`,
   labelled offcuts on acceptance, audit trail).
7. ~~Interactive plan editing with locks and re-solve; exports (PDF/DXF/CSV/SVG)~~
   — done (validated plan versions, `pinned-2d`/`pinned-1d` re-solve around
   locks, CSV/SVG/DXF/PDF downloads).
8. ~~Costing, realized-yield KPIs~~ — done (`cost` breakdown on every result,
   `GET /api/v1/kpis`, dashboard cards and a yield trend).
9. ~~Campaign planning (group jobs into a campaign with a shared stock budget
   and schedule)~~ — done (`campaigns` + `campaign_items`, ordered `run-next`
   with a shrinking/refilling stock budget, CMP-… remnant labels, campaign UI).

Remaining backlog (see the forward plan, `plan.txt`): auth/sessions/RBAC, CSV
imports, machine-control formats, multi-plant, irregular nesting
and 3D bin packing, OpenAPI-driven codegen; the original v1 plan is kept as
`plan-v1.txt`.
