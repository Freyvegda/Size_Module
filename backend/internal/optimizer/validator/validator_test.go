package validator

import (
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
)

// pinwheel builds the classic four-piece arrangement that has no edge-to-edge
// cut: A and C are horizontal, B and D vertical, interlocked.
func pinwheel(cutMode core.CutMode) (core.Problem, core.Solution) {
	parts := []core.Part{
		{ID: "a", Code: "A", Width: core.FromMM(600), Height: core.FromMM(400), Quantity: 1},
		{ID: "b", Code: "B", Width: core.FromMM(400), Height: core.FromMM(600), Quantity: 1},
		{ID: "c", Code: "C", Width: core.FromMM(600), Height: core.FromMM(400), Quantity: 1},
		{ID: "d", Code: "D", Width: core.FromMM(400), Height: core.FromMM(600), Quantity: 1},
	}
	stocks := []core.StockItem{
		{ID: "s", Code: "SHEET-1000x1000", Width: core.FromMM(1000), Height: core.FromMM(1000), Quantity: 1},
	}
	rules := core.Rules{
		Kerf:        0, // no kerf: the pinwheel tiles the sheet exactly
		Trim:        0,
		AllowRotate: true,
		GrainMode:   core.GrainNone,
		CutMode:     cutMode,
	}
	p := core.Normalize(core.Problem{Parts: parts, Stocks: stocks, Rules: rules})

	sheet := core.SheetPlan{
		Index: 0, StockID: "s", StockCode: "SHEET-1000x1000",
		Width: core.FromMM(1000), Height: core.FromMM(1000),
		Placements: []core.Placement{
			{PartID: "a", PartCode: "A", X: 0, Y: 0, W: core.FromMM(600), H: core.FromMM(400)},
			{PartID: "b", PartCode: "B", X: core.FromMM(600), Y: 0, W: core.FromMM(400), H: core.FromMM(600)},
			{PartID: "c", PartCode: "C", X: core.FromMM(400), Y: core.FromMM(600), W: core.FromMM(600), H: core.FromMM(400)},
			{PartID: "d", PartCode: "D", X: 0, Y: core.FromMM(400), W: core.FromMM(400), H: core.FromMM(600)},
		},
	}
	solution := core.Solution{Solver: "hand-built", Sheets: []core.SheetPlan{sheet}}
	solution.Metrics = core.Summarize(p, solution.Sheets, nil, 0)
	return p, solution
}

func codes(violations []Violation) map[string]bool {
	out := map[string]bool{}
	for _, v := range violations {
		out[v.Code] = true
	}
	return out
}

func TestPinwheelIsRejectedForGuillotine(t *testing.T) {
	p, sol := pinwheel(core.CutGuillotine)
	found := codes(Validate(p, sol))
	if !found["not_guillotine"] {
		t.Fatalf("expected a not_guillotine violation, got %v", found)
	}
}

func TestPinwheelIsAcceptedForFreeCutting(t *testing.T) {
	p, sol := pinwheel(core.CutFree)
	found := codes(Validate(p, sol))
	if found["not_guillotine"] {
		t.Fatal("a free-cutting order must not require a guillotine sequence")
	}
	for code := range found {
		if code != "unplaced_parts" && code != "missing_unplaced_report" {
			t.Fatalf("unexpected violation %q", code)
		}
	}
}

func TestOverlapIsAlwaysRejected(t *testing.T) {
	p, sol := pinwheel(core.CutFree)
	// Move piece D onto piece A: the overlap must be reported even in free mode.
	sol.Sheets[0].Placements[3].X = core.FromMM(10)
	sol.Sheets[0].Placements[3].Y = core.FromMM(10)
	found := codes(Validate(p, sol))
	if !found["overlap"] {
		t.Fatalf("expected an overlap violation, got %v", found)
	}
}

func TestDefectOverlapIsRejected(t *testing.T) {
	parts := []core.Part{{ID: "a", Code: "A", Width: core.FromMM(400), Height: core.FromMM(400), Quantity: 1}}
	stocks := []core.StockItem{{
		ID: "s", Code: "REMNANT", Width: core.FromMM(1000), Height: core.FromMM(1000), Quantity: 1,
		Defects: []core.Rect{{X: core.FromMM(100), Y: core.FromMM(100), W: core.FromMM(300), H: core.FromMM(300)}},
	}}
	rules := core.Rules{Kerf: 0, Trim: 0, AllowRotate: true, CutMode: core.CutFree}
	p := core.Normalize(core.Problem{Parts: parts, Stocks: stocks, Rules: rules})

	overlapping := core.Solution{Sheets: []core.SheetPlan{{
		Index: 0, StockID: "s", Width: core.FromMM(1000), Height: core.FromMM(1000),
		Placements: []core.Placement{{PartID: "a", PartCode: "A", X: core.FromMM(150), Y: core.FromMM(150), W: core.FromMM(400), H: core.FromMM(400)}},
	}}}
	if !codes(Validate(p, overlapping))["defect_overlap"] {
		t.Fatalf("expected defect_overlap when a piece covers a defect")
	}

	clear := core.Solution{Sheets: []core.SheetPlan{{
		Index: 0, StockID: "s", Width: core.FromMM(1000), Height: core.FromMM(1000),
		Placements: []core.Placement{{PartID: "a", PartCode: "A", X: core.FromMM(500), Y: core.FromMM(500), W: core.FromMM(400), H: core.FromMM(400)}},
	}}}
	if codes(Validate(p, clear))["defect_overlap"] {
		t.Fatalf("did not expect defect_overlap for a piece clear of the defect")
	}
}
