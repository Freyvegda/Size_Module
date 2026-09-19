package pack2d

import (
	"context"
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/validator"
)

func freeProblem() core.Problem {
	rules := core.DefaultRules()
	rules.CutMode = core.CutFree
	p := sampleProblem()
	p.Rules = rules
	return core.Normalize(p)
}

func TestMaxRectsProducesValidFreeLayout(t *testing.T) {
	p := freeProblem()
	sol, err := NewMaxRects().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	for _, v := range validator.Validate(p, sol) {
		if v.Severity == "error" {
			t.Errorf("validation error: %s", v.Message)
		}
	}
	if sol.Metrics.PartsPlaced != 14 {
		t.Fatalf("expected 14 pieces placed, got %d", sol.Metrics.PartsPlaced)
	}
	if sol.Solver != "maxrects-2d" {
		t.Fatalf("unexpected solver %q", sol.Solver)
	}
}

func TestMaxRectsIsDeterministic(t *testing.T) {
	p := freeProblem()
	first, err := NewMaxRects().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewMaxRects().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
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

func TestMaxRectsKeepsReusableOffcuts(t *testing.T) {
	rules := core.DefaultRules()
	rules.CutMode = core.CutFree
	p := core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "pane", Code: "PANE-1000x800", Width: core.FromMM(1000), Height: core.FromMM(800), Quantity: 1},
		},
		Stocks: []core.StockItem{
			{ID: "sheet", Code: "SHEET-3000x2000", Width: core.FromMM(3000), Height: core.FromMM(2000), Quantity: 1},
		},
		Rules: rules,
	})
	sol, err := NewMaxRects().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if len(sol.Sheets) != 1 || len(sol.Sheets[0].Offcuts) == 0 {
		t.Fatalf("expected reusable offcuts, got %+v", sol.Sheets)
	}
	if sol.Metrics.OffcutAreaM2 < 3 {
		t.Fatalf("expected at least 3 m² of offcuts, got %.2f", sol.Metrics.OffcutAreaM2)
	}
	if sol.Metrics.OffcutAreaM2 > sol.Metrics.StockAreaM2 {
		t.Fatalf("offcut area (%.2f) cannot exceed stock area (%.2f)", sol.Metrics.OffcutAreaM2, sol.Metrics.StockAreaM2)
	}
}

// In free mode a layout may interlock arbitrarily; the validator only requires
// that the pieces fit, respect the kerf and do not overlap.
func TestMaxRectsPlacesAllPiecesWhenThereIsRoom(t *testing.T) {
	rules := core.DefaultRules()
	rules.CutMode = core.CutFree
	p := core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "a", Code: "A", Width: core.FromMM(600), Height: core.FromMM(400), Quantity: 1},
			{ID: "b", Code: "B", Width: core.FromMM(400), Height: core.FromMM(600), Quantity: 1},
			{ID: "c", Code: "C", Width: core.FromMM(600), Height: core.FromMM(400), Quantity: 1},
			{ID: "d", Code: "D", Width: core.FromMM(400), Height: core.FromMM(600), Quantity: 1},
		},
		Stocks: []core.StockItem{
			{ID: "s", Code: "SHEET-1400x1400", Width: core.FromMM(1400), Height: core.FromMM(1400), Quantity: 1},
		},
		Rules: rules,
	})
	sol, err := NewMaxRects().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	for _, v := range validator.Validate(p, sol) {
		if v.Severity == "error" {
			t.Errorf("free layout rejected: %s", v.Message)
		}
	}
	if len(sol.Sheets) != 1 || len(sol.Sheets[0].Placements) != 4 {
		t.Fatalf("expected all four pieces on one sheet, got %+v", sol.Sheets)
	}
}
