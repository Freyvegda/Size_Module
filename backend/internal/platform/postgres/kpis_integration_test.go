package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/size-module/backend/internal/modules/jobs"
	"github.com/size-module/backend/internal/optimizer/core"
)

// TestKPIsReportAcceptedPlans checks that the dashboard aggregation sees a
// freshly accepted plan and that the realized bucket is coherent.
func TestKPIsReportAcceptedPlans(t *testing.T) {
	store := queueTestStore(t)
	ctx := context.Background()
	formatID, formatCode, width, height, cost := demoSheetFormat(t, store)

	part := core.Part{ID: "kpi-pane", Code: "KPI-PANE-600x400", Width: core.FromMM(600), Height: core.FromMM(400), Quantity: 1, AllowRotate: true}
	stock := core.StockItem{ID: formatID, Code: formatCode, Width: width, Height: height, Quantity: 1, CostPerUnit: cost}
	problem := core.Normalize(core.Problem{Parts: []core.Part{part}, Stocks: []core.StockItem{stock}, Rules: core.DefaultRules()})
	offcut := core.Rect{X: core.FromMM(700), Y: core.FromMM(10), W: core.FromMM(900), H: core.FromMM(400)}
	result := oneSheetResult(problem, part, stock, &offcut)

	saved, err := store.SaveRun(ctx, jobs.SaveRunRequest{Problem: problem, Solver: "test-solver", Result: result})
	if err != nil {
		t.Fatalf("save run: %v", err)
	}
	if _, err := store.AcceptPlan(ctx, saved.PlanID); err != nil {
		t.Fatalf("accept: %v", err)
	}

	from := time.Now().Add(-time.Hour)
	to := time.Now().Add(time.Hour)
	report, err := store.KPIs(ctx, from, to)
	if err != nil {
		t.Fatalf("kpis: %v", err)
	}
	if report.Realized.Plans < 1 {
		t.Fatalf("realized plans = %d, want at least 1", report.Realized.Plans)
	}
	if report.Realized.Sheets < 1 || report.Realized.PartsPlaced < 1 {
		t.Fatalf("realized bucket is empty: %+v", report.Realized)
	}
	if report.Realized.StockAreaM2 <= 0 || report.Realized.PartAreaM2 <= 0 {
		t.Fatalf("realized areas are empty: %+v", report.Realized)
	}
	if report.Realized.YieldPct <= 0 || report.Realized.YieldPct > 100 {
		t.Fatalf("realized yield = %v", report.Realized.YieldPct)
	}
	if report.Created.Plans < report.Realized.Plans {
		t.Fatalf("created plans (%d) below realized (%d)", report.Created.Plans, report.Realized.Plans)
	}
	if len(report.Series) < 1 {
		t.Fatalf("expected at least one series point, got %+v", report.Series)
	}
	if report.Series[len(report.Series)-1].PlanID == "" {
		t.Fatalf("series point has no plan id: %+v", report.Series)
	}
}
