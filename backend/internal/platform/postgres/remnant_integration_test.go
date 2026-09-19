package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/size-module/backend/internal/modules/jobs"
	"github.com/size-module/backend/internal/modules/plans"
	"github.com/size-module/backend/internal/modules/stock"
	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
)

// The remnant lifecycle integration tests need a real, seeded database. They
// are skipped unless CUTOPTICS_TEST_DATABASE_URL is set (see
// queue_integration_test.go).

// demoSheetFormat returns the seeded glass sheet format, or skips the test.
func demoSheetFormat(t *testing.T, store *Store) (id, code string, w, h int64, cost float64) {
	t.Helper()
	formats, err := store.ListStockFormats(context.Background())
	if err != nil {
		t.Fatalf("list stock formats: %v", err)
	}
	for _, f := range formats {
		if f.Code == "SHEET-2440x1220" {
			return f.ID, f.Code, f.WidthMicron, f.HeightMicron, f.CostPerUnit
		}
	}
	t.Skip("demo seed data is not applied (no SHEET-2440x1220)")
	return "", "", 0, 0, 0
}

// oneSheetResult builds a valid one-sheet result with an optional offcut.
func oneSheetResult(problem core.Problem, part core.Part, stockItem core.StockItem, offcut *core.Rect) optimizer.Result {
	sheets := []core.SheetPlan{{
		Index:     0,
		StockID:   stockItem.ID,
		StockCode: stockItem.Code,
		Label:     stockItem.Label,
		Width:     stockItem.Width,
		Height:    stockItem.Height,
		Placements: []core.Placement{{
			PartID:   part.ID,
			PartCode: part.Code,
			X:        core.FromMM(10),
			Y:        core.FromMM(10),
			W:        part.Width,
			H:        part.Height,
		}},
	}}
	if offcut != nil {
		sheets[0].Offcuts = []core.Rect{*offcut}
	}
	solution := core.Solution{
		Solver:        "test-solver",
		SolverVersion: "test",
		Seed:          1,
		Sheets:        sheets,
		Notes:         []string{"integration test plan"},
	}
	solution.Metrics = core.Summarize(problem, sheets, nil, 0)
	return optimizer.Result{Solution: solution, Score: 1}
}

func TestPlanAcceptCreatesRemnants(t *testing.T) {
	store := queueTestStore(t)
	ctx := context.Background()
	formatID, formatCode, width, height, cost := demoSheetFormat(t, store)

	part := core.Part{ID: "accept-pane", Code: "ACCEPT-PANE-600x400", Width: core.FromMM(600), Height: core.FromMM(400), Quantity: 1, AllowRotate: true}
	sheet := core.StockItem{ID: formatID, Code: formatCode, Width: width, Height: height, Quantity: 1, CostPerUnit: cost}
	problem := core.Normalize(core.Problem{Parts: []core.Part{part}, Stocks: []core.StockItem{sheet}, Rules: core.DefaultRules()})

	offcut := core.Rect{X: core.FromMM(700), Y: core.FromMM(10), W: core.FromMM(900), H: core.FromMM(400)}
	result := oneSheetResult(problem, part, sheet, &offcut)

	saved, err := store.SaveRun(ctx, jobs.SaveRunRequest{Problem: problem, Solver: "test-solver", Result: result})
	if err != nil {
		t.Fatalf("save run: %v", err)
	}

	detail, err := store.GetPlan(ctx, saved.PlanID)
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if detail.Status != "draft" {
		t.Fatalf("fresh plan status = %q, want draft", detail.Status)
	}
	if len(detail.Solution.Sheets) != 1 || len(detail.Solution.Sheets[0].Offcuts) != 1 {
		t.Fatalf("archived plan lost its layout: %+v", detail.Solution.Sheets)
	}
	if detail.Solution.Metrics.PartsPlaced != 1 {
		t.Fatalf("archived metrics look wrong: %+v", detail.Solution.Metrics)
	}

	accepted, err := store.AcceptPlan(ctx, saved.PlanID)
	if err != nil {
		t.Fatalf("accept plan: %v", err)
	}
	if accepted.Status != "accepted" {
		t.Fatalf("status after accept = %q", accepted.Status)
	}
	if len(accepted.RemnantsCreated) != 1 {
		t.Fatalf("expected one remnant, got %+v", accepted.RemnantsCreated)
	}
	remnant := accepted.RemnantsCreated[0]
	if !strings.HasPrefix(remnant.Label, "OFF-") {
		t.Fatalf("remnant label %q is not an offcut label", remnant.Label)
	}
	if remnant.WidthMicron != offcut.W || remnant.HeightMicron != offcut.H {
		t.Fatalf("remnant dims = %d x %d, want %d x %d", remnant.WidthMicron, remnant.HeightMicron, offcut.W, offcut.H)
	}
	if accepted.FormatDecrements != 1 {
		t.Fatalf("format decrements = %d, want 1", accepted.FormatDecrements)
	}

	// The plan is frozen: accepting twice must not double-book the stock.
	if _, err := store.AcceptPlan(ctx, saved.PlanID); !errors.Is(err, plans.ErrNotAcceptable) {
		t.Fatalf("second accept: got %v, want ErrNotAcceptable", err)
	}

	// The remnant is now part of the pool and is offered to later solves.
	items, err := store.ListItems(ctx, stock.ListFilter{Status: "available"})
	if err != nil {
		t.Fatalf("list stock items: %v", err)
	}
	var stored stock.Item
	for _, item := range items {
		if item.ID == remnant.ID {
			stored = item
		}
	}
	if stored.ID == "" {
		t.Fatalf("remnant %s is not in the available pool", remnant.ID)
	}
	if !stored.IsRemnant || stored.ParentPlanID != saved.PlanID || stored.ParentSheetIndex == nil || *stored.ParentSheetIndex != 0 {
		t.Fatalf("remnant lineage is wrong: %+v", stored)
	}
	if stored.CostPerUnit <= 0 || stored.CostPerUnit > cost {
		t.Fatalf("remnant cost %v should be prorated between 0 and %v", stored.CostPerUnit, cost)
	}

	remnants, err := store.ListRemnants(ctx, "")
	if err != nil {
		t.Fatalf("list remnants: %v", err)
	}
	var offered *core.StockItem
	for i := range remnants {
		if remnants[i].ID == remnant.ID {
			offered = &remnants[i]
		}
	}
	if offered == nil {
		t.Fatalf("remnant %s is not offered to solvers", remnant.ID)
	}
	if !offered.IsRemnant || offered.Quantity != 1 || offered.Width != offcut.W || offered.Height != offcut.H {
		t.Fatalf("solver remnant looks wrong: %+v", *offered)
	}
	if offered.FormatID != formatID {
		t.Fatalf("remnant format lineage = %q, want %q", offered.FormatID, formatID)
	}
}

