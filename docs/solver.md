# Solvers and the benchmark gate

This document explains how the optimization engine is built, what each solver
does, and how quality is measured. If you change a solver, read the benchmark
section first — that is the contract.

## The seam

Everything plugs into one interface (`internal/optimizer/core/solver.go`):

```go
type Solver interface {
    Name() string
    Version() string
    Capabilities() Capabilities
    Solve(ctx context.Context, p Problem, progress ProgressFunc) (Solution, error)
}
```

`Problem` and `Solution` are plain structs with all lengths as integer
micrometers. Solvers live in `internal/optimizer/` and never touch the database,
HTTP or the clock beyond the time budget. That keeps them testable, portable
(WASM later) and replaceable.

`Capabilities.Rank` decides which solver the job layer picks when the caller
does not name one: lower rank wins, ties break by name. The portfolio ranks
first, so new strategies can be added without changing callers.

Every solution is validated before it is shown (`internal/optimizer/validator`):
bounds, kerf, overlap, grain, demand limits and **guillotine cut-tree
feasibility**. A layout that looks fine but has no edge-to-edge cut sequence is
rejected, never displayed or stored.

## Shipped solvers

| Name | Profile | Approach | Rank |
|---|---|---|---|
| `best-2d` | 2D | Portfolio: runs the strategies below, keeps the best objective score | 1 |
| `best-1d` | 1D | Portfolio: FFD baseline plus column generation | 1 |
| `polish-2d` | 2D | Beam search plus local search (dissolve, merge, repack sheets) | 3 |
| `cg-1d` | 1D | Gilmore–Gomory column generation with DP pricing | 5 |
| `cg-2d` | 2D | Two-stage guillotine column generation (strip pricing + height DP) | 5 |
| `beam-2d` | 2D | Beam search over guillotine cut trees | 10 |
| `maxrects-2d` | 2D | MaxRects free-rectangle packing for `cutMode: free` (CNC, laser, waterjet) | 10 |
| `shelf-2d` | 2D | Shelf/strip packer (baseline every candidate is compared against) | 20 |
| `ffd-1d` | 1D | First-Fit-Decreasing for bars, profiles and tubes | 10 |

The job layer picks by dimension profile **and** cut mode (`Registry.ForProblem`):
a guillotine problem is never handed to a free-cutting solver, while a
free-cutting problem may use a guillotine solver — that layout is always valid.

### `cg-1d` (column generation)

For bars, profiles and tubes the classical Gilmore–Gomory approach beats
constructive heuristics, and it is the only solver here that *proves* how good
its answer is:

1. The **restricted master LP** minimises the cost of the patterns used, subject
   to meeting demand. It has one row per part length and one column per pattern,
   so it is solved with a small dense two-phase simplex
   (`internal/optimizer/lp`, Bland's rule, duals read from the artificial
   columns, with dual feasibility and strong duality checked on every solve).
2. **Pricing** finds new patterns with an exact DP knapsack over the current
   dual values: items are `(length + kerf)` rounded **up**, capacity is rounded
   **down**, so every pattern the DP proposes is feasible in exact micrometers.
   A pattern is added when its dual value exceeds its stock cost.
3. When no pattern has a positive reduced cost the LP value is a **lower bound**
   on the true cost, and it is reported in the plan notes.
4. **Rounding**: the LP pattern values are floored into real bars, the residual
   demand is placed by the FFD baseline on the remaining stock, and an exact 1D
   compaction pass (dissolve the least-filled bar into the others, best fit)
   removes the slack that rounding leaves behind.

If any step of the LP machinery fails a sanity check — infeasible master,
untrustworthy duals, a duality gap — the solver silently falls back to the FFD
baseline and says so in the notes, so a numerical problem can never produce a
bad plan.

### `cg-2d` (two-stage column generation)

The 2D counterpart of `cg-1d`, restricted to **two-stage guillotine** patterns:
a sheet is a stack of full-width strips, and every strip holds pieces of one
height laid out left to right. That is exactly what a panel saw or a glass
cutter does — crosscut into strips, then rip each strip.

