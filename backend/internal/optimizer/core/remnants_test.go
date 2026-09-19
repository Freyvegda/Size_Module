package core

import (
	"reflect"
	"testing"
)

func stockIDs(stocks []StockItem) []string {
	out := make([]string, 0, len(stocks))
	for _, s := range stocks {
		out = append(out, s.ID)
	}
	return out
}

func TestPrioritizeRemnants(t *testing.T) {
	p := Problem{
		Rules: Rules{PreferRemnants: true},
		Stocks: []StockItem{
			{ID: "format-a"},
			{ID: "remnant-1", IsRemnant: true},
			{ID: "format-b"},
			{ID: "remnant-2", IsRemnant: true},
		},
	}
	PrioritizeRemnants(&p)

	want := []string{"remnant-1", "remnant-2", "format-a", "format-b"}
	if got := stockIDs(p.Stocks); !reflect.DeepEqual(got, want) {
		t.Fatalf("remnant-first order = %v, want %v", got, want)
	}
}

func TestPrioritizeRemnantsRespectsTheFlag(t *testing.T) {
	p := Problem{
		Rules: Rules{PreferRemnants: false},
		Stocks: []StockItem{
			{ID: "format-a"},
			{ID: "remnant-1", IsRemnant: true},
		},
	}
	PrioritizeRemnants(&p)

	want := []string{"format-a", "remnant-1"}
	if got := stockIDs(p.Stocks); !reflect.DeepEqual(got, want) {
		t.Fatalf("order changed without preferRemnants: %v", got)
	}
}

func TestNormalizeAppliesRemnantFirst(t *testing.T) {
	p := Normalize(Problem{
		Parts: []Part{{ID: "p", Code: "P", Width: 1000, Height: 1000, Quantity: 1}},
		Stocks: []StockItem{
			{ID: "sheet", Code: "SHEET", Width: 3000, Height: 2000, Quantity: 2},
			{ID: "offcut", Code: "OFFCUT", Label: "OFF-1", Width: 1000, Height: 1000, IsRemnant: true},
		},
	})
	if !p.Rules.PreferRemnants {
		t.Fatal("default rules should prefer remnants")
	}
	if p.Stocks[0].ID != "offcut" {
		t.Fatalf("expected the remnant first, got %v", stockIDs(p.Stocks))
	}
}

func TestDefaultRulesRemainBackwardsCompatible(t *testing.T) {
	r := DefaultRules()
	if r.Kerf != 4*Millimeter || r.Trim != 10*Millimeter {
		t.Fatalf("default kerf/trim changed: %+v", r)
	}
	if !r.PreferRemnants {
		t.Fatal("default rules should ask for remnant-first allocation")
	}
}

func TestRemnantSheetsAreNotChargedAsNewStock(t *testing.T) {
	p := Normalize(Problem{
		Parts: []Part{{ID: "p", Code: "P", Width: 1000, Height: 1000, Quantity: 3}},
		Stocks: []StockItem{
			{ID: "offcut", Code: "OFF-1", Label: "OFF-1", Width: 2000, Height: 2000, IsRemnant: true},
			{ID: "sheet", Code: "SHEET-2000x2000", Width: 2000, Height: 2000},
		},
	})
	placements := func(stockID string, n int) SheetPlan {
		var pls []Placement
		for i := 0; i < n; i++ {
			pls = append(pls, Placement{PartID: "p", PartCode: "P", W: 1000, H: 1000, X: FromMM(float64(10 + i*1010))})
		}
		return SheetPlan{Index: 0, StockID: stockID, Width: 2000, Height: 2000, Placements: pls}
	}
	sheets := []SheetPlan{placements("offcut", 2), placements("sheet", 1)}
	m := Summarize(p, sheets, nil, 0)
	if m.RemnantSheets != 1 {
		t.Fatalf("RemnantSheets = %d, want 1", m.RemnantSheets)
	}
	// The same layout without the remnant flag pays for two fresh sheets and
	// must score lower.
	freshOnly := m
	freshOnly.RemnantSheets = 0
	if Score(p, m) <= Score(p, freshOnly) {
		t.Fatalf("using a remnant should score higher: %v vs %v", Score(p, m), Score(p, freshOnly))
	}
}
