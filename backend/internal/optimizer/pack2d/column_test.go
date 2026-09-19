package pack2d

import (
	"context"
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/validator"
)

func columnProblem() core.Problem {
	return core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "a", Code: "PANE-1200x1400", Width: core.FromMM(1200), Height: core.FromMM(1400), Quantity: 6, AllowRotate: true, Priority: 1},
			{ID: "b", Code: "PANE-800x1000", Width: core.FromMM(800), Height: core.FromMM(1000), Quantity: 6, AllowRotate: true, Priority: 1},
			{ID: "c", Code: "SHELF-500x250", Width: core.FromMM(500), Height: core.FromMM(250), Quantity: 10, AllowRotate: true, Priority: 2},
		},
		Stocks: []core.StockItem{
			{ID: "s1", Code: "SHEET-3210x2250", Width: core.FromMM(3210), Height: core.FromMM(2250), Quantity: 6, CostPerUnit: 62.5},
			{ID: "s2", Code: "SHEET-2440x1220", Width: core.FromMM(2440), Height: core.FromMM(1220), Quantity: 4, CostPerUnit: 28},
		},
		Rules: core.DefaultRules(),
	})
}

// homogeneousSheetOrder: 200 identical panes that tile perfectly as 5 pieces
// per strip and 5 strips per sheet, so the optimal answer is 8 sheets.
func homogeneousSheetOrder() core.Problem {
	return core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "pane", Code: "PANE-600x400", Width: core.FromMM(600), Height: core.FromMM(400), Quantity: 200, AllowRotate: true, Priority: 1},
		},
		Stocks: []core.StockItem{
			{ID: "s", Code: "SHEET-3210x2250", Width: core.FromMM(3210), Height: core.FromMM(2250), Quantity: 40, CostPerUnit: 62.5},
		},
		Rules: core.DefaultRules(),
	})
}

func TestColumnSolverProducesValidPlan(t *testing.T) {
	p := columnProblem()
	p.BudgetMS = 400
	sol, err := NewColumn().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	for _, v := range validator.Validate(p, sol) {
		if v.Severity == "error" {
			t.Errorf("validation error: %s", v.Message)
		}
	}
	if sol.Metrics.PartsPlaced != 22 {
		t.Fatalf("expected 22 pieces placed, got %d", sol.Metrics.PartsPlaced)
	}
	if sol.Solver != "cg-2d" {
		t.Fatalf("unexpected solver %q", sol.Solver)
	}
	notes := ""
	for _, note := range sol.Notes {
		notes += note + "\n"
	}
	if notes == "" {
		t.Fatal("expected notes describing the column generation run")
	}
}

func TestColumnSolverPacksHomogeneousOrderOptimally(t *testing.T) {
	p := homogeneousSheetOrder()
	p.BudgetMS = 500
	sol, err := NewColumn().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	for _, v := range validator.Validate(p, sol) {
		if v.Severity == "error" {
			t.Errorf("validation error: %s", v.Message)
		}
	}
	if sol.Metrics.PartsPlaced != 200 {
		t.Fatalf("expected all 200 panes placed, got %d", sol.Metrics.PartsPlaced)
	}
	// 25 panes per sheet (5 strips of 5) is the best possible: 200/25 = 8.
	if sol.Metrics.SheetCount != 8 {
		t.Fatalf("expected the optimal 8 sheets, got %d", sol.Metrics.SheetCount)
	}
}

func TestColumnSolverIsDeterministic(t *testing.T) {
	p := columnProblem()
	p.BudgetMS = 400
	first, err := NewColumn().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewColumn().Solve(context.Background(), p, nil)
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

func TestColumnSolverRespectsStockLimits(t *testing.T) {
	p := homogeneousSheetOrder()
	p.Stocks[0].Quantity = 3 // only three sheets exist
	p.BudgetMS = 400
	sol, err := NewColumn().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if sol.Metrics.SheetCount > 3 {
		t.Fatalf("planned %d sheets but only 3 exist", sol.Metrics.SheetCount)
	}
	if sol.Metrics.PartsPlaced >= 200 {
		t.Fatalf("expected unmet demand with only three sheets, got %d placed", sol.Metrics.PartsPlaced)
	}
	if len(sol.Unplaced) == 0 {
		t.Fatal("expected unplaced pieces to be reported")
	}
}

func TestColumnSolverKeepsReusableOffcuts(t *testing.T) {
	p := core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "a", Code: "PANE-900x700", Width: core.FromMM(900), Height: core.FromMM(700), Quantity: 4, AllowRotate: true},
		},
		Stocks: []core.StockItem{
			{ID: "s", Code: "SHEET-3210x2250", Width: core.FromMM(3210), Height: core.FromMM(2250), Quantity: 1},
		},
		Rules: core.DefaultRules(),
	})
	p.BudgetMS = 300
	sol, err := NewColumn().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if len(sol.Sheets) != 1 {
		t.Fatalf("expected one sheet, got %d", len(sol.Sheets))
	}
	if len(sol.Sheets[0].Offcuts) == 0 {
		t.Fatal("expected reusable offcuts from a mostly empty sheet")
	}
	if sol.Metrics.OffcutAreaM2 <= 0 || sol.Metrics.OffcutAreaM2 > sol.Metrics.StockAreaM2 {
		t.Fatalf("implausible offcut area %.2f", sol.Metrics.OffcutAreaM2)
	}
}
