# internal/modules/ — HTTP feature modules

One folder per feature. Each module owns its request/response DTOs, its routes
and a **small store interface** that `platform/postgres` implements. Modules do
not import each other; shared helpers live in `platform/httpx`.

| Module | Files | Responsibilities | Store interface |
|---|---|---|---|
| `catalog` | `store.go` | materials, material types (specs) + stock formats | `ListMaterials`, `CreateMaterial`, `ListMaterialSpecs`, `CreateMaterialSpec`, `ListStockFormats`, `CreateStockFormat` |
| `parts` | `store.go` | part catalog; finished sizes + routings → cut sizes (`CutSize`) | `ListParts`, `CreatePart` |
| `assemblies` | `store.go` | product catalog: assemblies (overall size) + components (glass panels, frame beams) rendered in 2D/3D | `ListAssemblies`, `GetAssembly`, `CreateAssembly` |
| `jobs` | `service.go`, `queue.go`, `worker.go`, `http.go` | solve orchestration, demo problems, run archiving, asynchronous queue + SSE progress, remnant injection | `SaveRun` (sync), `QueueStore` (async), `RemnantSource` (optional) |
| `stock` | `store.go` | physical stock pieces: full pieces and labelled remnants, each with an optional **defect map** (unusable regions) | `ListItems`, `GetItem`, `CreateItem`, `UpdateItem` |
| `plans` | `store.go`, `http.go`, `edit.go` | archived plans (read), acceptance, validated edits, locked re-solve, exports | `ListPlans`, `GetPlan`, `AcceptPlan`, `SaveVersion` |
| `campaigns` | `store.go`, `budget.go`, `http.go` | ordered jobs sharing one stock budget; `run-next` solves the next item and updates the budget | `ListCampaigns`, `GetCampaign`, `CreateCampaign`, `UpdateCampaign`, `AddItem`, `DeleteItem`, `NextItem`, `CompleteItem` |
| `kpis` | `store.go` | realized-yield and costing aggregates over plan scorecards | `KPIs` |
| `rules` | `store.go` | named constraint + objective presets (kerf, trim, grain, cut mode, offcut policy, weights); resolved into a problem via `?rulesProfileId=` | `ListProfiles`, `GetProfile`, `CreateProfile`, `UpdateProfile` |

## jobs

- `Service` holds the solver `Registry`; `Run` executes `optimizer.Solve`,
  validates the plan, and archives it through `Store.SaveRun` unless
  `dryRun` is set.
- `DemoProblem()` / `DemoBarProblem()` back the demo endpoints (also used by
  the tests and the frontend fallback data).
- **Synchronous routes**: `POST /optimize` (with `?solver=` and `?dryRun=`),
  `GET /solvers`, `GET /demo/plan`, `GET /demo/bar-plan`.
- **Asynchronous pipeline** (`queue.go`, `worker.go`):
  - `POST /api/v1/jobs` snapshots the problem into `cut_jobs` and returns `202`
    with the job id — it works even when no worker is running.
  - `Worker` runs N goroutines (`CUTOPTICS_WORKERS`, default 2). Each claims the
    oldest queued job with `FOR UPDATE SKIP LOCKED`, runs the solver, publishes
    progress to the `platform/events` hub and archives the plan in one
    transaction (`Complete`). On startup it fails jobs left running by a crash
    (`ReleaseStale`).
  - `GET /api/v1/jobs/{id}` returns the `JobView` (status, plan id, metrics,
    archived result); `POST …/cancel` cancels a queued or running job;
    `GET …/events` streams SSE (`snapshot`, `running`, `progress`, `done`,
    `failed`, `cancelled`) with a 2 s database poll as a cross-process fallback.
  - Progress is throttled in the worker (250 ms, first frame immediate) so a
    fast solver cannot flood the stream.
- **Remnants**: with `?includeRemnants=1` the handler asks the optional
  `RemnantSource` for the plant's available labelled remnants (optionally
  filtered by `materialSpecId`), drops the ones that do not match the problem's
  dimension profile and appends them to the problem snapshot. A nil source (no
  database) skips the lookup; a lookup error is a `500 remnant_lookup_failed`.

## assemblies

- A product is an overall size plus an ordered list of `components`. Each
  component is a box in the assembly's local frame (origin bottom-left-front,
  x right, y up, z out of the wall), so the same data drives the 2D elevation
  and the 3D scene on the Products page.
