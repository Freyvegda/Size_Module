package optimizer

import (
	"context"
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
)

func pinnedSheetProblem() core.Problem {
	p := sheetProblem()
	p.Pinned = []core.PinnedSheet{{
		StockID:   "s",
		StockCode: "SHEET",
		Width:     core.FromMM(2000),
		Height:    core.FromMM(1000),
		Placements: []core.Placement{{
			PartID: "p", PartCode: "PANE",
			X: core.FromMM(10), Y: core.FromMM(10),
			W: core.FromMM(500), H: core.FromMM(400),
		}},
	}}
	return p
}

func TestSolvePinsNeedAPinnedSolver(t *testing.T) {
	reg := DefaultRegistry()

	if _, err := Solve(context.Background(), pinnedSheetProblem(), "shelf-2d", reg, nil); err == nil {
		t.Fatal("expected shelf-2d to refuse a problem with pinned placements")
	}

	result, err := Solve(context.Background(), pinnedSheetProblem(), "", reg, nil)
	if err != nil {
		t.Fatalf("auto solve: %v", err)
	}
	if result.Solution.Solver != "pinned-2d" {
		t.Fatalf("expected pinned-2d, got %s", result.Solution.Solver)
	}
	for _, violation := range result.Violations {
		if violation.Severity == "error" {
			t.Fatalf("pinned solution is invalid: %+v", violation)
		}
	}
	if result.Solution.Metrics.PartsPlaced != 2 {
		t.Fatalf("placed %d pieces, want 2", result.Solution.Metrics.PartsPlaced)
	}
}

func TestSolvePinsForBars(t *testing.T) {
	reg := DefaultRegistry()
	p := barProblem()
	p.Pinned = []core.PinnedSheet{{
		StockID:   "s",
		StockCode: "BAR",
		Width:     core.FromMM(6000),
		Height:    core.FromMM(50),
		Placements: []core.Placement{{
			PartID: "b", PartCode: "BAR-PART",
			X: core.FromMM(100), Y: 0,
			W: core.FromMM(1200), H: core.FromMM(50),
		}},
	}}
	result, err := Solve(context.Background(), p, "", reg, nil)
	if err != nil {
		t.Fatalf("auto solve: %v", err)
	}
	if result.Solution.Solver != "pinned-1d" {
		t.Fatalf("expected pinned-1d, got %s", result.Solution.Solver)
	}
	if result.Solution.Metrics.PartsPlaced != 3 {
		t.Fatalf("placed %d pieces, want 3", result.Solution.Metrics.PartsPlaced)
	}
}
