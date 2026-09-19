package pack2d

import (
	"context"
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/validator"
)

func sampleProblem() core.Problem {
	parts := []core.Part{
		{ID: "p1", Code: "DOOR", Width: core.FromMM(700), Height: core.FromMM(1200), Quantity: 4, AllowRotate: true, Priority: 1},
		{ID: "p2", Code: "SIDE", Width: core.FromMM(550), Height: core.FromMM(1200), Quantity: 4, AllowRotate: true, Priority: 1},
		{ID: "p3", Code: "SHELF", Width: core.FromMM(500), Height: core.FromMM(250), Quantity: 6, AllowRotate: true, Priority: 2},
	}
	stocks := []core.StockItem{
		{ID: "s1", Code: "BOARD-2800x2070", Width: core.FromMM(2070), Height: core.FromMM(2800), Quantity: 4},
	}
	return core.Normalize(core.Problem{
		Parts:  parts,
		Stocks: stocks,
		Rules:  core.DefaultRules(),
	})
}

func TestShelfSolverProducesValidLayout(t *testing.T) {
	p := sampleProblem()
	ctx := context.Background()
	sol, err := New().Solve(ctx, p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	violations := validator.Validate(p, sol)
	for _, v := range violations {
		if v.Severity == "error" {
			t.Errorf("validation error: %s", v.Message)
		}
	}
	if sol.Metrics.PartsPlaced != 14 {
		t.Fatalf("expected 14 pieces placed, got %d", sol.Metrics.PartsPlaced)
	}
	if sol.Metrics.YieldPct <= 0 || sol.Metrics.YieldPct > 100 {
		t.Fatalf("implausible yield %.1f%%", sol.Metrics.YieldPct)
	}
	if len(sol.Sheets) == 0 {
		t.Fatal("expected at least one sheet")
	}
	if len(sol.Sheets[0].CutSteps) == 0 {
		t.Fatal("expected cut instructions on the first sheet")
	}
}

func TestShelfSolverReportsOversizedPart(t *testing.T) {
	p := core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "huge", Code: "GIANT", Width: core.FromMM(3000), Height: core.FromMM(3000), Quantity: 1, AllowRotate: true},
		},
		Stocks: []core.StockItem{
			{ID: "s1", Code: "SMALL", Width: core.FromMM(1000), Height: core.FromMM(1000), Quantity: 1},
		},
		Rules: core.DefaultRules(),
	})
	sol, err := New().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if len(sol.Unplaced) != 1 || sol.Unplaced[0].PartCode != "GIANT" {
		t.Fatalf("expected GIANT to be reported unplaced, got %+v", sol.Unplaced)
	}
	if sol.Unplaced[0].Reason != "part is larger than any usable stock size" {
		t.Fatalf("unexpected reason: %s", sol.Unplaced[0].Reason)
	}
}

func TestShelfSolverIsDeterministic(t *testing.T) {
	p := sampleProblem()
	a, err := New().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Sheets) != len(b.Sheets) {
		t.Fatalf("sheet count differs between runs: %d vs %d", len(a.Sheets), len(b.Sheets))
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