- **Pricing** is a strip knapsack (maximise dual value across the usable width,
  in millimetres with conservative rounding) followed by a height knapsack that
  stacks those strips into a sheet. Rotation is handled by treating each
  orientation as a separate item; the master keeps demand per original part.
- **The master LP is a demand-rationing model**, not a plain covering LP:
  columns are patterns, unserved pieces (penalised) and unused sheet
  availability. That keeps the LP feasible and meaningful for stock-limited
  orders: it serves as much demand as the available stock allows and never
  plans more sheets than exist. Plan notes report the LP's view, e.g.
  "LP view: 19.8 of 56 pieces can be served (36.2 unmet) at a sheet cost of 264".
- **Rounding** instantiates only patterns that fill at least 55% of the usable
  sheet; thin patterns (typically LP basis artefacts) are left to the beam-based
  residual pass instead of wasting stock on near-empty sheets.
- Because the master minimises **stock cost** (not sheet count), it happily
  trades one expensive sheet for two cheap ones when that uses less material —
  which is exactly what the objective asks for.

### `maxrects-2d` (free cutting)

MaxRects keeps a list of maximal free rectangles, places the best-fitting piece
in a corner, splits the affected rectangles and prunes the ones contained in
another. Each sheet is packed three times — best-area-fit, best-short-side-fit
and bottom-left — and the arrangement with the highest sheet score wins.

It enforces **no guillotine constraint**, which is the correct model for CNC
routers, lasers and waterjets, and it is the only solver whose capability
matches `cutMode: free`. Layouts from it are not guaranteed to have an
edge-to-edge cut sequence, so the registry never uses it for a guillotine order.

Measured honestly: on the benchmark set it ties the guillotine solvers on sheet
count, and its leftovers are *more fragmented* — the free rectangles overlap by
construction, so only a non-overlapping subset can be reported as reusable
offcuts (7.7 m² vs 10.6 m² on `glass-free`). That is a real trade-off, not a
reporting artefact: free packing leaves a skeleton that is harder to reuse.
The portfolio therefore keeps a guillotine plan when one scores the same.

### `beam-2d`

A search state is a set of free rectangles plus the placements made so far
(linked through parent pointers, so copying a state is cheap). For every step
the solver:

1. picks the smallest free rectangles first — filling tight gaps keeps large
   contiguous areas free for large pieces;
2. tries the largest remaining part groups plus the two smallest, so slivers can
   still be filled;
3. places a piece in the lower-left corner of a region and creates **both**
   guillotine residuals: crosscut above the piece first, or rip beside it first;
4. keeps the best `Width` states by the heuristic
   `packed area − 0.25 × unusable free area − 1% × usable area × extra fragments`.

States that can no longer hold anything are skipped, not deleted, so their area
is still reported as offcut or scrap. The search is any-time: when the context
expires it returns the best state found so far, and the portfolio forwards
improving partial plans over `progress` for live UI updates.

Determinism: candidate order, rectangle order and tie-breaks are all total
orders, so the same problem always yields the same plan.

Measured effect of beam width on the benchmark set: 14 → 26.8% mean waste at
width 24, and width 32 was slightly worse (28.3%) — beam search is not
monotonic in width. 24 is the shipped default.

### `polish-2d` (local search)

Beam search is a constructive heuristic: it never revisits a decision. The local
search takes its plan and fixes the expensive mistakes, in this order of value:

1. **dissolve a sheet** — move every piece of the least-used sheet into the other
   sheets, largest first, choosing the fullest destination that still accepts
   the piece (best fit). If the last piece lands, a whole sheet is gone. This is
   the only move that reduces sheet count, which the objective rewards directly.
2. **merge two sheets** — pack both part sets into a single sheet of the larger
   format.
3. **repack a sheet** with a wider beam and more candidates, which cannot remove
   a sheet but often turns unusable scrap into reusable offcuts.

Every candidate is rebuilt with the same guillotine-safe beam packer and scored
with the real objective, so an accepted move is always valid and always an
improvement of at least 0.01 points (smaller gains are noise and are rejected —
without that threshold the search chased rounding dust and never settled).

