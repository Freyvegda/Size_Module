# internal/modules/ — HTTP feature modules

One folder per feature. Each module owns its request/response DTOs, its routes
and a **small store interface** that `platform/postgres` implements. Modules do
not import each other; shared helpers live in `platform/httpx`.

| Module | Files | Responsibilities | Store interface |
|---|---|---|---|
| `catalog` | `store.go` | materials + stock formats | `ListMaterials`, `CreateMaterial`, `ListStockFormats` |
| `parts` | `store.go` | part catalog (finished sizes, allowance concept) | `ListParts`, `CreatePart` |
| `jobs` | `service.go`, `queue.go`, `worker.go`, `http.go` | solve orchestration, demo problems, run archiving, asynchronous queue + SSE progress, remnant injection | `SaveRun` (sync), `QueueStore` (async), `RemnantSource` (optional) |
| `stock` | `store.go` | physical stock pieces: full pieces and labelled remnants | `ListItems`, `GetItem`, `CreateItem`, `UpdateItem` |
| `plans` | `store.go`, `http.go` | archived plans (read) and acceptance | `ListPlans`, `GetPlan`, `AcceptPlan` |

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

## stock

- One physical piece per row: full sheet/bar or labelled remnant, with status
  `available | reserved | consumed | retired`, location, prorated cost and
  lineage (`parentPlanId`, `parentSheetIndex`).
- `POST` requires a label and a 1D length or a 2D width+height; when a format is
  referenced, missing code/dimensions/cost are inherited from it.
- `PATCH` changes label, location or pool status. `consumed` cannot be set
  through the API — plan acceptance does that.

## plans

- `GET /plans` lists summaries; `GET /plans/{id}` rebuilds the stored layout into
  the same `OptimizeResult` shape as a fresh solve (the archived result JSON is
  preferred; a reconstruction with `validator.Validate` + `core.Score` is the
  fallback).
- `POST /plans/{id}/accept` is the lifecycle step: in one transaction it consumes
  the physical pieces the plan used, decrements `stock_formats.on_hand_qty`
  (never below zero), registers every reusable offcut as a labelled remnant
  (`OFF-…`, prorated cost, plan/sheet lineage), marks the plan `accepted` and
  writes an `audit_log` entry. Accepting twice → `409 not_acceptable`.

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
