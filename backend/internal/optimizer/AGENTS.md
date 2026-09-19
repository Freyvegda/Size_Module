# internal/optimizer/ — pure Go solver engine

This package tree is the heart of the product: stock-in / parts-in →
validated cutting plan out. It must stay **free of DB, HTTP and filesystem
dependencies** (benchmark/test code may read `testdata/`). All lengths are
integer micrometers (`core.Dim = int64`).

## Packages

| Package | Role | Key symbols |
|---|---|---|
| `optimizer.go` (root) | facade: builds the default registry, solves, reports violations | `DefaultRegistry()`, `Solve()`, `DetectProfile()`, `Result` |
| `core/` | domain language + solver seam | `Problem`, `Solution`, `Rules`, `Part`, `StockItem`, `Rect`, `Placement`, `SheetPlan`, `Metrics`, `Capabilities`, `Solver`, `Registry`, `Summarize`, `Score` |
| `geom/` | rectangle math + guillotine cut-tree reconstruction | `Separated`, `Intersects`, `SplitV/H`, `BuildCutTree`, `Instructions` |
| `pack1d/` | 1D solver for bars/profiles | `FFDSolver` (`ffd-1d`) |
| `pack2d/` | 2D solvers for sheets/panels | `ShelfSolver` (`shelf-2d`), `BeamSolver` (`beam-2d`), `PolishSolver` (`polish-2d`), `PortfolioSolver` (`best-2d`), `assembleSolution` |
| `cutter/` | turns a sheet layout into ordered cut steps | `ForSheet(sheet, kerf)` |
| `validator/` | mandatory gate before a plan is shown/stored | `Validate(problem, solution)`, `Violation` |
| `explain/` | plain-language notes, waste split, lower bound | `Notes(problem, solution)` |
| `bench/` | benchmark harness + golden regression gate | `LoadDir`, `Run`, `Report`, `Generate`, `Golden`, `Compare` |

## The seam

Everything implements `core.Solver` (`core/solver.go`):

```go
type Solver interface {
    Name() string
    Version() string
    Capabilities() Capabilities
    Solve(ctx context.Context, p Problem, progress ProgressFunc) (Solution, error)
}
```

`Registry` ranks solvers — **lower rank wins**, ties break by name:

| Solver | Profile | Rank | Notes |
|---|---|---|---|
| `best-2d` | 2D | 1 | portfolio: runs the other 2D solvers, keeps best `core.Score` |
| `polish-2d` | 2D | 3 | beam plan + local search (dissolve/merge/repack sheets) |
| `beam-2d` | 2D | 10 | beam search over guillotine cut trees; ship width 24 |
| `shelf-2d` | 2D | 20 | fast baseline |
| `ffd-1d` | 1D | 10 | First-Fit-Decreasing |

Progress callbacks must be **monotonic** (only improving partial plans).

## Invariants

- Every solution goes through `validator.Validate`: bounds, kerf, overlap,
  grain, demand limits and **guillotine cut-tree feasibility**. Never bypass it.
- Determinism: candidate, rectangle and tie-break orders are total; same
  problem + seed + solver version ⇒ byte-identical plan.
- Metrics come from `core.Summarize` / `assembleSolution` so all solvers are
  comparable.

## Quality gate (do not regress)

`backend/testdata/benchmarks/golden.json` stores accepted mean scores
(seed 7, 500 ms budget). `bench -check` and `TestGoldenGate` (in
`bench/quality_test.go`) fail when a solver drops more than 0.5 points.
See `docs/solver.md` for the current numbers (shelf 35.0% →
beam 26.8% → polish 24.5% mean waste) and the beam-width experiment.

## Adding a solver

1. Implement `core.Solver` in a suitable package, returning `core.Solution`.
2. Use `core.Summarize` (1D) / `assembleSolution` (2D) and attach cut steps via
   `cutter.ForSheet` when guillotine.
3. Register it in `optimizer.DefaultRegistry()` with a `Rank`.
4. Add/extend benchmark instances, run `go run ./cmd/cutoptics bench -update`,
   commit `golden.json` with the solver.

## Commands (from `backend/`)

```powershell
go test ./internal/optimizer/...        # unit + property tests
go test ./...                           # includes the golden gate
go run ./cmd/cutoptics bench -check     # regression gate only
```
