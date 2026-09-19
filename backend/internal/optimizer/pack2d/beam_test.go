package pack2d

import (
	"context"
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/validator"
)

func TestBeamProducesValidLayout(t *testing.T) {
	p := sampleProblem()
	sol, err := NewBeam().Solve(context.Background(), p, nil)
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
	if len(sol.Unplaced) != 0 {
		t.Fatalf("expected no unplaced pieces, got %+v", sol.Unplaced)
	}
	if len(sol.Sheets) == 0 || len(sol.Sheets[0].CutSteps) == 0 {
		t.Fatal("expected cut instructions on the first sheet")
	}
	if sol.Solver != "beam-2d" {
		t.Fatalf("unexpected solver name %q", sol.Solver)
	}
}

func TestBeamIsDeterministic(t *testing.T) {
	p := sampleProblem()
	a, err := NewBeam().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewBeam().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Sheets) != len(b.Sheets) {
		t.Fatalf("sheet count differs: %d vs %d", len(a.Sheets), len(b.Sheets))
	}
	for i := range a.Sheets {
		if len(a.Sheets[i].Placements) != len(b.Sheets[i].Placements) {
			t.Fatalf("sheet %d placement count differs", i)
		}
		for j := range a.Sheets[i].Placements {
			pa, pb := a.Sheets[i].Placements[j], b.Sheets[i].Placements[j]
			if pa != pb {
				t.Fatalf("sheet %d placement %d differs: %+v vs %+v", i, j, pa, pb)
			}
		}
	}
}

func TestBeamKeepsLargeOffcuts(t *testing.T) {
	p := core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "pane", Code: "PANE-1000x800", Width: core.FromMM(1000), Height: core.FromMM(800), Quantity: 1, AllowRotate: true},
		},
		Stocks: []core.StockItem{
			{ID: "sheet", Code: "SHEET-3000x2000", Width: core.FromMM(3000), Height: core.FromMM(2000), Quantity: 1},
		},
		Rules: core.DefaultRules(),
	})
	sol, err := NewBeam().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if len(sol.Sheets) != 1 {
		t.Fatalf("expected a single sheet, got %d", len(sol.Sheets))
	}
	offcuts := sol.Sheets[0].Offcuts
	if len(offcuts) == 0 {
		t.Fatal("expected reusable offcuts from a 6 m² sheet holding one 0.8 m² pane")
	}
	if sol.Metrics.OffcutAreaM2 < 3 {
		t.Fatalf("expected at least 3 m² of reusable offcuts, got %.2f", sol.Metrics.OffcutAreaM2)
	}
}

func TestBeamReportsOversizedPart(t *testing.T) {
	p := core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "huge", Code: "GIANT", Width: core.FromMM(4000), Height: core.FromMM(4000), Quantity: 2, AllowRotate: true},
		},
		Stocks: []core.StockItem{
			{ID: "s", Code: "SMALL", Width: core.FromMM(1000), Height: core.FromMM(1000), Quantity: 2},
		},
		Rules: core.DefaultRules(),
	})
	sol, err := NewBeam().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if len(sol.Unplaced) != 1 || sol.Unplaced[0].PartCode != "GIANT" || sol.Unplaced[0].Quantity != 2 {
		t.Fatalf("expected 2 x GIANT unplaced, got %+v", sol.Unplaced)
	}
	if sol.Unplaced[0].Reason != "part is larger than any usable stock size" {
		t.Fatalf("unexpected reason: %s", sol.Unplaced[0].Reason)
	}
}
