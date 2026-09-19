package pack2d

import (
	"context"
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/validator"
)

// wastefulPlan builds a deliberately bad plan by hand: six 500x500 pieces split
// over two sheets, although all of them fit on one.
func wastefulPlan(t *testing.T) (core.Problem, core.Solution) {
	t.Helper()
	p := core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "a", Code: "PLATE-500x500", Width: core.FromMM(500), Height: core.FromMM(500), Quantity: 6, AllowRotate: true},
		},
		Stocks: []core.StockItem{
			{ID: "sheet", Code: "SHEET-3210x2250", Width: core.FromMM(3210), Height: core.FromMM(2250), Quantity: 2},
		},
		Rules: core.DefaultRules(),
	})

	piece := func(x float64) core.Placement {
		return core.Placement{
			PartID:   "a",
			PartCode: "PLATE-500x500",
			X:        core.FromMM(x),
			Y:        core.FromMM(10),
			W:        core.FromMM(500),
			H:        core.FromMM(500),
		}
	}
	sheets := []core.SheetPlan{
		{
			Index: 0, StockID: "sheet", StockCode: "SHEET-3210x2250", Label: "Sheet 1",
			Width: core.FromMM(3210), Height: core.FromMM(2250),
			Placements: []core.Placement{piece(10), piece(514), piece(1018)},
		},
		{
			Index: 1, StockID: "sheet", StockCode: "SHEET-3210x2250", Label: "Sheet 2",
			Width: core.FromMM(3210), Height: core.FromMM(2250),
			Placements: []core.Placement{piece(10), piece(514), piece(1018)},
		},
	}
	return p, core.Solution{
		Solver:  "manual",
		Sheets:  sheets,
		Metrics: core.Summarize(p, sheets, nil, 0),
	}
}

func TestImproveDissolvesASheet(t *testing.T) {
	p, sol := wastefulPlan(t)
	before := core.Score(p, sol.Metrics)

	improved := Improve(context.Background(), p, sol, 1, nil)
	after := core.Score(p, improved.Metrics)

	if len(improved.Sheets) != 1 {
		t.Fatalf("expected local search to dissolve a sheet, got %d sheets", len(improved.Sheets))
	}
	if after <= before {
		t.Fatalf("score must improve: before %.2f, after %.2f", before, after)
	}
	if placed := improved.Metrics.PartsPlaced; placed != 6 {
		t.Fatalf("expected all 6 pieces to survive the move, got %d", placed)
	}
	for _, v := range validator.Validate(p, improved) {
		if v.Severity == "error" {
			t.Errorf("validation error after improvement: %s", v.Message)
		}
	}
}

func TestImproveIsDeterministic(t *testing.T) {
	p, sol := wastefulPlan(t)
	first := Improve(context.Background(), p, sol, 42, nil)
	second := Improve(context.Background(), p, sol, 42, nil)

	if len(first.Sheets) != len(second.Sheets) {
		t.Fatalf("sheet counts differ: %d vs %d", len(first.Sheets), len(second.Sheets))
	}
	for i := range first.Sheets {
		a, b := first.Sheets[i], second.Sheets[i]
		if len(a.Placements) != len(b.Placements) {
			t.Fatalf("sheet %d placement counts differ", i)
		}
		for j := range a.Placements {
			if a.Placements[j] != b.Placements[j] {
				t.Fatalf("sheet %d placement %d differs: %+v vs %+v", i, j, a.Placements[j], b.Placements[j])
			}
		}
	}
}

func TestPolishSolverProducesValidPlan(t *testing.T) {
	p := sampleProblem()
	p.BudgetMS = 250

	sol, err := NewPolish().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	for _, v := range validator.Validate(p, sol) {
		if v.Severity == "error" {
			t.Errorf("validation error: %s", v.Message)
		}
	}
	if sol.Solver != "polish-2d" {
		t.Fatalf("unexpected solver name %q", sol.Solver)
	}
	if sol.Metrics.PartsPlaced != 14 {
		t.Fatalf("expected 14 pieces placed, got %d", sol.Metrics.PartsPlaced)
	}
	if len(sol.Notes) == 0 {
		t.Fatal("expected the polish solver to explain what the local search did")
	}
}