Bounded by three things, whichever comes first: the time budget (the tighter of
the caller's context and `Problem.BudgetMS`), 25 rounds, or 14 fruitless rounds.
On the benchmark set it converges in roughly 250–500 ms per instance, so it does
not simply grind the clock.

### `best-2d` (portfolio)

Runs every 2D strategy with an equal share of the time budget and returns the
plan with the highest `core.Score`. Forwarded progress is monotonic: only
improving partial plans reach the caller.

Note the objective, not waste percentage, decides the winner. Fulfilling one
more part is worth more than saving some area, because demand fulfilment is the
first priority in the default weights. The benchmark reports all three numbers
(fill, waste, score) so this is never hidden.

## Remnant-first allocation

Physical leftovers can enter a solve as stock items flagged `isRemnant` (the
`includeRemnants` query parameter makes the API load the plant's available
remnants from the stock pool). The rules flag `preferRemnants` (default true)
then:

- **orders the stock list** so remnants are offered first — constructive solvers
  (shelf, beam, MaxRects, FFD) fill them before touching a fresh sheet
  (`core.PrioritizeRemnants`, a stable partition, so plans stay deterministic);
- **does not charge remnant sheets as new sheets** in `core.Score`: the
  `Metrics.RemnantSheets` count is subtracted from the sheet term, because a
  remnant was paid for when its original sheet was bought. Its prorated cost
  still appears in `Metrics.Cost`, so consuming a valuable leftover is visible.

This is a policy, not a hard constraint: the portfolio still picks the plan with
the best objective score, and `explain.Notes` says either how many sheets came
from remnants or that the available remnants did not improve the plan. A remnant
that is too small to hold the demand efficiently stays in the pool instead of
being chopped into scrap — which is the right shop decision.

## Benchmark harness

```powershell
cd backend
go run ./cmd/cutoptics bench -dir testdata/benchmarks -random 6   # run everything
go run ./cmd/cutoptics bench -update                              # accept current quality
go run ./cmd/cutoptics bench -check                               # regression gate, exit 1 on failure
go run ./cmd/cutoptics bench -solvers beam-2d,shelf-2d -random 20 # focus on specific solvers
```

Instances come from two places:

- **committed JSON files** (`backend/testdata/benchmarks/*.json`) for stable,
  reviewable cases: `glass-mixed`, `wood-panels` and `glass-interlock` (2D
  guillotine), `glass-free` (2D free cutting, which also exercises the
  guillotine solvers), `bars-varied`, `bars-homogeneous` and `metal-bars` (1D);
- **generated instances** (`bench.Generate`) for volume. The generator is part
  of the package, seeded by `bench.DefaultSeed`, so a run is fully reproducible.

Solvers are only run on instances they can serve (same dimension profile and a
cut mode they support), exactly like the registry, so a free-cutting layout is
never scored against a guillotine order.

The **golden file** (`testdata/benchmarks/golden.json`) stores the accepted mean
score per solver. Runs use `bench.DefaultBudgetMS` (500 ms) per solve; the golden
file is budget-specific, so changing `-budget` means re-running `-update`.
`-check` fails when a solver's mean score drops by more than the tolerance
(default 0.5 points), when a run fails, when a validation error appears, or when
a golden entry produced no rows. `go test ./...` runs the same gate
(`TestGoldenGate`, skipped by `go test -short`), so CI catches regressions even
if nobody runs the CLI.

### Current numbers (seed 7, 7 committed + 6 generated instances, 500 ms budget)

The score follows the objective: fulfil demand first, then the weighted terms
(fresh sheet count — remnant sheets are not charged, stock **cost**, scrap,
pattern count, offcut area). Cost is part of the score — stock prices are read
from the problem, so a plan that uses two cheap sheets beats one that uses a
single expensive sheet.

| Solver | Profile | Runs | Fill % | Yield % | Waste % | Score | Total ms |
|---|---|---|---|---|---|---|---|
| `best-2d` | 2D | 10 | 94.1 | 74.4 | 25.6 | **635.9** | 340 |
| `cg-2d` | 2D | 10 | 94.1 | 73.7 | 26.3 | 632.4 | 24 |
| `polish-2d` | 2D | 10 | 93.7 | 72.7 | 27.3 | 626.2 | 2852 |
| `beam-2d` | 2D | 10 | 93.7 | 70.8 | 29.2 | 616.0 | 75 |
| `shelf-2d` | 2D | 10 | 92.5 | 64.3 | 35.7 | 569.6 | <1 |
| `maxrects-2d` | 2D (free) | 2 | 100.0 | 61.3 | 38.7 | 779.5 | <1 |
| `cg-1d` | 1D | 3 | 94.0 | 97.1 | **2.9** | 495.2 | 1 |
| `best-1d` | 1D | 3 | 94.0 | 97.1 | 2.9 | 495.2 | 1 |
| `ffd-1d` | 1D | 3 | 90.7 | 94.4 | 5.6 | 450.3 | <1 |

`maxrects-2d` runs only on the two free-cutting instances, so its row is not
comparable with the others — it is included for completeness.

Where the solvers bite (beam → column generation, committed instances):

| Instance | Result |
|---|---|
| `glass-mixed` | 5 sheets / 33.4% waste → **6 cheaper sheets / 31.0% waste**, score 672.9 → 676.9 |
| `wood-panels` | 5 sheets / 28.8% waste → **6 sheets / 22.9% waste**, score 782.7 → 796.5 |
| `glass-free` | score 674.2 → **716.6** at 22.2% waste |
| `bars-varied` (1D) | 28 bars / 7.9% waste → **26 bars / 0.8% waste** (equal to the LP bound, so optimal) |
| `metal-bars` (only 8 bars) | 36/50 pieces → **41/50 pieces** at 98.8% yield |

2D progression on mean waste: shelf 35.7% → beam 29.2% → local search 27.3% →
two-stage column generation 26.3%. `cg-2d` reaches that in ~2 ms per instance
while `polish-2d` needs a few hundred.

Progression: shelf 35.0% waste → beam 26.8% → local search 24.5%. Where the
local search bites (beam → polish):

| Instance | Sheets | Waste % | Note |
|---|---|---|---|
| `random-02` | 9 → **7** | 34.9 → 21.3 | two sheets dissolved |
| `random-03` | 7 → **6** | 26.2 → 21.2 | one sheet dissolved |
| `random-04` | 7 | 26.5 | offcuts 7.21 → 8.02 m², scrap 6.17 → 5.35 m² (repack) |
| others | unchanged | unchanged | already tight; 14 fruitless rounds stop the search |

`best-2d` is the portfolio (shelf + beam + polish) and matches `polish-2d` here
while usually finishing faster, because the cheap strategies often already win.

## Adding a solver

1. Implement `core.Solver` in the right package (`pack1d`, `pack2d`, or a new
   one), returning `core.Solution` values.
2. Use `assembleSolution` (2D) or `core.Summarize` so metrics are comparable.
3. Call `cutter.ForSheet` for guillotine plans so cut steps are attached.
4. Register it in `optimizer.DefaultRegistry` and give it a `Rank`.
5. Add an instance or two that represent its strengths to
   `backend/testdata/benchmarks`, then run `bench -update` and commit the golden
   file together with the solver.

## Known limits and next steps

- Column generation is two-stage only (same-height strips). Three-stage and
  free-form guillotine patterns need different pricing; the LP machinery is
  ready for them.
- `cg-2d`'s rounding only instantiates patterns above 55% fill; the beam
  handles the rest. Better rounding (sequential residual LPs instead of a single
  floor plus beam) would squeeze more out of the pattern pool.
- Irregular nesting (polygons) is still out of scope — `maxrects-2d` covers
  free cutting for rectangles only.
- The local search moves whole sheets; it does not swap individual pieces to
  improve offcut shapes.
- 3D bin packing is not implemented; the registry and `Capabilities` are shaped
  so it can be added without touching callers.
