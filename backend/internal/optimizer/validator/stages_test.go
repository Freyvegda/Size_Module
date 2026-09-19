package validator

import (
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
)

func TestMaxCutStagesIsEnforced(t *testing.T) {
	half := core.FromMM(500)
	parts := []core.Part{
		{ID: "A", Code: "A", Width: half, Height: half, Quantity: 1},
		{ID: "B", Code: "B", Width: half, Height: half, Quantity: 1},
		{ID: "C", Code: "C", Width: half, Height: half, Quantity: 1},
		{ID: "D", Code: "D", Width: half, Height: half, Quantity: 1},
	}
	stock := core.StockItem{ID: "s", Code: "SHEET-1000", Width: core.FromMM(1000), Height: core.FromMM(1000), Quantity: 1}
	sheet := core.SheetPlan{
		Index: 0, StockID: "s", Width: core.FromMM(1000), Height: core.FromMM(1000),
		Placements: []core.Placement{
			{PartID: "A", PartCode: "A", X: 0, Y: 0, W: half, H: half},
			{PartID: "B", PartCode: "B", X: 0, Y: half, W: half, H: half},
			{PartID: "C", PartCode: "C", X: half, Y: 0, W: half, H: half},
			{PartID: "D", PartCode: "D", X: half, Y: half, W: half, H: half},
		},
	}
	solution := core.Solution{Sheets: []core.SheetPlan{sheet}}

	problem := core.Normalize(core.Problem{
		Parts:  parts,
		Stocks: []core.StockItem{stock},
		Rules:  core.Rules{Kerf: 0, Trim: 0, AllowRotate: true, CutMode: core.CutGuillotine},
	})
	if codes(Validate(problem, solution))["too_many_stages"] {
		t.Fatal("unlimited stages must not raise too_many_stages")
	}

	limited := problem
	limited.Rules.MaxCutStages = 1
	if !codes(Validate(limited, solution))["too_many_stages"] {
		t.Fatal("expected too_many_stages with maxCutStages=1")
	}

	twoStages := problem
	twoStages.Rules.MaxCutStages = 2
	if codes(Validate(twoStages, solution))["too_many_stages"] {
		t.Fatal("a 2-stage shelf layout must satisfy maxCutStages=2")
	}
}
