// Package bench runs solvers against a set of instances and reports quality and
// speed. It is the quality gate for solver work: a change that makes plans
// worse must not slip through unnoticed.
//
// What "worse" means: the objective score (core.Score) of the normalized
// problem. Waste percentage alone is not the goal — a plan that packs tighter
// but places fewer parts is worse, because demand fulfilment is the first
// priority. The report therefore shows fulfilment, waste and score side by
// side, and the golden file gates on score.
//
// Two kinds of instances feed it:
//
//   - committed JSON files (testdata/benchmarks) for stable, reviewable cases
//   - deterministic generated instances for volume, with the generator living
//     in this package so the seed fully reproduces a run
package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/validator"
)

// Instance is one benchmark problem: a problem plus a name.
type Instance struct {
	Name   string           `json:"name"`
	Parts  []core.Part      `json:"parts"`
	Stocks []core.StockItem `json:"stocks"`
	Rules  core.Rules       `json:"rules,omitempty"`
}

// Problem returns the normalized, ready-to-solve problem.
func (in Instance) Problem() core.Problem {
	return core.Normalize(core.Problem{Parts: in.Parts, Stocks: in.Stocks, Rules: in.Rules})
}

// LoadDir reads every *.json instance in dir, sorted by file name. The golden
// file is skipped.
func LoadDir(dir string) ([]Instance, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "golden.json" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	out := make([]Instance, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var in Instance
		if err := json.Unmarshal(data, &in); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if in.Name == "" {
			in.Name = strings.TrimSuffix(name, ".json")
		}
		out = append(out, in)
	}
	return out, nil
}

// DefaultBudgetMS is the time budget per solve used by the benchmark unless the
// caller overrides it. The committed golden file was generated with this budget,
// so tests and CI reproduce it exactly. 500 ms is short enough to keep the gate
// fast and long enough for the local search to show what it can do.
const DefaultBudgetMS = 500

// Row is one solver's result on one instance.
type Row struct {
	Instance   string
	Solver     string
	Sheets     int
	Placed     int
	Requested  int
	FillPct    float64
	YieldPct   float64
	WastePct   float64
	Score      float64
	OffcutM2   float64
	ScrapM2    float64
	ElapsedMS  int64
	Violations int
	Failed     bool
}

// Run solves every instance with every compatible solver and returns the rows.
// Solvers whose dimension profile does not match an instance are skipped.
func Run(ctx context.Context, reg *core.Registry, instances []Instance, solverNames []string, budgetMS int) []Row {
	if budgetMS <= 0 {
		budgetMS = DefaultBudgetMS
	}
	var rows []Row
	for _, in := range instances {
		problem := in.Problem()
		problem.BudgetMS = budgetMS
		profile := optimizer.DetectProfile(problem)

		for _, name := range solverNames {
			solver, ok := reg.Get(name)
			if !ok || solver.Capabilities().Dimension != profile {
				continue
			}
			// Same compatibility rule as the registry: a guillotine layout is
			// valid when free cutting is allowed, but a free layout is not
			// guillotine-cuttable, so those pairs are skipped rather than
			// reported as solver failures.
			if !core.CutModeCompatible(solver.Capabilities().CutMode, problem.Rules.CutMode) {
				continue
			}
			// Solvers honour the time budget themselves; the small margin here
			// only lets them wrap up. Giving them extra seconds would silently
			// change the benchmark's meaning.
			runCtx, cancel := context.WithTimeout(ctx, time.Duration(budgetMS)*time.Millisecond+250*time.Millisecond)
			sol, err := solver.Solve(runCtx, problem, nil)
			cancel()

			row := Row{Instance: in.Name, Solver: name}
			if err != nil {
				row.Failed = true
				rows = append(rows, row)
				continue
			}
			row.Sheets = sol.Metrics.SheetCount
			row.Placed = sol.Metrics.PartsPlaced
			row.Requested = sol.Metrics.PartsRequested
			if row.Requested > 0 {
				row.FillPct = 100 * float64(row.Placed) / float64(row.Requested)
			}
			row.YieldPct = sol.Metrics.YieldPct
			row.WastePct = sol.Metrics.WastePct
			row.Score = core.Score(problem, sol.Metrics)
			row.OffcutM2 = sol.Metrics.OffcutAreaM2
			row.ScrapM2 = sol.Metrics.ScrapAreaM2
			row.ElapsedMS = sol.Metrics.ElapsedMS
			row.Violations = errorCount(validator.Validate(problem, sol))
			rows = append(rows, row)
		}
	}
	return rows
}

