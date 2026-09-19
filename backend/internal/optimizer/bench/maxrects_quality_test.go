package bench_test

import (
	"context"
	"testing"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/bench"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/validator"
)

// MaxRects layouts are not guillotine-cuttable, but they still have to be
// inside the sheet, respect the kerf and never overlap.
func TestMaxRectsProducesValidLayouts(t *testing.T) {
	instances, err := bench.LoadDir(benchDir)
	if err != nil {
		t.Fatalf("loading benchmark instances: %v", err)
	}
	for i := 0; i < 6; i++ {
		instances = append(instances, bench.Generate(bench.DefaultSeed, i))
	}

	registry := optimizer.DefaultRegistry()
	solver, ok := registry.Get("maxrects-2d")
	if !ok {
		t.Fatal("maxrects-2d is not registered")
	}

	checked := 0
	for _, in := range instances {
		p := in.Problem()
		p.BudgetMS = 500
		if optimizer.DetectProfile(p) != core.Profile2D {
			continue
		}
		if p.Rules.CutMode != core.CutFree {
			// Free layouts on a guillotine order would be invalid by design.
			continue
		}
		sol, err := solver.Solve(context.Background(), p, nil)
		if err != nil {
			t.Fatalf("%s: solve: %v", in.Name, err)
		}
		checked++
		for _, v := range validator.Validate(p, sol) {
			if v.Severity == "error" {
				t.Errorf("%s: %s (sheet %d, part %s)", in.Name, v.Message, v.SheetIndex, v.PartCode)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no free-cutting instances were checked")
	}
}
