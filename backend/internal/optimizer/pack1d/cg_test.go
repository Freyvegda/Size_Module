package pack1d

import (
	"context"
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/validator"
)

// barRules matches testdata/benchmarks/bars-*.json: 3 mm kerf, 5 mm trim.
func barRules() core.Rules {
	return core.Rules{
		Kerf:            core.FromMM(3),
		Trim:            core.FromMM(5),
		AllowRotate:     false,
		GrainMode:       core.GrainNone,
		CutMode:         core.CutGuillotine,
		OffcutMinLength: core.FromMM(300),
	}
}

// homogeneousOrder: 200 identical rails on 6 m bars. Usable length is 5990 mm
// and four rails plus kerfs consume 5609 mm, so the optimal answer is 50 bars.
func homogeneousOrder() core.Problem {
	return core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "rail", Code: "RAIL-1400", Length: core.FromMM(1400), Quantity: 200, Priority: 1},
		},
		Stocks: []core.StockItem{
			{ID: "bar", Code: "BAR-6000", Length: core.FromMM(6000), Width: core.FromMM(60), Quantity: 60, CostPerUnit: 14.75},
		},
		Rules: barRules(),
	})
}

// variedOrder mirrors testdata/benchmarks/bars-varied.json, where the LP bound
// is 26 bars and the FFD baseline needs 28.
func variedOrder() core.Problem {
	return core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "a", Code: "PROFILE-2400", Length: core.FromMM(2400), Quantity: 12, Priority: 1},
			{ID: "b", Code: "PROFILE-1750", Length: core.FromMM(1750), Quantity: 18, Priority: 1},
			{ID: "c", Code: "PROFILE-1200", Length: core.FromMM(1200), Quantity: 24, Priority: 2},
			{ID: "d", Code: "PROFILE-830", Length: core.FromMM(830), Quantity: 30, Priority: 2},
			{ID: "e", Code: "PROFILE-620", Length: core.FromMM(620), Quantity: 36, Priority: 3},
			{ID: "f", Code: "PROFILE-410", Length: core.FromMM(410), Quantity: 45, Priority: 3},
		},
		Stocks: []core.StockItem{
			{ID: "bar", Code: "BAR-6000", Length: core.FromMM(6000), Width: core.FromMM(60), Quantity: 40, CostPerUnit: 14.75},
		},
		Rules: barRules(),
	})
}

func TestColumnSolverPacksHomogeneousOrderOptimally(t *testing.T) {
	p := homogeneousOrder()
	sol, err := NewColumn().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	for _, v := range validator.Validate(p, sol) {
		if v.Severity == "error" {
			t.Errorf("validation error: %s", v.Message)
		}
	}
	if sol.Metrics.PartsPlaced != 200 {
		t.Fatalf("expected all 200 rails placed, got %d", sol.Metrics.PartsPlaced)
	}
	if sol.Metrics.SheetCount != 50 {
		t.Fatalf("expected the optimal 50 bars, got %d", sol.Metrics.SheetCount)
	}
	if len(sol.Unplaced) != 0 {
		t.Fatalf("expected no unplaced pieces, got %+v", sol.Unplaced)
	}
	if len(sol.Notes) == 0 {
		t.Fatal("expected notes explaining the column generation run")
	}
}

func TestColumnSolverBeatsTheFFDBaselineOnVariedOrder(t *testing.T) {
	p := variedOrder()

	cg, err := NewColumn().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("cg solve: %v", err)
	}
	ffd, err := New().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("ffd solve: %v", err)
	}
	for _, v := range validator.Validate(p, cg) {
		if v.Severity == "error" {
			t.Errorf("cg validation error: %s", v.Message)
		}
	}

	// The LP bound for this order is 25.8 bars, so 26 is optimal.
	if cg.Metrics.SheetCount != 26 {
		t.Fatalf("expected column generation to reach 26 bars (the LP bound), got %d", cg.Metrics.SheetCount)
	}
	if cg.Metrics.SheetCount >= ffd.Metrics.SheetCount {
		t.Fatalf("expected cg (%d bars) to beat ffd (%d bars) on this order",
			cg.Metrics.SheetCount, ffd.Metrics.SheetCount)
	}
	if cg.Metrics.PartsPlaced != ffd.Metrics.PartsPlaced {
		t.Fatalf("piece counts differ: cg %d, ffd %d", cg.Metrics.PartsPlaced, ffd.Metrics.PartsPlaced)
	}
}

func TestColumnSolverRespectsStockLimits(t *testing.T) {
	p := core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "rail", Code: "RAIL-1400", Length: core.FromMM(1400), Quantity: 20},
		},
		Stocks: []core.StockItem{
			{ID: "bar", Code: "BAR-6000", Length: core.FromMM(6000), Width: core.FromMM(60), Quantity: 2},
		},
		Rules: core.DefaultRules(),
	})
	sol, err := NewColumn().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if sol.Metrics.SheetCount != 2 {
		t.Fatalf("expected the solver to use the two available bars, got %d", sol.Metrics.SheetCount)
	}
	if sol.Metrics.PartsPlaced != 8 {
		t.Fatalf("expected 8 pieces (4 per bar), got %d", sol.Metrics.PartsPlaced)
	}
	if len(sol.Unplaced) != 1 || sol.Unplaced[0].Quantity != 12 {
		t.Fatalf("expected 12 unplaced pieces, got %+v", sol.Unplaced)
	}
}

func TestColumnSolverIsDeterministic(t *testing.T) {
	p := variedOrder()
	first, err := NewColumn().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewColumn().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Sheets) != len(second.Sheets) {
		t.Fatalf("sheet counts differ: %d vs %d", len(first.Sheets), len(second.Sheets))
	}
	for i := range first.Sheets {
		a, b := first.Sheets[i], second.Sheets[i]
		if len(a.Placements) != len(b.Placements) {
			t.Fatalf("sheet %d placement counts differ", i)
		}
		for j := range a.Placements {
			if a.Placements[j] != b.Placements[j] {
				t.Fatalf("sheet %d placement %d differs: %+v vs %+v", i, j, a.Placements[j], b.Placements[j])
			}
		}
	}
}

func TestColumnSolverKeepsReusableOffcuts(t *testing.T) {
	p := core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "post", Code: "POST-2400", Length: core.FromMM(2400), Quantity: 2},
		},
		Stocks: []core.StockItem{
			{ID: "bar", Code: "BAR-6000", Length: core.FromMM(6000), Width: core.FromMM(60), Quantity: 1},
		},
		Rules: core.DefaultRules(),
	})
	sol, err := NewColumn().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if len(sol.Sheets) != 1 {
		t.Fatalf("expected a single bar, got %d", len(sol.Sheets))
	}
	if len(sol.Sheets[0].Offcuts) == 0 {
		t.Fatalf("expected a reusable offcut from a 6 m bar holding 4.8 m of posts")
	}
}
