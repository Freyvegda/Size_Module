package pack1d

import (
	"context"
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/validator"
)

func TestFFDPacksBarsAndKeepsOffcut(t *testing.T) {
	p := core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "a", Code: "RAIL-LONG", Length: core.FromMM(2400), Quantity: 3},
			{ID: "b", Code: "RAIL-SHORT", Length: core.FromMM(1100), Quantity: 4},
			{ID: "c", Code: "SPACER", Length: core.FromMM(450), Quantity: 5},
		},
		Stocks: []core.StockItem{
			{ID: "bar", Code: "TUBE-6000", Length: core.FromMM(6000), Quantity: 3},
		},
		Rules: core.DefaultRules(),
	})
	sol, err := New().Solve(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	violations := validator.Validate(p, sol)
	for _, v := range violations {
		if v.Severity == "error" {
			t.Errorf("validation error: %s", v.Message)
		}
	}
	if sol.Metrics.PartsPlaced != 12 {
		t.Fatalf("expected 12 pieces placed, got %d", sol.Metrics.PartsPlaced)
	}
	if sol.Metrics.StockLengthM != 18 {
		t.Fatalf("expected 18 m of stock, got %.2f", sol.Metrics.StockLengthM)
	}
	if sol.Metrics.YieldPct < 50 {
		t.Fatalf("suspiciously low yield %.1f%%", sol.Metrics.YieldPct)
	}
}
