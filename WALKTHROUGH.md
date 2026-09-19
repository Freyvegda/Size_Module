# Size Module — the complete beginner's walkthrough

This document explains the whole project from zero: the problem it solves, the
environment that problem lives in, how the solution is engineered, and what every
part of the backend and frontend does.

It was written by reading the repo top to bottom (guides, source, tests,
benchmarks, schema), cross-checking the architecture with the knowledge graph in
`graphify-out/`, and verifying current numbers against `testdata/benchmarks/golden.json`.
Where the docs and the code disagree, the code wins and the drift is called out.

**Suggested reading order:** sections 1–3 first (problem and environment), then
5 (mental model), then 6–8 (backend, database, frontend), then 9 (walkthroughs)
and 11 (current limits). Section 12 is a cheat sheet you can jump to any time.

---

## Contents

1. [The problem](#1-the-problem)
2. [The environment: where this problem lives](#2-the-environment-where-this-problem-lives)
3. [The domain vocabulary](#3-the-domain-vocabulary)
4. [How the project solves it (the basic approach)](#4-how-the-project-solves-it-the-basic-approach)
5. [The big picture](#5-the-big-picture)
6. [Backend, package by package](#6-backend-package-by-package)
7. [The database and the `db/` workspace](#7-the-database-and-the-db-workspace)
8. [Frontend, file by file](#8-frontend-file-by-file)
9. [End-to-end walkthroughs](#9-end-to-end-walkthroughs)
10. [Quality gates: how we know a plan is good and valid](#10-quality-gates-how-we-know-a-plan-is-good-and-valid)
11. [What works, what doesn't (yet), and known quirks](#11-what-works-what-doesnt-yet-and-known-quirks)
12. [Cheat sheet and reading order for new contributors](#12-cheat-sheet-and-reading-order-for-new-contributors)

---

## 1. The problem

### 1.1 In one sentence

A factory has **stock material** (big glass sheets, wooden panels, metal bars)
and a list of **parts** it must produce (window panes, shelves, rails). It must
decide *which pieces to cut from which stock, and where*, so that:

- demand is fulfilled (or the shortfall is explained),
- as few sheets/bars as possible are used,
- waste is as small as possible,
- and the result can actually be produced on the shop's machines.

This is the classic **cutting stock / nesting problem** (also called 1D and 2D
bin packing). Every material-processing shop solves it daily, usually by hand or
with Excel. Good cuts vs bad cuts is literally money: on the project's own
benchmark set, a naive packing leaves **~35 % of the glass as waste**, while the
best shipped solver gets that down to **~24.5 %**.

### 1.2 Make it concrete

Take the demo order in the code (`jobs.DemoProblem()`):

| Parts wanted | Size | Qty |
|---|---|---|
| Window pane | 1200 × 1400 mm | 6 |
| Window pane | 800 × 1000 mm | 6 |
| Shelf | 500 × 250 mm | 10 |

Available stock: 4 large sheets (3210 × 2250 mm) and 3 small sheets (2440 × 1220 mm).

A human would "just" arrange rectangles on sheets. Why is that hard?

1. **Combinatorics.** 22 pieces can be arranged in astronomically many ways.
   Optimal cutting stock is NP-hard. You cannot brute-force it.
2. **The machine constrains the layout.** A panel saw or glass cutter makes
   *guillotine* cuts only: every cut must run edge-to-edge across the piece.
   Some layouts that look perfect on screen are impossible to cut — the classic
   four-rectangle "pinwheel" has no valid first cut. The project therefore
   *proves* a layout is cuttable before showing it (see the cut-tree validator,
   section 6.2.7).
3. **Real losses are unavoidable.** The saw blade eats material (**kerf**), the
   sheet edges are unusable (**trim**), material grain forbids rotation, and
   leftovers matter: a leftover big enough to reuse (**offcut/remnant**) is an
   asset; anything smaller is **scrap**.
4. **Conflicting goals.** Using fewer sheets can mean more scrap; filling demand
   *completely* can mean accepting a worse yield. You need an explicit
   **objective** (see `core.DefaultWeights`), not a vibe.

### 1.3 The "material-agnostic" idea

The project is deliberately not a glass program or a wood program. Glass, sheet
metal, MDF and fabric are all described with the same words; the differences
(kerf width, trim, guillotine vs free cutting, grain rules, offcut minimum sizes)
live in a **rules profile** — stored data, not code branches. This is a hard
project rule: *"there is no glass or wood code path in the engine, only different
rule values."*

### 1.4 Scope of this first slice

The repo is the **cut optimizer** — the first vertical slice of a bigger
material-optimization platform. In scope: catalogue (materials, stock, parts),
2D and 1D solvers, validation, a plan viewer (2D/3D and 1D bars), run archiving,
the asynchronous job queue with SSE progress, the **remnant lifecycle**
(physical stock pieces, labelled offcuts, remnant-first allocation, plan
acceptance) and **plan editing** (validated versions, locked placements,
re-solve, CSV/SVG/DXF/PDF exports), plus **costing/KPIs**, **products
(assemblies)** and **campaigns** that share a stock budget. Out of scope for now
(see `plan.txt` and the README roadmap): auth, 3D bin packing, irregular
nesting.

---

## 2. The environment: where this problem lives

Understanding the environment explains half the design decisions.

**A single plant, self-hosted.** One factory runs the software; there is no
multi-tenancy, no cloud dependency, no Kubernetes. The database schema still
carries `plant_id` columns so multi-plant is possible later, but v1 is one site.

**Two kinds of geometry.**
- *1D*: bars, profiles, tubes — stock with a length; parts are lengths.
- *2D*: sheets, panels, plates — stock with width × height; parts are rectangles.

**Two kinds of cutting machines.**
- *Guillotine*: panel saws, glass cutters, shears — every cut goes edge to edge.
- *Free*: CNC routers, laser/waterjet — arbitrary cut paths, served by the
  `maxrects-2d` solver for `cutMode: free` problems.

**Physical facts that drive the math.**
| Fact | Project name | Effect |
|---|---|---|
| Blade removes material on each cut | **kerf** | pieces must be separated by ≥ kerf |
| Sheet edges are unusable | **trim** | usable area is inset by trim on all sides |
| Material has a direction | **grain** | grain-bound parts cannot be rotated |
| Some leftovers are reusable | **offcut / remnant** | minimum size is policy (`offcutMinW/H`) |
| Rest is garbage | **scrap** | the waste you want to minimize |
| Operators need instructions | **cut steps** | ordered, human-readable cut list per sheet |

**The software environment.**
- Go 1.27 backend, pure Go (no CGO) so it builds to a single static binary —
  suitable for a plant PC.
- PostgreSQL 16 (optional at runtime) for catalogue and run archive.
- React 19 SPA served by Vite in dev; talks to the Go API over `/api`.
- Everything local: ports 5173 (web), 8080 (API), 5433 (Postgres), 8081 (Adminer).

**The precision environment.** Shop reality forces a units rule: all lengths in
the database, API and solvers are **integer micrometers** (`int64`, 1 mm = 1000 µm).
Floats appear only for costs, percentages and areas in m². Integers make
geometry exact — no floating-point rounding can create an accidental
overlap of 0.000001 mm. Conversion to mm happens only in the UI
(`frontend/src/lib/format.ts`).

---

## 3. The domain vocabulary

The canonical list is `docs/glossary.md`. The essentials:

| Term | Meaning |
|---|---|
| **Material** | family of stock, e.g. GLASS, ALU, MDF |
| **Material spec** | concrete variant: thickness, finish, colour, grade |
| **Stock format** | catalogue size: `SHEET-3210x2250`, `BAR-6000` |
| **Stock item** | a physical piece of a format (a sheet, a bar, a remnant) |
| **Part** | something to produce, defined by its **finished** size |
| **Routing** | ordered operations on a part (grinding, edge deletion, …), each with an **allowance** |
| **Cut size** | finished size + allowances — what the solver receives |
| **Kerf** | material consumed by the tool per cut |
| **Trim** | unusable strip at stock edges |
| **Guillotine cut** | straight cut across the whole piece, edge to edge |
| **Cut tree** | recursive structure of guillotine cuts producing a layout |
| **Pattern** | a sheet layout; fewer distinct patterns = fewer machine setups |
| **Placement** | one part at one position on one sheet |
| **Plan / Solution** | the complete answer (sheets, placements, offcuts) + metrics + notes |
| **Rules profile** | stored constraints (kerf, trim, rotation, grain, cut mode, offcut policy) |
| **Objective** | weighted goal: fulfil priority demand, minimize sheets, scrap, patterns, offcut area, cost |
| **Yield / Waste** | part area ÷ stock area; waste = 100 % − yield, split into trim, kerf, scrap, offcuts |
| **Baseline** | deliberately simple solver used to judge improvements honestly (`shelf-2d`) |

---

## 4. How the project solves it (the basic approach)

The design rests on five load-bearing decisions. Once you internalise these, the
rest of the code is details.

### 4.1 One seam: `core.Solver`

Everything algorithmic plugs into a single interface
(`backend/internal/optimizer/core/solver.go`):

```go
type Solver interface {
    Name() string
    Version() string
    Capabilities() Capabilities
    Solve(ctx context.Context, p Problem, progress ProgressFunc) (Solution, error)
}
```

`Problem` and `Solution` are plain structs. Solvers never touch the database,
HTTP, or wall-clock time beyond the given budget. Adding a new algorithm means
implementing this interface and registering it — no caller changes.

### 4.2 A registry, not `if/else`

`optimizer.DefaultRegistry()` registers every shipped solver. Each solver
declares `Capabilities` (dimension profile, cut mode, rotation, grain, rank).
When the caller does not name a solver, the registry picks the best:
**lower rank wins, ties break by name**. A *portfolio* solver is just another
solver that wraps others, so "best of all strategies" can be the default
without hardcoding a winner in callers.

### 4.3 Validate before you show

Every solution passes `validator.Validate` before it reaches the API response or
the database. It checks bounds, piece sizes, orientation/grain/rotation,
kerf separation, demand limits and **guillotine cut-tree feasibility**. A plan
that cannot be produced is a bug, never an answer.

### 4.4 Determinism

Same problem + same seed + same solver version ⇒ identical plan. Candidate
orders, rectangle orders and tie-breaks are total orders. This is what makes the
benchmark golden file meaningful and shop results reproducible.

### 4.5 Degrade, don't die

PostgreSQL is optional at startup. Without it, the optimizer, solver list and
demo endpoints still work; catalogue endpoints answer `503 database_unavailable`;
runs are simply not archived. The frontend mirrors this: it shows "API online ·
DB offline" and the viewer falls back to a built-in sample plan.

---

## 5. The big picture

```
Browser (React 19 SPA, :5173)
  │  /api/v1/...  (Vite dev proxy → :8080; production serves relative paths)
  ▼
Go API process (cmd/cutoptics, :8080)
  ├─ chi router: middleware, /healthz, /api/v1/*
  ├─ modules: jobs · catalog · parts · stock · plans   ← HTTP DTOs + store interfaces
  │     └─ jobs.Service.Run ────────────────────────────┐
  ├─ optimizer (pure Go, no DB/HTTP)                    │
  │     ├─ core/      Problem, Solution, Rules, Score   │
  │     ├─ pack2d/    shelf-2d · beam-2d · polish-2d · best-2d
  │     ├─ pack1d/    ffd-1d  (+ cg-1d and best-1d: implemented, not wired)
  │     ├─ geom/      rect math, cut-tree reconstruction
  │     ├─ cutter/    layout → ordered cut steps
  │     ├─ validator/ mandatory gate
  │     ├─ explain/   waste notes + area lower bound
  │     └─ bench/     instances, golden gate  ← go test / bench -check
  ├─ platform/postgres: Store implements module stores
  │     └─ platform/db: sqlc-generated queries (pgx v5)
  └─ optional PostgreSQL (:5433) — jobs, plans, placements, catalogue
```

Flow of one optimization request:

```
POST /api/v1/optimize  {problem JSON}
  → jobs.Handler.optimize
      decode + Normalize (defaults, units)
      → jobs.Service.Run → optimizer.Solve
          → registry picks solver (or ?solver=)
          → solver.Solve (time-budgeted, any-time, progress callbacks)
          → validator.Validate    (errors/warnings)
          → explain.Notes         (plain-language notes)
      → unless ?dryRun=1: postgres.Store.SaveRun  (job + plan + sheets + placements, 1 tx)
  → JSON { id?, result: { solution, violations, score } }
```

---

## 6. Backend, package by package

Module path: `github.com/size-module/backend`. One process serves the REST API
and hosts the optimizer. Layout:

```
backend/
  cmd/cutoptics/          main.go (server + demo flags) · bench.go (bench subcommand)
  api/openapi.yaml        hand-written OpenAPI 3.1 contract (source of truth)
  internal/modules/       catalog · parts · jobs        (HTTP layer)
  internal/optimizer/     core · geom · pack1d · pack2d · lp · cutter · validator · explain · bench
  internal/platform/      config · httpserver · httpx · id · postgres · db (generated)
  testdata/benchmarks/    glass-mixed.json, wood-panels.json, metal-bars.json, golden.json
```

### 6.1 Entry point — `cmd/cutoptics/main.go`

- `bench` as first argument dispatches to `runBench` (kept out of the normal flag
  set so the common path stays simple).
- Flags: `-demo` (print 2D demo result as JSON and exit), `-demo-bar` (1D demo),
  `-addr` (override listen address).
- Config comes from environment (`config.Load`), logging switches format by env
  (text in dev, JSON otherwise).
- Builds `optimizer.DefaultRegistry()` and `jobs.NewService(registry)`.
- Tries `postgres.Connect`; on failure it logs a warning and continues in
  **optimizer-only mode**.
- Builds `httpserver.New(httpserver.Deps{...})` with handlers for jobs, catalog
  and parts; a nil DB store is legal.
- Runs `http.Server` with a 10 s header timeout; waits for SIGINT/SIGTERM, then
  graceful shutdown with a 10 s deadline.

`cmd/cutoptics/bench.go` implements the `bench` subcommand: flags `-dir`,
`-random N`, `-seed`, `-solvers`, `-budget`, `-check`, `-update`, `-golden`,
`-tolerance`. It loads committed instances, generates deterministic random ones,
runs every solver, prints the table, optionally rewrites the golden file, and
exits non-zero on a regression in `-check` mode (CI gate).

### 6.2 The optimizer — `internal/optimizer/`

#### 6.2.1 `optimizer.go` — the facade

- `DefaultRegistry()` registers: `ffd-1d` (pack1d), `shelf-2d`, `beam-2d`,
  `polish-2d`, `best-2d` (portfolio wrapping the 2D three).
- `Solve(ctx, problem, solverName, registry, progress)`:
  1. `core.Normalize` fills defaults (rules, cut mode, grain, weights, budget,
     quantities) and, when `preferRemnants` is set, orders the stock list so
     physical remnants are offered first (stable, so plans stay deterministic).
  2. Picks the solver: by name if given, else `registry.ForProblem`.
  3. Runs it.
  4. `validator.Validate` collects violations.
  5. `explain.Notes` appends human-readable notes.
  6. Returns `Result{Solution, Violations, Score}` — score from `core.Score`.

#### 6.2.2 `core/` — the language of the engine

`types.go` defines the domain structs, frozen for other packages:

- `Dim = int64` micrometers plus `Millimeter`, `Meter` constants and conversion
  helpers (`FromMM`, `ToMM`, `AreaM2`, `LengthM`).
- `Part`: ID, code, material spec, cut size (Length for 1D; Width/Height for 2D),
  quantity, grain, `AllowRotate`, priority (1 = most important), group.
- `StockItem`: ID, format, code, label, dimensions, quantity, cost, `IsRemnant`,
  `Defects` (unusable regions).
- `Rules`: kerf, trim, allowRotate, grainMode, cutMode (guillotine/free),
  maxCutStages, offcut minimum sizes (2D and 1D), minPartDim, maxPartsPerSheet,
  oversAllowedPct, `preferRemnants` (offer physical remnants before fresh stock;
  see `core.PrioritizeRemnants`).
- `Weights` + `Objective`: the scalar goal. `DefaultWeights()` = fill priority
  demand first (1000), then sheets (1), scrap (1), patterns (1), offcut area
  (0.25), cost (1).
- `DefaultRules()`: 4 mm kerf, 10 mm trim, rotation on, guillotine, offcuts from
  300 × 300 mm / 300 mm.
- `Normalize(Problem)`: default rules when empty, default cut mode/grain/weights,
  budget default 5000 ms, quantity ≥ 1.
- `Problem`: parts + stocks + rules + objective + `BudgetMS` + `Seed`.
- `Placement`: part on a sheet (x, y, w, h, rotated, priority).
- `SheetPlan`: one sheet's layout — stock info, placements, `Offcuts`,
  `CutSteps`.
- `Metrics`: the scorecard — sheet count, parts placed/requested, areas
  (stock/part/kerf/trim/scrap/offcut in m²), yield %, waste %, pattern count,
  cost, `remnantSheets` (sheets cut from physical remnants), 1D lengths, elapsed
  ms. `core.Score` charges only fresh sheets: remnant sheets are already-paid-for
  capacity.
- `Solution`: solver name/version, seed, sheets, unplaced parts, metrics, notes.

`solver.go` defines `Capabilities`, `ProgressFunc` (any-time best-so-far
callbacks) and the `Registry` (register/get/list/names; `ForProfile` and
`ForProblem` rank candidates; cut-mode compatibility rule: a guillotine layout
is always acceptable to a free-cutting machine, the reverse is not).

`profile.go` decides 1D vs 2D from the parts (any 2D part ⇒ 2D; length-only ⇒ 1D;
default 2D) and implements the cut-mode compatibility rule.

`score.go` has the two functions every solver relies on:

- `Summarize(...)` derives all metrics from sheets + unplaced + elapsed, so all
  solvers are measured identically. Scrap = stock − part − offcut area (floored
  at 0); yield/waste from part/stock; pattern count = distinct part-code
  multisets per sheet.
- `Score(p, metrics)` = `fill × 1000 − sheets − scrap − patterns − 0.25 × offcut
  − cost`, rounded to 6 decimals. Higher is better. Note: **fulfilling demand
  outranks saving area** by construction.

`portfolio.go` is the generic "best of strategies" machinery: filters strategies
that can serve the problem (dimension + cut mode), splits `BudgetMS` equally
(min 500 ms each), forwards only improving partial plans (monotonic progress),
prints one note explaining what it compared and which strategy won by score.

#### 6.2.3 `geom/` — rectangle math and cut trees

`rect.go`: `Separated(a,b,gap)` (kerf-aware non-overlap), `Intersects`, `Union`,
`SplitV/SplitH` (cut a rect at a position, consuming kerf between halves),
`UsableArea` (area after trim inset).

`guillotine.go`: **`BuildCutTree`** — the feasibility proof at the heart of the
guillotine promise. Given a region and the part rectangles in it, it searches
for a sequence of edge-to-edge cuts that separates the parts:

- candidate cut lines are derived from part edges (`r.Right()`, `r.X − kerf` for
  vertical; `r.Bottom()`, `r.Y − kerf` for horizontal), tried from the low
  position upward — reproducing the workshop habit "rip strips first, then
  crosscut";
- a cut is only valid if every rectangle lies fully on one side or the other
  (no piece may straddle a cut);
- recursion is bounded by a node budget (default 50 000) so a pathological
  layout cannot hang the server;
- ordering is deterministic (same input ⇒ same tree).
- `Instructions(tree, kerf)` walks the tree into ordered human-readable steps
  ("vertical (rip) cut at 1200 mm from the region edge, kerf 4 mm").

If no tree exists, the layout is physically impossible (think pinwheel) and the
caller rejects it.

#### 6.2.4 `pack2d/` — the 2D sheet solvers

Common helpers: `instance` (one part copy to place; parts are expanded by
quantity), `orientation` (normal or rotated), `expand`, `sortInstances`
(priority, then area, height, width, code — a total order for determinism),
`allTaken`, `collectUnplaced` (aggregated per part with a reason: "no stock or
remaining capacity" vs "part is larger than any usable stock size"),
`hitsDefect`, `itoa` (local, to avoid formatting dependencies).

**`shelf.go` — `shelf-2d` (rank 20, the baseline).**
The simplest real approach: pack parts into horizontal strips, then parts into
each strip left to right. A strip is created by a full-width rip cut and all
pieces in it share the same height, so every layout is guillotine-feasible by
construction. Per unused stock copy: inset by trim, scan remaining instances in
order, try to fit into an existing strip (with kerf between pieces), else create
a new strip below the last one. `findOffcuts` reconstructs reusable leftovers
(the unused tail of each strip and the unused band below the last strip) and
checks them against the offcut minimum policy. `overheads` estimates trim and
kerf areas for the metrics; `cutter.ForSheet` attaches cut steps. It calls
`progress` after every sheet, making it any-time-ish (it stops early if the
context expires or everything is placed).

**`beam.go` — `beam-2d` (rank 10).**
Beam search over guillotine cut trees: instead of committing to one strip order,
it keeps `Width = 24` alternative states per expansion level (24 was measured
best on the benchmark set; wider is not monotonically better). A state
(`beamNode`) is a set of free rectangles plus a linked list of placements via
parent pointers — copying a state costs one small slice, not the whole history.
Each step:

1. considers the 8 **smallest free rectangles first** (filling tight gaps keeps
   big areas free for big pieces);
2. tries the largest remaining part groups plus the two smallest (so slivers can
   still be filled), max 8 candidates per step;
3. places the piece in the lower-left corner of the region and creates **both**
   guillotine residuals: mode 0 = crosscut above the piece first, mode 1 = rip
   beside it first;
4. scores every child with
   `packed area − 0.25 × unusable free area − 1 % × usable area × fragments`,
   sorts children by score (ties: fewer free rects, then part code) and keeps
   the best 24.

When the context expires it returns the best state found so far (any-time).
Finished states become sheets with offcuts (free rects ≥ offcut minimum) and cut
steps via the same cut-tree reconstruction.

**`improve.go` + `polish.go` — `polish-2d` (rank 3).**
Beam search is constructive: it never revisits a decision. `Improve` takes the
beam's plan and attacks the expensive mistakes in order of value:

1. **dissolve a sheet** — move every piece of the least-filled sheet into the
   other sheets, largest piece first, best-fit destination (the fullest sheet
   that still accepts it). Only commits if *every* piece lands; then a whole
   sheet is gone. This is the only move that reduces sheet count.
2. **merge two sheets** — pack both part sets into one sheet of the larger
   format.
3. **repack a sheet** with a wider beam and more candidates — cannot remove a
   sheet but often turns unusable scrap into reusable offcuts.

Every candidate is rebuilt with the same guillotine-safe beam packer and scored
with the real objective; a move is accepted only if it improves the score by at
least **0.01 points** (without that threshold the search chased rounding noise
and never settled). Bounds: the tighter of context deadline and `BudgetMS`,
**25 rounds**, or **14 fruitless rounds**. Work is capped per round (3 emptiest
sheets, 3 sheet pairs, one repack), and RNG rotation over candidates keeps
repeated rounds from retrying the same targets. On the benchmark set it
converges in ~250–500 ms.

`polish.go` wires it together: run the beam, run `Improve`, recompute metrics,
set the solver name, and add a note saying either how many sheets the local
search removed or that the beam plan stands.

**`portfolio.go` — `best-2d` (rank 1).**
`core.Portfolio` wrapping shelf + beam + polish, capabilities 2D guillotine,
rank 1 — so when a caller says "just optimize", this is what runs by default.

**`pinned-2d`** — added with the editing milestone. It packs the residual demand
around planner-locked placements: it builds a guillotine cut tree over the
locked pieces (which proves they are separable), subtracts each piece from its
leaf region into the five disjoint bands around it, fills those free regions
shelf-style, and packs the rest of the stock like `shelf-2d`. Only solvers with
`Capabilities.Pinned` are considered when a problem carries pins, so an ordinary
solver can never silently move a locked piece.

**`solution.go`** — `assembleSolution`: the shared "build a Solution shell +
`Summarize` metrics + timing" helper every 2D solver uses.

#### 6.2.5 `pack1d/` — bar and profile solvers

**`ffd.go` — `ffd-1d` (the registered 1D solver).**
Classic First-Fit-Decreasing: sort pieces by priority, then length descending,
then code; for each bar, place pieces left to right, first fitting piece wins
(with kerf between pieces), track the cursor. Bars are represented as
`SheetPlan`s with `Width = bar length`, a nominal 1 mm height so area metrics
stay meaningful, `X` as the position along the bar. Offcut: remainder after the
last piece if ≥ `OffcutMinLength`. Cut steps come from the same guillotine
cutter. Metrics include `stockLengthM` and `usedLengthM`.

**`cg.go` — `cg-1d` (implemented and unit-tested, currently *not registered*).**
Gilmore–Gomory **column generation**, the classical exact-ish method for 1D
cutting stock and the strongest approach for homogeneous orders (many pieces of
few lengths):

- **Master LP**: minimize the cost of the patterns used, subject to meeting
  demand (equality form with one surplus column per part type so demand may be
  over-satisfied, never under). Solved with a small dense two-phase simplex
  (`lp/` package).
- **Pricing**: find the pattern (knapsack) with the highest dual value per
  format, using an exact DP over lengths rounded conservatively to millimetres
  (items rounded up, capacity down) plus an exact final fit check. If the best
  pattern's value exceeds its cost, add it as a new column and iterate
  (≤ 200 rounds, ≤ 600 columns).
- **Assembly**: floor the LP solution, use a best-fit pass to place the integer
  patterns, hand the residual demand to the FFD baseline, then compact
  (dissolve the least-filled bars into others, exact for 1D since a move only
  needs free length).
- **Safety**: any LP problem (non-optimal status, untrustworthy duals, duality
  gap) triggers a documented fallback to the FFD baseline instead of emitting a
  bad plan.

It comes with `cg_test.go`, which pins down real behaviour: 200 identical
1400 mm rails on 6 m bars must pack into the optimal 50 bars; a varied order
that FFD packs into 28 bars must reach the LP bound of 26; stock limits must
produce a correct unplaced report; output must be deterministic; and a 6 m bar
holding 4.8 m of posts must still report a reusable offcut.

**`portfolio.go` — `best-1d` (also not registered).**
Mirrors the 2D portfolio: wraps FFD + column generation, rank 1, keeps the best
score.

**`pinned-1d`** — added with the editing milestone. It keeps locked bar
placements at their exact X, fills the free intervals first-fit, and reports
interval remainders that meet the offcut policy. Like `pinned-2d`, it is only
reachable when a problem carries `Pinned` sheets.

Why "not registered": `optimizer.DefaultRegistry()` only calls `pack1d.New()`
(the FFD solver). Neither `cg-1d` nor `best-1d` is reachable from the API, and
`golden.json` has no entry for either. They are complete, tested building blocks
waiting to be wired in (the README roadmap still lists column generation as the
*next* solver). If you wire them in, register with the declared ranks
(`best-1d` = 1, `cg-1d` = 5) and re-run `bench -update` to extend the golden
file.

#### 6.2.6 `lp/` — a small simplex

`simplex.go` solves dense LPs in equality form `min c·x s.t. A x = b, x ≥ 0`
with a **two-phase simplex tableau**. Deliberately small and dependency-free:
it exists only for the column-generation master problem (few rows, growing
columns). It uses **Bland's rule** for entering/leaving variables — slower than
Dantzig's but cycle-free, which matters more than speed (a cycling solver would
hang a request). Duals are read from the objective row of the artificial
columns (`y_i = −r_i`), and `DualViolation` is the caller's safety net: a large
violation means the duals must not be trusted (cg-1d then bails out).

#### 6.2.7 `cutter/` and `validator/`

`cutter.ForSheet(sheet, kerf)` converts a layout into ordered cut instructions
by running `geom.BuildCutTree` over the whole sheet. The bool return *is* the
guillotine feasibility test — false means "no valid cut sequence".

`validator.Validate(problem, solution)` is the mandatory gate. It returns a
list of `Violation{severity, code, sheetIndex, partCode, message}`:

| Check | Code (error unless noted) |
|---|---|
| sheet has positive size | `bad_sheet_size` |
| piece has positive size | `bad_piece_size` |
| piece inside the sheet | `out_of_bounds` |
| piece matches the part (size, rotation permission, grain) | `bad_orientation` |
| unknown part | `unknown_part` (warning) |
| kerf separation against all later pieces | `overlap` |
| guillotine cut tree exists (when rules say guillotine) | `not_guillotine` |
| produced ≤ requested (+ overs percentage) | `demand_exceeded` |
| missing pieces are reported in `Unplaced` | `missing_unplaced_report` (warning) |
| each unplaced entry | `unplaced_parts` (warning) |

#### 6.2.8 `explain/` — no black boxes

`Notes(problem, solution)` turns metrics into plain language:

- sheets used, part area vs stock area, yield %;
- how many reusable offcuts stay in circulation and their total area;
- trim and kerf as machine losses that cannot be packed away;
- the **area lower bound** (`ceil(part area ÷ largest usable sheet)`) when the
  plan uses more sheets than the bound — "the gap is shape waste, not size
  waste";
- how many requested pieces could not be placed;
- pattern count (fewer patterns = fewer setups).

This is the "honest plan" principle: plans are never oversold.

#### 6.2.9 `bench/` — the quality gate

- `bench.go`: `Instance` (name + parts + stocks + rules), `LoadDir` (all
  `*.json` except `golden.json`, sorted), `DefaultBudgetMS = 500`, `Row`
  (per solver per instance: sheets, placed/requested, fill %, yield, waste,
  score, offcut/scrap m², ms, violations, failed), `Run` (skips solvers whose
  dimension profile doesn't match; note the +250 ms grace on the timeout),
  `Aggregate` (per-solver means), `Report` (tabwriter table).
- `generate.go`: deterministic random instances — same seed+index always gives
  the same problem (`DefaultSeed = 7`), with a distribution loosely modelled on
  shop reality (a few large pieces, a tail of medium ones, small fillers, 1–2
  stock formats).
- `golden.go`: the committed bar. `Golden` stores accepted mean score + waste +
  fill per solver; `Compare` fails on run failures, validation errors, a solver
  with no golden entry, a golden entry with no rows, or a mean score below
  golden − 0.5 points.
- `quality_test.go` (package `bench_test`): runs the real benchmark set in
  `go test`:
  - `TestSolversProduceValidPlans` — every 2D solver on every instance must
    produce zero validator errors;
  - `TestBeamBeatsShelfOnTheBenchmarkSet` — beam must beat shelf by ≥ 5 %
    relative waste (guards the core value proposition);
  - `TestPortfolioKeepsTheBestStrategy` — the portfolio's score may never be
    below any inner strategy's;
  - `TestGoldenGate` — same gate as `bench -check` (skipped by `go test -short`);
  - `TestGenerateIsDeterministic` — same seed ⇒ same instance.

Current committed numbers (`golden.json`, seed 7, 500 ms budget):

| Solver | Fill % | Waste % | Mean score |
|---|---|---|---|
| `polish-2d` / `best-2d` | 92.2 | **24.5** | **904.1** |
| `beam-2d` | 92.2 | 26.8 | 903.1 |
| `shelf-2d` (baseline) | 90.6 | 35.0 | 883.6 |
| `ffd-1d` | 72.0 | 2.3 | 707.9 |

#### 6.2.10 `export/` — plan files

Pure-Go writers over a `core.Solution`, used by `GET /plans/{id}/exports`:

- **CSV** (`csv.go`): the shop-floor cut list — one row per sheet, piece, cut
  step and offcut, in millimetres with stable column names, plus unplaced
  demand.
- **SVG** (`svg.go`): a review drawing with every sheet stacked vertically,
  pieces coloured by part code and offcuts in green.
- **DXF** (`dxf.go`): AutoCAD R12 ASCII in millimetres, sheets side by side,
  layers `SHEET` / `PARTS` / `OFFCUT` / `TEXT`.
- **PDF** (`pdf.go`): a minimal dependency-free PDF 1.4 writer — A4 landscape,
  one page per sheet, vector rectangles and Helvetica labels.
- `Write` dispatches by format; `Options.Sheet` selects one sheet.

#### 6.2.11 `costing/` — what the plan costs

`Breakdown(problem, solution)` values a plan: new material, remnants taken,
offcut credit, the part/trim/kerf/scrap shares of the stock value, net cost and
cost per part / per m². The optimizer attaches it to every `Result`, and plans
recompute it when they are read, so no cost is ever stored twice.

### 6.3 `internal/modules/` — HTTP feature layer

One folder per feature; each owns its DTOs, routes, and a **small store
interface** that `platform/postgres` implements. Modules never import each
other; shared helpers live in `platform/httpx`. A **nil store is legal** and
means "DB down" — handlers answer `503 database_unavailable` instead of
panicking.

**`jobs/`** (service.go + http.go)
- `Service` holds the registry; `Run` = `optimizer.Solve`.
- `DemoProblem()` and `DemoBarProblem()` are the built-in sample orders (also
  used by tests and the frontend fallback).
- Routes: `POST /optimize`, `GET /solvers`, `GET /demo/plan`,
  `GET /demo/bar-plan`.
- `optimize` decodes the body (8 MiB limit, unknown fields rejected), reads
  `?solver=` and `?dryRun=`, applies `core.Normalize`, runs with
  `budget + 5 s` timeout, and — unless dry run — archives via
  `Store.SaveRun`. Archiving failures only log a warning: the plan still
  returns. Response: `{ id?, result: { solution, violations, score } }`.
- `SaveRunRequest` = problem snapshot + solver + result; that is exactly what
  gets written to `cut_jobs` / `plans`.

**`catalog/`** (store.go)
- DTOs `Material` and `StockFormat`; `CreateMaterialInput`.
- Store interface: `ListMaterials`, `CreateMaterial`, `ListStockFormats`.
- Routes: `GET/POST /materials`, `GET /stock-formats`. Validation: code/name
  required, dimension profile defaults to `2d`.

**`parts/`** (store.go)
- DTO `Part` (finished sizes in µm, grain, allowRotate, priority);
  `CreatePartInput` with optional `materialSpecId`, `allowRotate` pointer
  (tri-state) and priority.
- Store interface: `ListParts`, `CreatePart`.
- Handler defaults: grain `none`, priority `100`.
- Note the domain comment: cut sizes are derived from routings/allowances —
  never typed twice.

**`stock/`** (store.go)
- One physical piece per row: full sheet/bar or labelled remnant, status
  `available | reserved | consumed | retired`, location, prorated cost, lineage
  (`parentPlanId`, `parentSheetIndex`).
- Routes: `GET/POST /stock-items`, `GET/PATCH /stock-items/{id}`. Creating with
  a `formatId` inherits code, dimensions and cost; `consumed` is set by plan
  acceptance, never through the API.

**`plans/`** (store.go + http.go + edit.go)
- `GET /plans` lists summaries; `GET /plans/{id}` rebuilds the stored layout in
  the same `OptimizeResult` shape as a fresh solve, with per-placement `id` and
  `locked` values plus the plan rules.
- `POST /plans/{id}/accept` is the lifecycle step: one transaction consumes the
  physical pieces used, decrements `stock_formats.on_hand_qty` (never below
  zero), registers every reusable offcut as a labelled remnant (`OFF-…`, prorated
  cost, plan/sheet lineage), marks the plan `accepted` and writes an
  `audit_log` entry. A second accept is a `409`.
- `POST /plans/{id}/edit` applies move/rotate/lock/delete operations
  (`edit.go`), re-validates the layout (validator + edge trim) and stores it as
  version `n+1` (`parent_plan_id`), archiving the source. Invalid layouts get
  `422 edit_invalid` with the violation list.
- `POST /plans/{id}/reoptimize` turns locked placements into
  `core.PinnedSheet`s, removes the stock copies they occupy and runs a
  pinned-capable solver; the result is stored as a new version.
- `GET /plans/{id}/exports?format=csv|svg|dxf|pdf` streams the plan through
  `internal/optimizer/export`.

**`campaigns/`** (store.go + budget.go + http.go)
- A campaign is an ordered set of items plus one shared stock budget
  (`campaigns.stock` JSONB, `initial_stock` kept for reference).
- `BuildItemProblem` (http.go) turns the next pending item into a problem: its
  parts, the remaining budget, the campaign rules/objective and
  `seed + item.seq` for reproducible runs.
- `budget.go` is pure and unit-tested: `ConsumeBudget` removes one unit per
  sheet used (a physical remnant leaves the budget), re-adds every offcut that
  meets the policy as a `CMP-…` remnant with prorated cost, and `RemnantLabel`
  names them uniquely.
- `POST /campaigns/{id}/run-next` solves the earliest pending item, stores job
  + draft plan + item status + updated budget in one transaction under a
  campaign row lock (`CompleteItem`), and derives the campaign status: `active`
  after the first run, `completed` when nothing is pending.
- The budget is a planning sandbox: accepting a plan is still what changes the
  plant's real stock. Only `draft`/`active` campaigns accept items or runs.

### 6.4 `internal/platform/` — infrastructure

| Package | What it does |
|---|---|
| `config` | `Load()` from env: `CUTOPTICS_ADDR` (:8080), `DATABASE_URL` (default postgres on 5433), `CUTOPTICS_ENV` (dev), `CUTOPTICS_CORS_ORIGINS` (default `http://localhost:5173`). `Version` is ldflags-overridable. |
| `httpserver` | `New(Deps) http.Handler`: chi router with RequestID, RealIP, Logger, Recoverer, 10-min timeout, CORS; mounts `/healthz`, `/api/v1/healthz`, `/api/v1/meta` and the three module route groups. Health reports `{status, db, version, env, time}`; `/healthz` returns 503 when the DB is down. `Deps` carries everything (no globals). |
| `httpx` | `JSON`, `Error` (uniform `{error:{code,message}}` payload), `DecodeJSON` (size limit, `DisallowUnknownFields`, exactly one JSON object). |
| `id` | `New() string` — a UUID helper. Currently unused (a small piece of future-proofing). |
| `postgres` | `Connect` (pool: max 10 conns, 1 h lifetime, 5 s connect timeout, ping-verified), `Healthy` (2 s ping for /healthz), `NewStore`, and `Store` implementing all three module store interfaces. |
| `db` | **generated** sqlc code (pgx v5). Never edit by hand. |

`postgres.Store` highlights:

- every read maps `db` rows to module DTOs (UUIDs to strings, µm int64s kept
  as-is);
- `CreateMaterial` / `CreatePart` / `SaveRun` need the **default plant**
  (`GetDefaultPlant`); with a fresh DB they return "no plant configured; run
  db/scripts/seed.ps1";
- `SaveRun` is the big one: one transaction writes `cut_jobs`
  (created → running → done), `plans` (version 1, status `draft`, rules and
  metrics as JSONB), one `plan_sheets` row per sheet (offcuts and cut steps as
  JSONB), one `placements` row per placement, then marks the job done with the
  full result JSON. Any failure rolls the whole thing back — a partial plan can
  never land in the database.

---

## 7. The database and the `db/` workspace

`db/` is self-contained: schema, queries and the PowerShell commands to work
with them.

### 7.1 Schema — 19 tables (`migrations/0001` … `0005_campaigns.sql`)

| Table | Purpose | Notes |
|---|---|---|
| `plants` | one row per site | everything hangs off it |
| `users` | local users | roles: admin, planner, operator, viewer (auth not built yet) |
| `materials` | families (GLASS, ALU, MDF) | `dimension_profile` 1d/2d/3d, JSONB attributes |
| `material_specs` | concrete variant | thickness µm, finish, colour |
| `machines` | saws, cutters, CNCs | kind + JSONB attributes |
| `rules_profiles` | **the only place vertical specifics live** | rules JSONB, `is_default` |
| `stock_formats` | catalogue sizes | length OR width×height (CHECK enforces one), on-hand qty, cost NUMERIC(14,4) |
| `parts` | finished sizes | grain, allow_rotate, priority |
| `part_routings` | operations + allowances | cut size = finished + Σ allowances |
| `assemblies` | products built from subparts | overall size, kind (window/door/generic) |
| `assembly_components` | ordered subparts | role/kind, dimensions and local-frame offsets |
| `cut_jobs` | job queue + archive | status queued/running/done/failed/cancelled, input/result JSONB, seed, budget |
| `plans` | a plan per job (versioned) | status draft/approved/accepted/archived, parent_plan_id, accepted_at, rules/metrics/notes JSONB |
| `plan_sheets` | one row per stock sheet | dimensions, offcuts JSONB, cut_steps JSONB, stock_id/stock_item_id |
| `placements` | one row per placed part | x/y/w/h µm, rotated, locked, seq |
| `audit_log` | mutation log | action, entity, payload JSONB |
| `stock_items` | physical pieces and labelled remnants | status, location, cost basis, plan/sheet lineage, partial unique label index |
| `campaigns` | a batch of jobs sharing a stock budget | status, rules/objective, budget JSONB, initial_stock |
| `campaign_items` | ordered jobs inside a campaign | parts JSONB, due date, status, plan/job links |

Unit rule enforced everywhere: all lengths are `BIGINT` micrometers. Flexible
details (attributes, rules, metrics, offcuts, cut steps) live in JSONB while
indexed core fields stay relational. `0002_remnants.sql` adds
`plan_sheets.stock_id` / `stock_item_id` (which piece a sheet was cut from) and
`plans.accepted_at`; `0003_plan_edits.sql` adds `placements.locked` and
`plans.parent_plan_id`; `0004_assemblies.sql` adds the product tables;
`0005_campaigns.sql` adds campaigns and their items.

### 7.2 Queries and code generation

`queries/*.sql` holds **~50 named queries** (`-- name: X :one/:many/:exec`):
jobs (create/get/list/run/done/fail/cancel), materials + specs, parts +
routings, assemblies + components, plans/sheets/placements (create, list,
accept, archive, version), campaigns (create/get/update/stock/items/complete),
stock formats (incl. `TakeStockOnHand`), stock items (list/get/create/update/
consume, available remnants), kpis (aggregate + series), audit insert.
`sqlc.yaml` points generation at
`../backend/internal/platform/db` with pgx/v5, UUID → `google/uuid`,
numeric → `float64`.

Workflow for any DB change: edit `queries/*.sql` → `db/scripts/generate.ps1` →
use the generated function from `postgres.Store`. Never hand-edit
`backend/internal/platform/db/*`.

### 7.3 Seeds, tests, scripts

- `seeds/0001_demo_data.sql` — idempotent demo data: DEMO plant; GLASS (2D) and
  ALU (1D) materials; glass/alu specs; sheets 3210×2250 and 2440×1220 and bar
  BAR-6000 with quantities and costs; the three demo parts; a routing that
  grinds 2 mm per edge off the window panes; a glass rules profile; machine.
- `seeds/0002_demo_remnants.sql` — a small pool of labelled demo remnants
  (`OFF-DEMO-01`, `OFF-DEMO-02`, `OFF-DEMO-BAR`) with lineage to the formats, so
  remnant-first allocation can be tried without accepting a plan first.
- `tests/smoke.sql` — table tests with `ON_ERROR_STOP`, DO blocks and
  rolled-back writes.
- `scripts/` — `up.ps1` / `down.ps1`, `migrate.ps1` (goose, `-Status -Down -To`),
  `new-migration.ps1`, `seed.ps1`, `psql.ps1`, `test.ps1`, `generate.ps1`,
  `reset.ps1`. `common.ps1` is a dot-sourced helper (compose discovery, DB URL).
- Ports: `db/.env` `DATABASE_URL` must match `deploy/compose/.env`
  `POSTGRES_PORT` (default **5433** so a native Postgres on 5432 stays yours).

### 7.4 Local infrastructure (`deploy/`)

`deploy/compose/docker-compose.yml` (project `size-module`) runs only:
- `postgres:16-alpine` → host 5433, volume `pgdata`, `pg_isready` healthcheck;
- `adminer:4` → host 8081 (DB browsing UI).

There is intentionally no API/frontend Dockerfile yet — the Go and React apps
run on the host.

---

## 8. Frontend, file by file

Stack: React 19 + TypeScript + Vite 8 + Tailwind v4 + shadcn/ui primitives
(radix-ui) + Redux Toolkit + three.js (`@react-three/fiber`, `drei`,
lazy-loaded). The dev server (port 5173) proxies `/api` and `/healthz` to the
Go API on 8080, so the frontend uses **relative paths only** and behaves the
same in production behind a reverse proxy.

### 8.1 Entry and routing

- `src/main.tsx` — `createRoot` → Redux `<Provider>` → `<BrowserRouter>` →
  `<TooltipProvider>` → `App`.
- `src/App.tsx` — all routes nested under `AppShell`: index → Dashboard,
  `viewer`, `materials`, `parts`, `stock`, `jobs`, `settings`, `*` → redirect
  to `/`.
- `src/components/layout/AppShell.tsx` — sidebar navigation (Dashboard, Plan
  viewer, Materials, Parts, Stock, Jobs, Settings), header with the API status
  badge, and `<Outlet/>`. On mount it dispatches `fetchBackend()` once. The
  badge shows "checking API…", "API offline", "API + DB online", or "API
  online · DB offline" — the whole graceful-degradation story in one component.

### 8.2 Redux store and slices (`src/app/`)

- `store.ts` — `configureStore` with four reducers: `backend`, `catalog`,
  `optimizer`, `viewer`. Exports `RootState` and `AppDispatch`.
- `hooks.ts` — typed `useAppDispatch` / `useAppSelector`.
- `features/backend/backendSlice.ts` — `fetchBackend` thunk fetches `/healthz`
  and `/api/v1/meta` in parallel; state `{health, meta, status, error}`.
- `features/catalog/catalogSlice.ts` — materials, parts and stock formats:
  `fetchMaterials` / `createMaterial`, `fetchParts` / `createPart`,
  `fetchStockFormats`, each with its own status/error, so the catalogue pages
  share one store instead of local `useState`.
- `features/optimizer/optimizerSlice.ts` — the heart of the demo flows:
  - `loadDemoPlan` / `loadBarDemoPlan`: GET `/api/v1/demo/plan` or
    `/api/v1/demo/bar-plan`; **on any failure returns the built-in sample**
    (`sampleResult()` / `sampleBarResult()`) so the viewer always renders;
  - `runDemoOptimization({profile, cutMode, solver, includeRemnants})`: POST the
    matching demo problem to `/api/v1/optimize` (adding `?solver=` unless "auto"
    and `?includeRemnants=true`; free-cut requests send the full default rules
    with `cutMode: free`); an archived run also stores `lastJobId`/`lastPlanId`;
  - `compareSolvers({profile, cutMode, solvers, includeRemnants})`: runs each
    **compatible** solver on the same problem with `?dryRun=true` (not archived);
  - `startAsyncJob(...)` / `cancelAsyncJob(id)`: queue a
    job on `POST /api/v1/jobs`, follow `GET /api/v1/jobs/{id}/events` over SSE,
    track `jobRequest`/`jobState`/`jobProgress`/`jobError`, remember the plan id
    from the job view and load the archived result into the viewer when it
    finishes;
  - `acceptPlan(planId)`: `POST /api/v1/plans/{id}/accept` — consumes used
    pieces, decrements on-hand quantities and registers labelled remnants; state
    tracks `acceptStatus` / `acceptResult` / `acceptError`;
  - `editPlan({planId, operations})` / `reoptimizePlan({planId, operations, solver})`:
    POST the next plan version; the fulfilled reducer swaps in the new result
    and plan id, and `planEditStatus` / `planEditError` drive the editor
    toolbar;
  - state: result + dimension + source (`api`/`sample`/`none`), run status,
    comparison status, async job state, plan acceptance and editing status.
- `features/kpis/kpiSlice.ts` — the realized-yield report (`GET /api/v1/kpis`)
  fetched with the chosen window in days; the dashboard renders its buckets and
  trend bars.
- `features/campaigns/campaignSlice.ts` — campaign list + detail plus the
  create/add-item/remove/run-next mutations; a fulfilled mutation stores the
  returned detail, so the budget and item list always reflect the server.
- `features/viewer/viewerSlice.ts` — UI state: mode (`2d`/`3d`, default
  **3d**), selected sheet, selected part, show offcuts, explode amount, plus the
  edit draft (`editing`, `draftSheets`, `pendingOps`).

### 8.3 Library (`src/lib/`)

- `api.ts` — thin `fetch` wrapper: `api.get` / `api.post`, relative paths.
  Non-OK responses are parsed into `ApiError{status, code, message}` using the
  backend's uniform error payload; network failures become
  `ApiError(0, 'network_error', ...)`.
- `types.ts` — hand-written TypeScript mirrors of the Go contracts (`Rect`,
  `Placement`, `SheetPlan`, `UnplacedPart`, `Metrics`, `Solution`, `Violation`,
  `OptimizeResult/Response`, `Health`, `SolverInfo` with `Capabilities.Rank`,
  `Meta`, `Problem`/`Rules`, `Material`, `StockFormat`, `Part`, plus the async
  `JobSubmitResponse`/`JobView`/`JobEvent` types). Must be kept in sync with
  `backend/api/openapi.yaml` manually (codegen is planned but not wired).
- `solvers.ts` — mirrors the backend registry: `cutModeCompatible` (guillotine
  solver ≤ free request), `solversFor(profile, cutMode)` ordered by rank, so the
  UI never offers a solver the API would reject with a 422.
- `results.ts` — `normalizeResult()` fills the arrays Go marshals as `null`
  (`unplaced`, `offcuts`, `sheets`); the optimizer slice applies it to every
  result entering the store, so the viewer never crashes on a null slice.
- `format.ts` — the units boundary: µm → mm (`micronToMm`), µm → m, m², %,
  and `mmToMicron`. UI components convert for display only.
- `samplePlan.ts` — hand-built fallback plans (2D `shelf-2d`-style sheets and a
  1D FFD-style bar layout) that keep the viewer useful when the API is down.
- `utils.ts` — `cn` class-merge helper re-exported from the `cn` package.

### 8.4 The plan viewer (`src/features/viewer/`)

- `PlanViewer.tsx` — the page body: if no result, shows "No plan loaded"; else
  a two-column layout. Left: for 2D plans the 3D/2D canvas inside `Suspense`,
  toggles for 2D/3D, offcuts on/off and (in 3D) the explode slider; for 1D
  plans (detected via `optimizer.dimension`) the bar tracks of `BarPlanView`.
  When the current result has an archived plan id the card also carries a
  toolbar: **Edit layout** (2D), **Save edits**, **Re-solve with locks**,
  **Cancel** and CSV/SVG/DXF/PDF export links. Right: a stack of cards —
  **Scorecard** (yield, waste, offcut/scrap/kerf/trim areas for sheets or
  stock/used/offcut lengths for bars, pattern count, cost, solve time, score),
  **Sheets/Bars** list (click to inspect), **Selected piece** details (plus
  Rotate / Lock / Delete in edit mode), **Cut sequence** (the ordered cut steps
  for the active sheet), **Why this plan** (the solver's notes), **Unplaced
  demand** (part, quantity, reason) and **Validation** (violations with severity
  badges).
- `BarPlanView.tsx` — the 1D renderer: every bar is a horizontal track with
  coloured part segments (deterministic `partColor`), green offcut ranges,
  click-to-select, and millimetre end labels. Bars come back as `SheetPlan`s
  whose width is the bar length, so the three.js sheet scene would render them
  as slivers.
- `PlanScene.tsx` — the three.js scene (loaded with `React.lazy` so three.js
  stays out of the initial bundle). World units are **metres** (`UM = 1e-6`),
  sheet thickness is exaggerated (0.016) for visibility.
  - 2D mode: one sheet (the selected one), orthographic camera fitted to it via
    `FitOrthographic`, rotation locked so it behaves like a drawing.
  - 3D mode: all sheets stacked with a gap, perspective camera, orbit
    controls, grid; the explode slider spreads the stack (z computed per sheet
    index).
  - Edit mode (2D only): pieces are draggable — `PartMesh` captures the
    pointer, previews the move locally, snaps to millimetres on release and
    clamps to the sheet; orbit controls are disabled while dragging. Locked
    pieces glow emerald. Clicks select the placement key (`id` when the plan is
    archived, part id for sample data).
  - Each sheet: a base plate mesh, one box per placement (position converted
    from top-left-origin layout coordinates to a centre-origin scene), offcuts
    as translucent green plates, and an `Html` label.
  - Parts are clickable and hoverable (cursor changes, emissive highlight);
    clicking selects/deselects the part, which the right-hand panel reflects.
- `partColor.ts` — deterministic FNV-style hash of the part code → HSL colour,
  so the same part always gets the same colour; priority-1 parts get higher
  saturation.

### 8.5 Pages (`src/pages/`)

- `DashboardPage.tsx` — the KPI band (realized yield/waste, stock value, cost
  per part, remnants used, parts produced, offcuts kept, pipeline plans and
  value, plus a yield trend bar per accepted plan), then three cards (System
  status from health/meta, Solvers list from meta with capability badges and
  descriptions plus a 1D bar demo loader, Latest result with a link to the
  viewer) plus buttons: Refresh, Load demo plan, Run demo optimization. Running
  navigates to `/viewer` on success.
- `PlanViewerPage.tsx` — hosts `PlanViewer`; auto-loads the 2D demo plan on
  mount when no result exists; offers "Load 2D demo" and "Load 1D bar demo",
  and, when the current result has an archived plan id, an **Accept plan**
  button whose result card lists the created remnant labels. Accepting also
  refreshes the available stock pool.
- `CampaignsPage.tsx` — the batch list plus a "New campaign" form: name, solve
  budget, a switch that pulls in the plant's available remnants and a quantity
  per stock format for the budget.
- `CampaignDetailPage.tsx` — one campaign: progress, **Run next item**,
  cancel, the remaining-budget table (CMP-… pieces marked as remnants), the
  ordered item list with due dates and an **Open plan** action per planned item,
  and an "Add item" form that builds parts from the catalog with quantities.
- `JobsPage.tsx` — the solver playground: pick a dimension profile (2D sheets /
  1D bars) and cut mode (guillotine / free cutting), then Run, compare every
  compatible solver (dry runs, table with a "best" badge), or Queue job to the
  async pipeline. The queue card shows the live SSE progress bar (sheets,
  pieces, yield, elapsed), supports cancel, and loads the archived result into
  the viewer when done. Runs explain that the synchronous path works without a
  DB while the queue needs PostgreSQL.
- `MaterialsPage.tsx` — materials catalogue (`GET /api/v1/materials`) plus an
  "Add material" form (`POST /api/v1/materials`) with a dimension-profile
  selector.
- `StockPage.tsx` — two tabs. **Formats**: the catalog sizes (`GET
  /api/v1/stock-formats`). **Pieces & remnants**: the physical pool (`GET
  /api/v1/stock-items`, status filter), with a Retire action (`PATCH`) and a
  "Register a piece" form that creates a labelled remnant manually, inheriting
  code/size/cost from a chosen format.
- `PartsPage.tsx` / `StockPage.tsx` — catalogue pages backed by the `catalog`
  slice: parts list + create form (`GET/POST /api/v1/parts`, finished sizes in
  mm converted with `mmToMicron`) and the stock surfaces above, each with a
  friendly "Database not reachable" card that tells you which scripts to run.
- `SettingsPage.tsx` — service info, the units/precision explanation, a
  `db/scripts` cheat sheet, and a table of registered solvers with their
  dimension, cut mode, rank and capability flags.

### 8.6 UI primitives and styling

`src/components/ui/` contains the shadcn/ui primitives used across the app
(button, card, table, select, tabs, tooltip, badge, progress, slider-ish
toggle group, etc.). Tailwind v4 is wired through `@tailwindcss/vite` in
`vite.config.ts`; `components.json` configures the shadcn generator (note: its
`hooks` alias points at `@/hooks`, which does not exist yet).

---

## 9. End-to-end walkthroughs

### 9.1 "Run demo optimization" click path (with DB)

1. Dashboard/Jobs buttons dispatch `runDemoOptimization` → POST
   `/api/v1/optimize`.
2. Vite proxies to Go. `jobs.Handler.optimize` decodes the problem, applies
   `Normalize` (default rules: 4 mm kerf, 10 mm trim, guillotine, offcuts
   ≥ 300 mm; weights; budget 5000 ms).
3. `optimizer.Solve` asks the registry (no `?solver=`) → `ForProblem` detects
   **2D** → candidates sorted by rank → **`best-2d`** (rank 1) wins.
4. `best-2d` splits the budget across shelf/beam/polish, each of which returns
   a plan; the portfolio keeps the best `core.Score` and notes which strategies
   were compared. Ordering is deterministic, so the same order gives the same
   plan.
5. `validator.Validate` checks the winning plan; `explain.Notes` adds the
   plain-language reasons.
6. Not a dry run → `postgres.Store.SaveRun` writes job + plan + sheets +
   placements in one transaction; the response carries the job id.
7. The slice stores `result` and `lastJobId`; the UI navigates to `/viewer`,
   where `PlanViewer` renders the scene and panels. The scorecard shows the
   waste split; "Why this plan" shows the notes; "Cut sequence" shows the
   guillotine steps.

### 9.2 Same flow with the API down

`loadDemoPlan` catches the fetch error and returns `sampleResult()` — a
hand-built plan mirroring shelf packing of the demo problem. The viewer renders
normally, labelled "sample data" instead of "from API".

### 9.3 CLI smoke tests (no infrastructure)

```
cd backend
go run ./cmd/cutoptics -demo        # 2D demo plan as JSON
go run ./cmd/cutoptics -demo-bar    # 1D demo plan as JSON
go test ./...                       # solver tests + golden gate
go run ./cmd/cutoptics bench -dir testdata/benchmarks -random 6 -check
```

### 9.4 Deliberate solver comparison

Jobs page → "Compare 2D solvers" → for each 2D solver a POST with
`?solver=X&dryRun=true` → results are **not archived** → table with a best
badge. This is how you show, in the UI, that beam beats shelf and polish/portfolio
beats beam on the same problem.

### 9.5 Remnant lifecycle (plan → labels → pool → next plan)

1. In the Jobs page the **Use available remnants** toggle adds
   `includeRemnants=true`; the server loads the plant's `available` remnants
   (optionally one material spec), keeps the ones matching the problem's
   dimension profile and appends them to the problem snapshot.
2. `core.Normalize` puts them first in the stock list (`preferRemnants`) and
   `core.Score` does not charge a remnant sheet as a new sheet. The plan notes
   say either how many sheets came from remnants or that the available remnants
   did not improve the plan.
3. A sheet's label shows the remnant label (e.g. `OFF-DEMO-01`) in the viewer's
   sheet list; the scorecard shows "From remnants" when `remnantSheets > 0`.
4. **Accept plan** (viewer) freezes the plan and calls
   `POST /api/v1/plans/{id}/accept`: used pieces go to `consumed`, catalog
   on-hand drops (never below zero), every offcut that respects the stored
   offcut policy becomes an `OFF-…` remnant with prorated cost and lineage, and
   an audit row is written — all in one transaction.
5. The result card lists the new labels; Stock → Pieces & remnants shows them as
   available, and the next run can pick them up. Accepting again returns `409`.

### 9.6 Edit a plan, lock it, re-solve, export

1. Run an optimization (the sync `/optimize` or a queued job both archive a
   plan) and open the viewer. The toolbar appears because the result carries a
   plan id.
2. **Edit layout** switches to 2D and starts a draft: drag pieces (snapped to
   1 mm, clamped to the sheet), pick a piece and Rotate / Lock / Delete. Locked
   pieces glow emerald; nothing is stored until **Save edits**.
3. **Save edits** posts the pending operations to `POST /plans/{id}/edit`. The
   server applies them, re-checks bounds/trim/kerf/guillotine/demand and stores
   version `n+1`; the source plan becomes `archived`. An impossible drag is
   rejected with the violation text (overlap, trim, …) and the draft stays.
4. **Re-solve with locks** posts the same operations plus a solve to
   `POST /plans/{id}/reoptimize`. Locked placements become pinned sheets, the
   stock copies they occupy are removed from the free pool, and `pinned-2d`
   (or `pinned-1d`) fills the free regions with everything else. The notes say
   how many locked placements stayed put.
5. Exports are plain downloads: CSV (`sheet,label,stock,kind,seq,part,…` with
   cut steps and offcuts), SVG, DXF R12 and PDF — one click each in the same
   toolbar, or `GET /plans/{id}/exports?format=csv&sheet=2`.

### 9.7 Costing a result and reading the KPIs

1. Every `OptimizeResult` (sync run, queued job, plan read) carries `cost`:
   new material, remnants taken, offcut credit, net cost, cost per part and per
   m². The viewer's Scorecard shows the breakdown under the metrics.
2. Accept a plan and the dashboard's **Realized yield** band updates on the next
   refresh: accepted plans' yield/waste, stock value, remnants used, parts
   produced, offcuts kept. The window selector fetches
   `GET /api/v1/kpis?days=30|90|365`.
3. The trend bars are the accepted plans oldest-to-newest: bar height is yield,
   opacity rises with waste, and hovering shows the date and sizes. `created`
   numbers show the pipeline (all plans, not just accepted ones) so the gap
   between planned and committed work is visible.

### 9.8 Batching a week of orders (campaign)

1. Campaigns page → pick a stock budget (format quantities and/or "use
   available remnants"), name it and create it.
2. On the campaign page, add the jobs in order: catalog parts with quantities
   and an optional due date.
3. **Run next item** solves the earliest pending item against what is left:
   the used sheets leave the budget and the plan's offcuts re-enter it as
   `CMP-…` remnants with prorated cost. The result card links straight to the
   plan in the viewer, where it can be edited, re-solved or accepted.
4. When the last item is planned the campaign flips to `completed`; further
   runs answer `409`. The budget table is the honest picture of what the batch
   consumed — accepting the individual plans is what changes plant stock.

---

## 10. Quality gates: how we know a plan is good and valid

Three independent layers:

1. **Runtime correctness** — `validator.Validate` on every solve (overlap,
   bounds, kerf, grain, guillotine, demand). The API never returns a solution
   without having validated it; `violations` travel with the response (errors
   mean the plan must not be trusted; warnings are informational).
2. **Benchmark quality gate** — `golden.json` + `bench -check` / `TestGoldenGate`
   block any solver change that regresses the mean objective score by more than
   0.5 points on seed-7 instances at a 500 ms budget. Score (not waste alone)
   is the gate because demand fulfilment is the first objective.
3. **Invariant tests** — unit/property tests per package (`geom`, `pack2d`
   including pinned re-solve, `pack1d` including `cg_test.go`, `lp`,
   `core/registry_test.go`, `export`), a postgres integration test that walks
   edit → lock → re-solve → accept, plus cross-solver quality tests.
   `go test ./...` runs all of it; `go test -short ./...` skips the slow golden
   gate while iterating.

Why a *mean score* and not "best waste"? Because a plan that packs tighter but
places fewer parts is worse by the stated objective — the report always shows
fill %, yield %, waste % and score side by side so nothing is hidden.

---

## 11. What works, what doesn't (yet), and known quirks

Wired and working today: 2D shelf/beam/polish/column-generation/MaxRects
solvers plus the `best-2d` portfolio; 1D FFD, column generation and `best-1d`;
pinned re-solve solvers (`pinned-2d`, `pinned-1d`); guillotine cut-tree proof;
validator; explainer; cost breakdowns; benchmark harness + golden gate; REST API
(health/meta/solvers/optimize/demo/materials/stock-formats/parts/assemblies/
stock-items/plans/campaigns/kpis); the async job pipeline (`POST /jobs`, worker
pool, `GET /jobs/{id}`, cancel, SSE progress); the remnant lifecycle (physical
pieces, labels, `includeRemnants`, plan acceptance with stock decrement + audit);
plan editing with locked-placements re-solve and CSV/SVG/DXF/PDF exports;
realized-yield KPI aggregation with a dashboard band; campaign planning (shared
stock budget, ordered `run-next`, CMP-… offcut remnants); product/assembly
catalog with a 3D scene; PostgreSQL schema + seeds + sqlc generation; run
archiving in one transaction; React SPA with dashboard, viewer (2D/3D and 1D
bars, drag editor), solver comparison, async queue with live progress,
materials, parts, products, stock pool (formats + remnants), jobs, campaigns,
settings; graceful degradation at every layer.

Not implemented yet (from README + plan.txt, confirmed in code):
- auth/sessions/RBAC; CSV imports; machine-control formats; defect maps;
- multi-plant UI, irregular nesting, 3D bin packing;
- OpenAPI-driven codegen (both sides are hand-maintained for now).

Known quirks and drift (worth knowing before you triage a bug):
- **`plan-v1.txt` is historical.** It describes TanStack Query + Zustand +
  oapi-codegen; the code actually uses Redux Toolkit and hand-written types.
  The README/AGENTS files acknowledge this drift.
- **The synchronous `POST /optimize` still exists** for interactive use (plan
  viewer, solver comparison) and can block up to the budget (handler timeout =
  budget + 5 s; router timeout 10 min). Long jobs belong on the queue.
- **`platform/id` is unused** (UUID helper kept for later).
- **`MaxCutStages` in rules is declared but not enforced** by the solvers today.
- **1D `Height` trick**: bars get a nominal 1 mm height so area-based metrics
  don't divide by zero; use `stockLengthM`/`usedLengthM` for 1D reporting.
- **Beam width 24 is empirical.** `docs/solver.md` records the measurement:
  width 14 wastes 26.8 % on the benchmark set, width 24 also 26.8 % (the shipped
  default), width 32 slightly worse at 28.3 % — beam search is not monotonic in
  width, because wider beams keep states that fill worse later.
- **Remnant-first is a policy, not a hard rule.** Remnants are ordered first for
  constructive solvers and take no sheet penalty, but the portfolio still keeps
  the plan with the best objective score — a small remnant that would only add
  scrap stays in the pool, and the notes say so.
- The frontend's `lib/types.ts` and `backend/api/openapi.yaml` must be kept in
  sync manually — a classic source of small bugs.

---

## 12. Cheat sheet and reading order for new contributors

### Commands

Backend (`backend/`):
```powershell
go run ./cmd/cutoptics                 # API on :8080 (works without PostgreSQL)
go run ./cmd/cutoptics -demo           # 2D demo plan as JSON
go run ./cmd/cutoptics -demo-bar       # 1D demo plan as JSON
go test ./...                          # all tests incl. golden gate
go test -short ./...                   # skip the slow gate
go run ./cmd/cutoptics bench -dir testdata/benchmarks -random 6 -check
go run ./cmd/cutoptics bench -update   # accept current quality into golden.json
```
Frontend (`frontend/`):
```powershell
pnpm install
pnpm dev        # :5173, proxies /api and /healthz to :8080
pnpm build      # tsc -b && vite build
pnpm lint       # oxlint
```
Database (`db/scripts/`):
```powershell
up.ps1 → migrate.ps1 → seed.ps1 → test.ps1
generate.ps1    # after editing db/queries/*.sql
reset.ps1       # drop, migrate, seed
new-migration.ps1 -Name add_x
```
Knowledge graph (project venv): `.\graphify.ps1 query "<q>"` · `path "A" "B"` ·
`explain "Concept"` · `update .`.

### Ports and env

| Thing | Default |
|---|---|
| Go API | `:8080` (`CUTOPTICS_ADDR`) |
| Vite dev server | `:5173` |
| PostgreSQL | host `5433` (`DATABASE_URL` in `db/.env`; `POSTGRES_PORT` in compose) |
| Adminer | `:8081` |

### Where to read first, depending on your goal

| Goal | Read |
|---|---|
| Understand the domain language | `docs/glossary.md`, then `core/types.go` |
| Understand the solver engine | `internal/optimizer/AGENTS.md`, then `pack2d/beam.go`, `improve.go`, `validator/validator.go`, `docs/solver.md` |
| Touch the API | `backend/AGENTS.md`, `modules/jobs/http.go`, `api/openapi.yaml` |
| Touch the DB | `db/AGENTS.md`, `migrations/0001_init.sql`, `queries/*.sql`, then regenerate |
| Touch the UI | `frontend/AGENTS.md`, `App.tsx`, `optimizerSlice.ts`, `PlanViewer.tsx`, `PlanScene.tsx` |
| Change solver quality | `bench/quality_test.go` + `golden.json` + `docs/solver.md` (the benchmark section is the contract) |

### The three rules that prevent 90 % of mistakes

1. **Units**: every length anywhere is integer **micrometers**. Convert only in
   the UI (`lib/format.ts`).
2. **The seam**: new algorithms implement `core.Solver` and get registered —
   never add branching for a specific solver in callers.
3. **The gate**: every solution is validated (including guillotine cut-tree
   feasibility) before it is shown or stored, and solver quality changes must
   pass the golden benchmark gate.
