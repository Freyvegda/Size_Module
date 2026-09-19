package costing

import (
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
)

// costProblem has one expensive sheet and one cheap remnant, both fully able to
// hold the single pane.
func costProblem() core.Problem {
	return core.Normalize(core.Problem{
		Parts: []core.Part{{ID: "pane", Code: "PANE", Width: core.FromMM(500), Height: core.FromMM(400), Quantity: 1}},
		Stocks: []core.StockItem{
			{ID: "sheet", Code: "SHEET", Width: core.FromMM(1000), Height: core.FromMM(1000), Quantity: 1, CostPerUnit: 60},
			{ID: "remnant", Code: "OFF", Label: "OFF-1", Width: core.FromMM(1000), Height: core.FromMM(1000), Quantity: 1, CostPerUnit: 10, IsRemnant: true},
		},
	})
}

func costSolution(p core.Problem, stockID string) core.Solution {
	sheets := []core.SheetPlan{{
		Index: 0, StockID: stockID, StockCode: "S",
		Width: core.FromMM(1000), Height: core.FromMM(1000),
		Placements: []core.Placement{{
			PartID: "pane", PartCode: "PANE",
			X: core.FromMM(10), Y: core.FromMM(10), W: core.FromMM(500), H: core.FromMM(400),
		}},
		Offcuts: []core.Rect{{X: core.FromMM(514), Y: core.FromMM(10), W: core.FromMM(486), H: core.FromMM(400)}},
	}}
	sol := core.Solution{Sheets: sheets}
	sol.Metrics = core.Summarize(p, sheets, nil, 0)
	sol.Metrics.TrimAreaM2 = 0
	sol.Metrics.KerfAreaM2 = 0
	return sol
}

func TestBreakdownSplitsNewMaterialAndRemnants(t *testing.T) {
	p := costProblem()
	sol := costSolution(p, "sheet")
	report := Breakdown(p, sol)

	if report.NewMaterialCost != 60 || report.RemnantCost != 0 || report.TotalStockCost != 60 {
		t.Fatalf("new material = %+v, want 60 new / 0 remnant", report)
	}
	// Part 0.2 m² of 1 m² -> 12; offcut 0.1944 m² -> 11.664; the rest is scrap.
	if report.PartCost != 12 {
		t.Fatalf("part cost = %v, want 12", report.PartCost)
	}
	if report.NetCost != report.TotalStockCost-report.OffcutCredit {
		t.Fatalf("net cost %v != total %v - offcut %v", report.NetCost, report.TotalStockCost, report.OffcutCredit)
	}
	if report.CostPerPart != report.NetCost {
		t.Fatalf("one part placed: cost per part %v != net %v", report.CostPerPart, report.NetCost)
	}
}

func TestBreakdownClassifiesRemnantSheets(t *testing.T) {
	p := costProblem()
	report := Breakdown(p, costSolution(p, "remnant"))
	if report.RemnantCost != 10 || report.NewMaterialCost != 0 || report.TotalStockCost != 10 {
		t.Fatalf("remnant plan = %+v, want 0 new / 10 remnant", report)
	}
}

func TestBreakdownWithoutCostsIsZero(t *testing.T) {
	p := core.Normalize(core.Problem{
		Parts:  []core.Part{{ID: "p", Code: "PANE", Width: core.FromMM(500), Height: core.FromMM(400), Quantity: 1}},
		Stocks: []core.StockItem{{ID: "s", Code: "SHEET", Width: core.FromMM(1000), Height: core.FromMM(1000), Quantity: 1}},
	})
	report := Breakdown(p, costSolution(p, "s"))
	if report.TotalStockCost != 0 || report.NetCost != 0 || report.CostPerPart != 0 {
		t.Fatalf("expected an empty report, got %+v", report)
	}
}

func TestBreakdownIsDeterministic(t *testing.T) {
	p := costProblem()
	first := Breakdown(p, costSolution(p, "sheet"))
	second := Breakdown(p, costSolution(p, "sheet"))
	if first != second {
		t.Fatalf("reports differ: %+v vs %+v", first, second)
	}
}
