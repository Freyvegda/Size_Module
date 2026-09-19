// Quality tests across solvers: they run the real benchmark set, so `go test`
// enforces the same bar as `cutoptics bench -check`.
package bench_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/bench"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/pack2d"
	"github.com/size-module/backend/internal/optimizer/validator"
)

const benchDir = "../../../testdata/benchmarks"

type namedProblem struct {
	name string
	p    core.Problem
}

func loadProblems(t *testing.T) []namedProblem {
	t.Helper()
	instances, err := bench.LoadDir(benchDir)
	if err != nil {
		t.Fatalf("loading benchmark instances: %v", err)
	}
	for i := 0; i < 6; i++ {
		instances = append(instances, bench.Generate(bench.DefaultSeed, i))
	}
	out := make([]namedProblem, 0, len(instances))
	for _, in := range instances {
		p := in.Problem()
		// Validity and invariant tests do not depend on the time budget; a
		// tiny budget keeps them fast.
		p.BudgetMS = 60
		out = append(out, namedProblem{name: in.Name, p: p})
	}
	return out
}

func twoDSolvers() []core.Solver {
	// The portfolio wraps the others; TestPortfolioKeepsTheBestStrategy and
	// TestGoldenGate cover it, so it is left out here to keep this test fast.
	return []core.Solver{
		pack2d.New(),
		pack2d.NewBeam(),
		pack2d.NewPolish(),
	}
}

func TestSolversProduceValidPlans(t *testing.T) {
	for _, item := range loadProblems(t) {
		if optimizer.DetectProfile(item.p) != core.Profile2D {
			continue
		}
		for _, solver := range twoDSolvers() {
			sol, err := solver.Solve(context.Background(), item.p, nil)
			if err != nil {
				t.Fatalf("%s on %s: %v", solver.Name(), item.name, err)
			}
			for _, v := range validator.Validate(item.p, sol) {
				if v.Severity == "error" {
					t.Errorf("%s on %s: %s", solver.Name(), item.name, v.Message)
				}
			}
		}
	}
}

func TestBeamBeatsShelfOnTheBenchmarkSet(t *testing.T) {
	shelf := pack2d.New()
	beam := pack2d.NewBeam()
	var shelfWaste, beamWaste float64
	runs := 0

	for _, item := range loadProblems(t) {
		if optimizer.DetectProfile(item.p) != core.Profile2D {
			continue
		}
		s, err := shelf.Solve(context.Background(), item.p, nil)
		if err != nil {
			t.Fatalf("shelf on %s: %v", item.name, err)
		}
		b, err := beam.Solve(context.Background(), item.p, nil)
		if err != nil {
			t.Fatalf("beam on %s: %v", item.name, err)
		}
		shelfWaste += s.Metrics.WastePct
		beamWaste += b.Metrics.WastePct
		runs++
	}
	if runs == 0 {
		t.Fatal("no 2D instances found")
	}
	// Measured roughly 35% (shelf) vs 27% (beam). Keep a 5% relative margin so
	// the test catches a real regression without being brittle.
	if beamWaste > shelfWaste*0.95 {
		t.Errorf("beam search should beat the shelf baseline by at least 5%% relative: shelf %.1f%%, beam %.1f%%",
			shelfWaste/float64(runs), beamWaste/float64(runs))
	}
}

func TestPortfolioKeepsTheBestStrategy(t *testing.T) {
	shelf := pack2d.New()
	beam := pack2d.NewBeam()
	portfolio := pack2d.NewPortfolio(shelf, beam, pack2d.NewPolish())
	const epsilon = 1e-9

	for _, item := range loadProblems(t) {
		if optimizer.DetectProfile(item.p) != core.Profile2D {
			continue
		}
		s, err := shelf.Solve(context.Background(), item.p, nil)
		if err != nil {
			t.Fatalf("shelf on %s: %v", item.name, err)
		}
		b, err := beam.Solve(context.Background(), item.p, nil)
		if err != nil {
			t.Fatalf("beam on %s: %v", item.name, err)
		}
		best, err := portfolio.Solve(context.Background(), item.p, nil)
		if err != nil {
			t.Fatalf("portfolio on %s: %v", item.name, err)
		}
		bestScore := core.Score(item.p, best.Metrics)
		if bestScore < core.Score(item.p, s.Metrics)-epsilon || bestScore < core.Score(item.p, b.Metrics)-epsilon {
			t.Errorf("%s: portfolio score %.3f is below a strategy (shelf %.3f, beam %.3f)",
				item.name, bestScore, core.Score(item.p, s.Metrics), core.Score(item.p, b.Metrics))
		}
	}
}

// TestGoldenGate mirrors `cutoptics bench -check`: the committed golden file
// must not regress on the committed instance set plus the generated ones. It is
// the slowest test in the repository because the polish solver spends its time
// budget; use `go test -short ./...` while iterating.
func TestGoldenGate(t *testing.T) {
	if testing.Short() {
		t.Skip("benchmark gate runs the solvers with a real time budget")
	}
	registry := optimizer.DefaultRegistry()
	instances, err := bench.LoadDir(benchDir)
	if err != nil {
		t.Fatalf("loading benchmark instances: %v", err)
	}
	for i := 0; i < 6; i++ {
		instances = append(instances, bench.Generate(bench.DefaultSeed, i))
	}

	rows := bench.Run(context.Background(), registry, instances, registry.Names(), bench.DefaultBudgetMS)
	totals := bench.Aggregate(rows)

	golden, err := bench.LoadGolden(filepath.Join(benchDir, "golden.json"))
	if err != nil {
		t.Fatalf("loading golden file: %v", err)
	}
	if problems := bench.Compare(golden, totals, bench.GoldenTolerancePct); len(problems) > 0 {
		for _, problem := range problems {
			t.Errorf("golden check: %s", problem)
		}
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	a := bench.Generate(5, 2)
	b := bench.Generate(5, 2)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("the same seed and index must produce the same instance")
	}
	c := bench.Generate(6, 2)
	if reflect.DeepEqual(a, c) {
		t.Fatal("different seeds should produce different instances")
	}
}