func TestPlanAcceptConsumesPhysicalPiece(t *testing.T) {
	store := queueTestStore(t)
	ctx := context.Background()
	formatID, formatCode, width, height, cost := demoSheetFormat(t, store)

	notRemnant := false
	label := fmt.Sprintf("TEST-PHY-%d", time.Now().UnixNano())
	piece, err := store.CreateItem(ctx, stock.CreateInput{
		FormatID:     formatID,
		Label:        label,
		WidthMicron:  width,
		HeightMicron: height,
		IsRemnant:    &notRemnant,
		Location:     "integration test",
		CostPerUnit:  cost,
	})
	if err != nil {
		t.Fatalf("create physical piece: %v", err)
	}

	part := core.Part{ID: "consume-pane", Code: "CONSUME-PANE-600x400", Width: core.FromMM(600), Height: core.FromMM(400), Quantity: 1, AllowRotate: true}
	sheet := core.StockItem{ID: piece.ID, Code: formatCode, Label: piece.Label, Width: width, Height: height, Quantity: 1, CostPerUnit: cost}
	problem := core.Normalize(core.Problem{Parts: []core.Part{part}, Stocks: []core.StockItem{sheet}, Rules: core.DefaultRules()})
	result := oneSheetResult(problem, part, sheet, nil)

	saved, err := store.SaveRun(ctx, jobs.SaveRunRequest{Problem: problem, Solver: "test-solver", Result: result})
	if err != nil {
		t.Fatalf("save run: %v", err)
	}
	accepted, err := store.AcceptPlan(ctx, saved.PlanID)
	if err != nil {
		t.Fatalf("accept plan: %v", err)
	}
	if len(accepted.StockConsumed) != 1 || accepted.StockConsumed[0] != label {
		t.Fatalf("stock consumed = %v, want [%s]", accepted.StockConsumed, label)
	}
	if accepted.FormatDecrements != 0 {
		t.Fatalf("a physical piece must not also decrement the format (got %d)", accepted.FormatDecrements)
	}

	after, err := store.GetItem(ctx, piece.ID)
	if err != nil {
		t.Fatalf("get consumed piece: %v", err)
	}
	if after.Status != "consumed" {
		t.Fatalf("piece status = %q, want consumed", after.Status)
	}
	if after.ConsumedByPlanID != saved.PlanID || after.ConsumedAt == nil {
		t.Fatalf("piece consumption lineage is wrong: %+v", after)
	}
}