- `POST /assemblies` writes the assembly and all components in one transaction;
  `kind` is `window | door | generic` and a component kind is
  `beam | panel | custom`. The product optionally binds to a `materialSpecId`
  (the default for its components) and a component carries a `quantity`.
- Assemblies are still stored/served here rather than solved server-side: the
  frontend explodes components into cut parts, **splits them by (material spec,
  dimension profile)** and posts one problem per group to `/optimize`. See
  `frontend/src/features/optimizer/buildProblem.ts`.

## stock

- One physical piece per row: full sheet/bar or labelled remnant, with status
  `available | reserved | consumed | retired`, location, prorated cost and
  lineage (`parentPlanId`, `parentSheetIndex`).
- `POST` requires a label and a 1D length or a 2D width+height; when a format is
  referenced, missing code/dimensions/cost are inherited from it.
- `PATCH` changes label, location or pool status. `consumed` cannot be set
  through the API — plan acceptance does that.

## plans

- `GET /plans` lists summaries; `GET /plans/{id}` rebuilds the stored layout
  into the same `OptimizeResult` shape as a fresh solve, with per-placement
  `id`/`locked` values and the plan rules.
- `POST /plans/{id}/accept` is the lifecycle step: in one transaction it consumes
  the physical pieces the plan used, decrements `stock_formats.on_hand_qty`
  (never below zero), registers every reusable offcut as a labelled remnant
  (`OFF-…`, prorated cost, plan/sheet lineage), marks the plan `accepted` and
  writes an `audit_log` entry. Accepting twice → `409 not_acceptable`.
- `POST /plans/{id}/edit` applies move/rotate/lock/delete operations, re-runs
  `validator.Validate` plus an edge-trim check and stores the result as the next
  version (`plans.version + 1`, `parent_plan_id`, source archived). A layout that
  cannot be produced → `422 edit_invalid` with the violation list.
- `POST /plans/{id}/reoptimize` builds `core.PinnedSheet`s from the locked
  placements, runs the solver (registry picks a pinned-capable one) and stores
  the next version. `pinned-2d`/`pinned-1d` keep locks exactly in place.
- `GET /plans/{id}/exports?format=…` streams CSV/SVG/DXF/PDF from
  `internal/optimizer/export`; `sheet=` selects one sheet.
- Only `draft`/`approved` plans can be edited or accepted; accepted/archived
  plans are frozen.

## kpis

- `GET /kpis?days=90` (or `from`/`to`, RFC3339 or `YYYY-MM-DD`) aggregates the
  JSONB scorecards stored on plans: `realized` = accepted plans, `created` =
  all plans in the window, `series` = accepted plans over time.
- Yield/waste/cost-per-part are derived in the store, never stored twice, so the
  dashboard always agrees with a plan's own scorecard.
- Nil store (no database) answers `503 database_unavailable` like every other
  read endpoint.

## campaigns

- A campaign is an ordered set of items (jobs) plus one shared stock budget
  (`stock` JSONB, with `initial_stock` kept for reference).
- `POST /campaigns/{id}/run-next` solves the earliest pending item with
  `BuildItemProblem` (item parts + remaining budget + campaign rules/objective,
  seed + item seq), then `ConsumeBudget` shrinks the budget and returns the
  offcuts as labelled `CMP-…` remnants with prorated cost. `CompleteItem` stores
  job + plan + item status + budget in one transaction under a campaign row
  lock, and derives the status: `active` after the first run, `completed` when
  nothing is pending.
- `budget.go` is pure and unit-tested: budget arithmetic needs no database.
- Only `draft`/`active` campaigns accept items or runs; the budget is a planning
  sandbox — accepting a plan is still what changes plant stock.

## Patterns

- Handler structs are created with `NewHandler(store)` and expose `Routes()`
  returning a chi sub-router.
- A **nil store is legal**: handlers answer
  `503 database_unavailable` instead of panicking (the API must keep working
  when PostgreSQL is down). Keep this behaviour for any new read/write
  endpoint.
- Decode with `httpx.DecodeJSON` (size-limited, `DisallowUnknownFields`) and
  respond with `httpx.JSON` / `httpx.Error`.
- Input DTOs are separate from `optimizer/core` structs; convert at the edge.
- Every new endpoint needs a matching entry in `backend/api/openapi.yaml`.
- The async queue is optional: handlers check `h.queue == nil` and answer
  `503 queue_unavailable`, so the API keeps working without PostgreSQL.
