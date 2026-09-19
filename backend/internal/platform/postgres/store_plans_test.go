package postgres

import (
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/db"
)

func TestOffcutLabel(t *testing.T) {
	got := offcutLabel("12345678-aaaa-bbbb-cccc-ddddeeeeffff", 0, 0)
	if want := "OFF-12345678-01-1"; got != want {
		t.Fatalf("offcutLabel = %q, want %q", got, want)
	}
	got = offcutLabel("abcdef01-0000-0000-0000-000000000000", 11, 2)
	if want := "OFF-ABCDEF01-12-3"; got != want {
		t.Fatalf("offcutLabel = %q, want %q", got, want)
	}
}

func TestOffcutReusable(t *testing.T) {
	rules := core.DefaultRules() // offcuts from 300 x 300 mm / 300 mm

	min := core.FromMM(300)
	cases := []struct {
		name string
		off  core.Rect
		is1D bool
		want bool
	}{
		{"2d exactly at the minimum", core.Rect{W: min, H: min}, false, true},
		{"2d one micron short", core.Rect{W: min - 1, H: min}, false, false},
		{"2d too shallow", core.Rect{W: core.FromMM(1200), H: min - 1}, false, false},
		{"1d exactly at the minimum", core.Rect{W: min, H: core.FromMM(60)}, true, true},
		{"1d too short", core.Rect{W: min - 1, H: core.FromMM(60)}, true, false},
		{"degenerate", core.Rect{W: 0, H: min}, false, false},
	}
	for _, tc := range cases {
		if got := offcutReusable(tc.off, tc.is1D, rules); got != tc.want {
			t.Errorf("%s: offcutReusable = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRemnantDimsAndCost(t *testing.T) {
	sheetFormat := &db.StockFormat{
		WidthUm: core.FromMM(2440), HeightUm: core.FromMM(1220), CostPerUnit: 28,
	}
	barFormat := &db.StockFormat{
		LengthUm: core.FromMM(6000), WidthUm: core.FromMM(60), CostPerUnit: 15,
	}

	// 2D: half the sheet is half the cost.
	off := core.Rect{W: core.FromMM(1220), H: core.FromMM(1220)}
	if got := remnantCost(off, false, sheetFormat); got != 14 {
		t.Fatalf("2d remnant cost = %v, want 14", got)
	}
	shape := remnantDims(off, false, sheetFormat)
	if shape.width != off.W || shape.height != off.H || shape.length != 0 {
		t.Fatalf("2d remnant dims = %+v", shape)
	}

	// 1D: a quarter of the bar is a quarter of the cost, and the bar width is
	// kept for display.
	off = core.Rect{W: core.FromMM(1500), H: core.FromMM(60)}
	if got := remnantCost(off, true, barFormat); got != 3.75 {
		t.Fatalf("1d remnant cost = %v, want 3.75", got)
	}
	shape = remnantDims(off, true, barFormat)
	if shape.length != off.W || shape.width != core.FromMM(60) || shape.height != 0 {
		t.Fatalf("1d remnant dims = %+v", shape)
	}

	// No format lineage: the piece is still created, it just carries no cost.
	if got := remnantCost(off, true, nil); got != 0 {
		t.Fatalf("cost without a format = %v, want 0", got)
	}
}

func TestSheetIs1D(t *testing.T) {
	stocks := map[string]core.StockItem{
		"bar":   {ID: "bar", Length: core.FromMM(6000), Width: core.FromMM(60)},
		"sheet": {ID: "sheet", Width: core.FromMM(2440), Height: core.FromMM(1220)},
	}
	bar := db.PlanSheet{StockID: "bar", WidthUm: core.FromMM(6000), HeightUm: core.FromMM(60)}
	if !sheetIs1D(bar, stocks, "ffd-1d") {
		t.Fatal("a stock with a Length should be 1d")
	}
	sheet := db.PlanSheet{StockID: "sheet", WidthUm: core.FromMM(2440), HeightUm: core.FromMM(1220)}
	if sheetIs1D(sheet, stocks, "shelf-2d") {
		t.Fatal("a width x height stock should be 2d")
	}
	// Without a problem snapshot the solver name is the best hint.
	orphanBar := db.PlanSheet{WidthUm: core.FromMM(6000), HeightUm: core.FromMM(60)}
	if !sheetIs1D(orphanBar, nil, "best-1d") {
		t.Fatal("a plan solved by a 1d solver should be 1d")
	}
	if sheetIs1D(orphanBar, nil, "shelf-2d") {
		t.Fatal("a 2d solver's sheets are 2d even when shallow")
	}
	// The shape fallback only kicks in when not even the solver is known.
	shallow := db.PlanSheet{WidthUm: core.FromMM(6000), HeightUm: core.Millimeter}
	if !sheetIs1D(shallow, nil, "") {
		t.Fatal("a shallow wide sheet should fall back to 1d")
	}
}
