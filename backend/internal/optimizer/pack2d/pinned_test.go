package pack2d

import (
	"context"
	"reflect"
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
)

// pinnedProblem is a 1x stock sheet 1000x1000 with one locked pane and three
// free panes; the locked one must never move.
func pinnedProblem(quantity int) core.Problem {
	return core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "locked", Code: "PANE-LOCKED", Width: core.FromMM(400), Height: core.FromMM(400), Quantity: 1, AllowRotate: true},
			{ID: "free", Code: "PANE-FREE", Width: core.FromMM(400), Height: core.FromMM(400), Quantity: 3, AllowRotate: true},
		},
		Stocks: []core.StockItem{
			{ID: "sheet", Code: "SHEET-1000x1000", Width: core.FromMM(1000), Height: core.FromMM(1000), Quantity: quantity},
		},
		Rules: core.DefaultRules(),
		Pinned: []core.PinnedSheet{{
			StockID:   "sheet",
			StockCode: "SHEET-1000x1000",
			Label:     "SHEET-1000x1000",
			Width:     core.FromMM(1000),
			Height:    core.FromMM(1000),
			Placements: []core.Placement{{
				PartID: "locked", PartCode: "PANE-LOCKED",
				X: core.FromMM(10), Y: core.FromMM(10),
				W: core.FromMM(400), H: core.FromMM(400),
			}},
		}},
	})
}

func TestPinned2DKeepsLockedPlacements(t *testing.T) {
	p := pinnedProblem(2)
	sol, err := NewPinned().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}

	var found *core.Placement
	placed := map[string]int{}
	for _, sheet := range sol.Sheets {
		for i := range sheet.Placements {
			pl := &sheet.Placements[i]
			placed[pl.PartCode]++
			if pl.PartCode == "PANE-LOCKED" {
				found = pl
			}
		}
	}
	if found == nil {
		t.Fatal("the locked pane is missing from the solution")
	}
	if found.X != core.FromMM(10) || found.Y != core.FromMM(10) ||
		found.W != core.FromMM(400) || found.H != core.FromMM(400) {
		t.Fatalf("locked pane moved: %+v", *found)
	}
	if placed["PANE-FREE"] != 3 || placed["PANE-LOCKED"] != 1 {
		t.Fatalf("pieces placed = %v, want 1 locked + 3 free", placed)
	}
	if len(sol.Unplaced) != 0 {
		t.Fatalf("unexpected unplaced demand: %+v", sol.Unplaced)
	}
}

func TestPinned2DBehavesLikeShelfWithoutPins(t *testing.T) {
	p := core.Normalize(core.Problem{
		Parts:  []core.Part{{ID: "p", Code: "PANE", Width: core.FromMM(400), Height: core.FromMM(400), Quantity: 4, AllowRotate: true}},
		Stocks: []core.StockItem{{ID: "sheet", Code: "SHEET-1000x1000", Width: core.FromMM(1000), Height: core.FromMM(1000), Quantity: 2}},
		Rules:  core.DefaultRules(),
	})
	sol, err := NewPinned().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if sol.Metrics.PartsPlaced != 4 {
		t.Fatalf("placed %d pieces, want 4", sol.Metrics.PartsPlaced)
	}
	if sol.Solver != "pinned-2d" {
		t.Fatalf("solver = %q", sol.Solver)
	}
}

func TestPinned2DIsDeterministic(t *testing.T) {
	p := pinnedProblem(2)
	first, err := NewPinned().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("first solve: %v", err)
	}
	second, err := NewPinned().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("second solve: %v", err)
	}
	if !reflect.DeepEqual(first.Sheets, second.Sheets) {
		t.Fatal("the same pinned problem produced different layouts")
	}
}
