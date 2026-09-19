package bench_test

import (
	"context"
	"testing"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/bench"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/validator"
)

// TestColumnSolverPlacesGeneratedOrders guards the rounding path: column
// generation must never serve fewer pieces than the beam baseline, and every
// plan it emits has to pass validation.
func TestColumnSolverPlacesGeneratedOrders(t *testing.T) {
	registry := optimizer.DefaultRegistry()
	columns, ok := registry.Get("cg-2d")
	if !ok {
		t.Fatal("cg-2d is not registered")
	}
	beam, ok := registry.Get("beam-2d")
	if !ok {
		t.Fatal("beam-2d is not registered")
	}

	instances, err := bench.LoadDir(benchDir)
	if err != nil {
		t.Fatalf("loading benchmark instances: %v", err)
	}
	for i := 0; i < 6; i++ {
		instances = append(instances, bench.Generate(bench.DefaultSeed, i))
	}

	checked := 0
	for _, in := range instances {
		p := in.Problem()
		if core.DetectProfile(p) != core.Profile2D {
			continue
		}
		if p.Rules.CutMode != core.CutGuillotine {
			continue
		}
		p.BudgetMS = 500
		checked++

		columnSolution, err := columns.Solve(context.Background(), p, nil)
		if err != nil {
			t.Fatalf("%s: cg-2d solve: %v", in.Name, err)
		}
		beamSolution, err := beam.Solve(context.Background(), p, nil)
		if err != nil {
			t.Fatalf("%s: beam solve: %v", in.Name, err)
		}
		for _, v := range validator.Validate(p, columnSolution) {
			if v.Severity == "error" {
				t.Errorf("%s: %s (sheet %d, part %s)", in.Name, v.Message, v.SheetIndex, v.PartCode)
			}
		}
		if columnSolution.Metrics.PartsPlaced < beamSolution.Metrics.PartsPlaced {
			t.Errorf("%s: cg-2d placed %d pieces, beam placed %d",
				in.Name, columnSolution.Metrics.PartsPlaced, beamSolution.Metrics.PartsPlaced)
		}
	}
	if checked == 0 {
		t.Fatal("no 2d guillotine instances were checked")
	}
}
