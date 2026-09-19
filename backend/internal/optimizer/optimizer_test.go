package optimizer

import (
	"context"
	"strings"
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
)

func sheetProblem() core.Problem {
	return core.Normalize(core.Problem{
		Parts:  []core.Part{{ID: "p", Code: "PANE", Width: core.FromMM(500), Height: core.FromMM(400), Quantity: 2}},
		Stocks: []core.StockItem{{ID: "s", Code: "SHEET", Width: core.FromMM(2000), Height: core.FromMM(1000), Quantity: 1}},
	})
}

func barProblem() core.Problem {
	return core.Normalize(core.Problem{
		Parts:  []core.Part{{ID: "b", Code: "BAR-PART", Length: core.FromMM(1200), Quantity: 3}},
		Stocks: []core.StockItem{{ID: "s", Code: "BAR", Length: core.FromMM(6000), Width: core.FromMM(50), Quantity: 2}},
	})
}

func TestSolveRejectsSolverForTheWrongProfile(t *testing.T) {
	reg := DefaultRegistry()

	_, err := Solve(context.Background(), sheetProblem(), "cg-1d", reg, nil)
	if err == nil {
		t.Fatal("expected an error when a 1d solver is named for a 2d problem")
	}
	if !strings.Contains(err.Error(), "handles 1d problems") {
		t.Fatalf("unexpected error message: %v", err)
	}

	_, err = Solve(context.Background(), barProblem(), "beam-2d", reg, nil)
	if err == nil {
		t.Fatal("expected an error when a 2d solver is named for a 1d problem")
	}
}

func TestSolvePicksAPortfolioPerProfile(t *testing.T) {
	reg := DefaultRegistry()

	sheetResult, err := Solve(context.Background(), sheetProblem(), "", reg, nil)
	if err != nil {
		t.Fatalf("2d solve: %v", err)
	}
	if sheetResult.Solution.Solver != "best-2d" {
		t.Fatalf("expected the 2d portfolio, got %s", sheetResult.Solution.Solver)
	}

	barResult, err := Solve(context.Background(), barProblem(), "", reg, nil)
	if err != nil {
		t.Fatalf("1d solve: %v", err)
	}
	if barResult.Solution.Solver != "best-1d" {
		t.Fatalf("expected the 1d portfolio, got %s", barResult.Solution.Solver)
	}
}

func TestSolveRejectsUnknownSolver(t *testing.T) {
	reg := DefaultRegistry()
	if _, err := Solve(context.Background(), sheetProblem(), "nope-2d", reg, nil); err == nil {
		t.Fatal("expected an error for an unknown solver name")
	}
}