func errorCount(violations []validator.Violation) int {
	count := 0
	for _, v := range violations {
		if v.Severity == "error" {
			count++
		}
	}
	return count
}

// Totals aggregates the rows of one solver.
type Totals struct {
	Runs         int
	MeanFillPct  float64
	MeanYieldPct float64
	MeanWastePct float64
	MeanScore    float64
	TotalMS      int64
	Violations   int
	Failures     int
}

// Aggregate computes per-solver totals as the mean over the runs that did not
// fail.
func Aggregate(rows []Row) map[string]Totals {
	type acc struct {
		totals Totals
		fill   float64
		yield  float64
		waste  float64
		score  float64
	}
	bySolver := map[string]*acc{}
	var order []string
	for _, row := range rows {
		a, ok := bySolver[row.Solver]
		if !ok {
			a = &acc{}
			bySolver[row.Solver] = a
			order = append(order, row.Solver)
		}
		a.totals.Runs++
		a.totals.TotalMS += row.ElapsedMS
		a.totals.Violations += row.Violations
		if row.Failed {
			a.totals.Failures++
			continue
		}
		a.fill += row.FillPct
		a.yield += row.YieldPct
		a.waste += row.WastePct
		a.score += row.Score
	}
	out := make(map[string]Totals, len(bySolver))
	for _, name := range order {
		a := bySolver[name]
		divisor := float64(a.totals.Runs - a.totals.Failures)
		if divisor <= 0 {
			divisor = 1
		}
		a.totals.MeanFillPct = a.fill / divisor
		a.totals.MeanYieldPct = a.yield / divisor
		a.totals.MeanWastePct = a.waste / divisor
		a.totals.MeanScore = a.score / divisor
		out[name] = a.totals
	}
	return out
}

// Report writes a human readable table plus per-solver totals.
func Report(w io.Writer, rows []Row) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "INSTANCE\tSOLVER\tSHEETS\tPIECES\tFILL %\tYIELD %\tWASTE %\tSCORE\tOFFCUT m²\tSCRAP m²\tMS\tERR")
	for _, row := range rows {
		if row.Failed {
			fmt.Fprintf(tw, "%s\t%s\t-\t-\t-\t-\t-\t-\t-\t-\t-\tFAILED\n", row.Instance, row.Solver)
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d/%d\t%.1f\t%.1f\t%.1f\t%.1f\t%.2f\t%.2f\t%d\t%d\n",
			row.Instance, row.Solver, row.Sheets, row.Placed, row.Requested,
			row.FillPct, row.YieldPct, row.WastePct, row.Score,
			row.OffcutM2, row.ScrapM2, row.ElapsedMS, row.Violations)
	}
	tw.Flush()

	totals := Aggregate(rows)
	names := make([]string, 0, len(totals))
	for name := range totals {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Totals per solver (mean over its instances):")
	tw = tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "SOLVER\tRUNS\tFILL %\tYIELD %\tWASTE %\tSCORE\tTOTAL MS\tVIOLATIONS\tFAILURES")
	for _, name := range names {
		t := totals[name]
		fmt.Fprintf(tw, "%s\t%d\t%.1f\t%.1f\t%.1f\t%.1f\t%d\t%d\t%d\n",
			name, t.Runs, t.MeanFillPct, t.MeanYieldPct, t.MeanWastePct,
			t.MeanScore, t.TotalMS, t.Violations, t.Failures)
	}
	tw.Flush()
}
