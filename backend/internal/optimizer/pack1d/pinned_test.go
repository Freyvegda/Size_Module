package pack1d

import (
	"context"
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
)

func pinnedBarProblem() core.Problem {
	return core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "locked-a", Code: "RAIL-1000-A", Length: core.FromMM(1000), Quantity: 1},
			{ID: "locked-b", Code: "RAIL-1000-B", Length: core.FromMM(1000), Quantity: 1},
			{ID: "free", Code: "RAIL-900", Length: core.FromMM(900), Quantity: 2},
		},
		Stocks: []core.StockItem{
			{ID: "bar", Code: "BAR-6000", Length: core.FromMM(6000), Width: core.FromMM(60), Quantity: 2},
		},
		Rules: core.DefaultRules(),
		Pinned: []core.PinnedSheet{{
			StockID:   "bar",
			StockCode: "BAR-6000",
			Label:     "OFF-DEMO-BAR",
			Width:     core.FromMM(6000),
			Height:    core.FromMM(60),
			Placements: []core.Placement{
				{PartID: "locked-a", PartCode: "RAIL-1000-A", X: core.FromMM(20), Y: 0, W: core.FromMM(1000), H: core.FromMM(60)},
				{PartID: "locked-b", PartCode: "RAIL-1000-B", X: core.FromMM(2500), Y: 0, W: core.FromMM(1000), H: core.FromMM(60)},
			},
		}},
	})
}

func TestPinned1DKeepsLockedPlacements(t *testing.T) {
	p := pinnedBarProblem()
	sol, err := NewPinned().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}

	positions := map[string]core.Dim{}
	placed := map[string]int{}
	for _, sheet := range sol.Sheets {
		for _, pl := range sheet.Placements {
			positions[pl.PartCode] = pl.X
			placed[pl.PartCode]++
		}
	}
	if positions["RAIL-1000-A"] != core.FromMM(20) || positions["RAIL-1000-B"] != core.FromMM(2500) {
		t.Fatalf("locked rails moved: %v", positions)
	}
	if placed["RAIL-1000-A"] != 1 || placed["RAIL-1000-B"] != 1 || placed["RAIL-900"] != 2 {
		t.Fatalf("pieces placed = %v", placed)
	}
	if len(sol.Unplaced) != 0 {
		t.Fatalf("unexpected unplaced demand: %+v", sol.Unplaced)
	}
	if sol.Solver != "pinned-1d" {
		t.Fatalf("solver = %q", sol.Solver)
	}
}

func TestPinned1DFillsTheGaps(t *testing.T) {
	p := pinnedBarProblem()
	sol, err := NewPinned().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	// The pinned bar has a 70 mm gap between the two rails and a long tail
	// after them; the 900 mm pieces must land on the pinned bar.
	pinnedBar := sol.Sheets[0]
	if pinnedBar.Label != "OFF-DEMO-BAR" {
		t.Fatalf("expected the pinned bar first, got %q", pinnedBar.Label)
	}
	if len(pinnedBar.Placements) != 4 {
		t.Fatalf("pinned bar holds %d placements, want 4 (2 locked + 2 filled)", len(pinnedBar.Placements))
	}
}
