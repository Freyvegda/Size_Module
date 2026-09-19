package campaigns

import (
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
)

func budgetProblem() []core.StockItem {
	return []core.StockItem{
		{ID: "sheet", Code: "SHEET-1000x1000", Width: core.FromMM(1000), Height: core.FromMM(1000), Quantity: 2, CostPerUnit: 60},
		{ID: "remnant", Code: "OFF-1", Label: "OFF-1", Width: core.FromMM(500), Height: core.FromMM(500), Quantity: 1, CostPerUnit: 10, IsRemnant: true},
	}
}

func budgetSolution() core.Solution {
	return core.Solution{
		Sheets: []core.SheetPlan{
			{
				Index: 0, StockID: "sheet", StockCode: "SHEET-1000x1000",
				Width: core.FromMM(1000), Height: core.FromMM(1000),
				Placements: []core.Placement{{
					PartID: "p", PartCode: "PANE", X: 0, Y: 0, W: core.FromMM(500), H: core.FromMM(500),
				}},
				Offcuts: []core.Rect{{X: core.FromMM(500), Y: 0, W: core.FromMM(500), H: core.FromMM(500)}},
			},
			{
				Index: 1, StockID: "remnant", StockCode: "OFF-1", Label: "OFF-1",
				Width: core.FromMM(500), Height: core.FromMM(500),
				Placements: []core.Placement{{
					PartID: "p", PartCode: "PANE", X: 0, Y: 0, W: core.FromMM(200), H: core.FromMM(200),
				}},
				Offcuts: []core.Rect{{X: core.FromMM(200), Y: 0, W: core.FromMM(300), H: core.FromMM(500)}},
			},
		},
	}
}

func TestConsumeBudgetKeepsLeftoversInThePool(t *testing.T) {
	rules := core.DefaultRules() // offcuts from 300 x 300 mm
	labels := []string{}
	stock := ConsumeBudget(budgetProblem(), budgetSolution(), rules, func(sheetIndex, offcutIndex int) string {
		label := RemnantLabel("abcdef01-0000-0000-0000-000000000000", 2, sheetIndex, offcutIndex)
		labels = append(labels, label)
		return label
	})

	// The sheet drops from 2 to 1; the remnant is consumed entirely.
	var sheet, remnant *core.StockItem
	var created []core.StockItem
	for i := range stock {
		switch stock[i].ID {
		case "sheet":
			sheet = &stock[i]
		case "remnant":
			remnant = &stock[i]
		default:
			created = append(created, stock[i])
		}
	}
	if sheet == nil || sheet.Quantity != 1 {
		t.Fatalf("sheet budget = %+v, want quantity 1", sheet)
	}
	if remnant != nil {
		t.Fatalf("the consumed remnant must leave the budget: %+v", remnant)
	}
	if len(created) != 2 {
		t.Fatalf("expected two new remnants, got %+v", created)
	}
	for _, remnant := range created {
		if !remnant.IsRemnant || remnant.Quantity != 1 || remnant.Label == "" || remnant.ID == "" {
			t.Fatalf("bad remnant %+v", remnant)
		}
	}
	if created[0].Width != core.FromMM(500) || created[0].Height != core.FromMM(500) {
		t.Fatalf("first remnant dims = %+v", created[0])
	}
	// Half the sheet (0.25 m² of 1 m²) is worth 15; the bar remnant's 300 mm of
	// 500 mm width share is prorated from its 10 cost.
	if created[0].CostPerUnit != 15 {
		t.Fatalf("first remnant cost = %v, want 15", created[0].CostPerUnit)
	}
	if created[1].CostPerUnit != 6 {
		t.Fatalf("second remnant cost = %v, want 6", created[1].CostPerUnit)
	}
	if len(labels) != 2 || labels[0] != "CMP-ABCDEF01-02-01-1" {
		t.Fatalf("labels = %v", labels)
	}
}

func TestConsumeBudgetSkipsScrapOffcuts(t *testing.T) {
	rules := core.DefaultRules()
	sol := budgetSolution()
	sol.Sheets[0].Offcuts = []core.Rect{{X: 0, Y: 0, W: core.FromMM(100), H: core.FromMM(100)}}
	sol.Sheets[1].Offcuts = nil
	stock := ConsumeBudget(budgetProblem(), sol, rules, func(int, int) string { return "X" })
	if len(stock) != 1 || stock[0].ID != "sheet" || stock[0].Quantity != 1 {
		t.Fatalf("scrap must not enter the budget: %+v", stock)
	}
}

func TestConsumeBudgetOneDimRemnants(t *testing.T) {
	rules := core.DefaultRules()
	stock := []core.StockItem{
		{ID: "bar", Code: "BAR-6000", Length: core.FromMM(6000), Width: core.FromMM(60), Quantity: 1, CostPerUnit: 15},
	}
	sol := core.Solution{Sheets: []core.SheetPlan{{
		Index: 0, StockID: "bar", StockCode: "BAR-6000",
		Width: core.FromMM(6000), Height: core.FromMM(60),
		Placements: []core.Placement{{PartID: "p", PartCode: "RAIL", X: 0, Y: 0, W: core.FromMM(1000), H: core.FromMM(60)}},
		Offcuts:    []core.Rect{{X: core.FromMM(1004), Y: 0, W: core.FromMM(1500), H: core.FromMM(60)}},
	}}}
	out := ConsumeBudget(stock, sol, rules, func(int, int) string { return "OFF" })
	if len(out) != 1 {
		t.Fatalf("expected only the new bar remnant, got %+v", out)
	}
	got := out[0]
	if got.Length != core.FromMM(1500) || got.Width != core.FromMM(60) || got.Height != 0 {
		t.Fatalf("bar remnant shape = %+v", got)
	}
	if got.CostPerUnit != 3.75 {
		t.Fatalf("bar remnant cost = %v, want 3.75", got.CostPerUnit)
	}
}

func TestBuildItemProblemNormalizes(t *testing.T) {
	campaign := Campaign{
		BudgetMS: 2000,
		Seed:     40,
		Stock:    budgetProblem(),
	}
	item := Item{Seq: 2, Parts: []core.Part{{ID: "p", Code: "PANE", Width: core.FromMM(500), Height: core.FromMM(500), Quantity: 1}}}
	p, err := BuildItemProblem(campaign, item, 0)
	if err != nil {
		t.Fatalf("build item problem: %v", err)
	}
	if p.Rules.Kerf != core.DefaultRules().Kerf {
		t.Fatalf("rules were not defaulted: %+v", p.Rules)
	}
	if p.Seed != 42 {
		t.Fatalf("seed = %d, want campaign seed + item seq", p.Seed)
	}
	if p.BudgetMS != 2000 {
		t.Fatalf("budget = %d, want the campaign budget", p.BudgetMS)
	}
}
