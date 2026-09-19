package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/size-module/backend/internal/modules/jobs"
	"github.com/size-module/backend/internal/modules/plans"
	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/validator"
)

// TestPlanEditAndReoptimize walks the full editing loop: save a plan, move and
// lock a placement (version 2), re-solve around the lock (version 3) and check
// that the shop rules still hold.
func TestPlanEditAndReoptimize(t *testing.T) {
	store := queueTestStore(t)
	ctx := context.Background()
	formatID, formatCode, width, height, cost := demoSheetFormat(t, store)

	part := core.Part{ID: "edit-pane", Code: "EDIT-PANE-600x400", Width: core.FromMM(600), Height: core.FromMM(400), Quantity: 2, AllowRotate: true}
	stock := core.StockItem{ID: formatID, Code: formatCode, Width: width, Height: height, Quantity: 1, CostPerUnit: cost}
	problem := core.Normalize(core.Problem{Parts: []core.Part{part}, Stocks: []core.StockItem{stock}, Rules: core.DefaultRules()})
	offcut := core.Rect{X: core.FromMM(700), Y: core.FromMM(10), W: core.FromMM(900), H: core.FromMM(400)}
	result := oneSheetResult(problem, part, stock, &offcut)

	saved, err := store.SaveRun(ctx, jobs.SaveRunRequest{Problem: problem, Solver: "test-solver", Result: result})
	if err != nil {
		t.Fatalf("save run: %v", err)
	}
	v1, err := store.GetPlan(ctx, saved.PlanID)
	if err != nil {
		t.Fatalf("get v1: %v", err)
	}
	if v1.Version != 1 || len(v1.Solution.Sheets) != 1 || len(v1.Solution.Sheets[0].Placements) != 1 {
		t.Fatalf("v1 shape unexpected: %+v", v1.Summary)
	}
	placement := v1.Solution.Sheets[0].Placements[0]
	if placement.ID == "" {
		t.Fatal("placements must carry their id when a plan is read back")
	}
	if v1.Problem == nil {
		t.Fatal("expected the problem snapshot")
	}

	// Edit: move the pane and lock it.
	newX, newY := core.FromMM(700), core.FromMM(600)
	locked := true
	sol := v1.Solution
	applied, violations, err := plans.ApplyOperations(&sol, *v1.Problem, []plans.EditOperation{
		{PlacementID: placement.ID, X: &newX, Y: &newY, Locked: &locked},
	})
	if err != nil {
		t.Fatalf("apply edit: %v", err)
	}
	if applied != 1 {
		t.Fatalf("applied %d operations, want 1", applied)
	}
	plans.Finalize(*v1.Problem, &sol)
	violations = append(violations, validator.Validate(*v1.Problem, sol)...)
	if plans.HasErrors(violations) {
		t.Fatalf("edited layout is invalid: %+v", violations)
	}

	v2, err := store.SaveVersion(ctx, v1, *v1.Problem, sol, "Moved pane")
	if err != nil {
		t.Fatalf("save version 2: %v", err)
	}
	if v2.Version != 2 || v2.ParentPlanID != v1.ID {
		t.Fatalf("v2 lineage wrong: version=%d parent=%s", v2.Version, v2.ParentPlanID)
	}
	moved := v2.Solution.Sheets[0].Placements[0]
	if moved.X != newX || moved.Y != newY || !moved.Locked {
		t.Fatalf("v2 placement = %+v, want (%d,%d) locked", moved, newX, newY)
	}
	source, err := store.GetPlan(ctx, v1.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if source.Status != "archived" {
		t.Fatalf("source plan status = %q, want archived", source.Status)
	}

	// Re-solve: the locked pane becomes a pinned sheet and the only stock copy
	// is occupied by it; the second pane has to fit in its free area.
	pinned := plans.BuildPinnedProblem(*v2.Problem, v2.Solution)
	if len(pinned.Pinned) != 1 || len(pinned.Pinned[0].Placements) != 1 {
		t.Fatalf("pinned problem = %+v", pinned.Pinned)
	}
	if len(pinned.Stocks) != 1 || pinned.Stocks[0].Quantity != 0 {
		t.Fatalf("pinned sheet must consume the stock copy: %+v", pinned.Stocks)
	}
	resolved, err := optimizer.Solve(ctx, pinned, "", optimizer.DefaultRegistry(), nil)
	if err != nil {
		t.Fatalf("solve pinned: %v", err)
	}
	if resolved.Solution.Solver != "pinned-2d" {
		t.Fatalf("solver = %q, want pinned-2d", resolved.Solution.Solver)
	}
	var kept *core.Placement
	for _, sheet := range resolved.Solution.Sheets {
		for i := range sheet.Placements {
			pl := &sheet.Placements[i]
			if pl.PartCode == part.Code && pl.Locked {
				kept = pl
			}
		}
	}
	if kept == nil || kept.X != newX || kept.Y != newY {
		t.Fatalf("the locked pane did not stay put: %+v", kept)
	}
	// The free area of the pinned sheet takes the second pane, so demand is
	// still fulfilled without any fresh stock.
	if resolved.Solution.Metrics.PartsPlaced != 2 {
		t.Fatalf("placed %d pieces, want 2: %+v", resolved.Solution.Metrics.PartsPlaced, resolved.Solution.Unplaced)
	}
	if len(resolved.Violations) > 0 {
		for _, v := range resolved.Violations {
			if v.Severity == "error" {
				t.Fatalf("re-solved plan is invalid: %+v", v)
			}
		}
	}

	v3, err := store.SaveVersion(ctx, v2, pinned, resolved.Solution, "Re-solved around lock")
	if err != nil {
		t.Fatalf("save version 3: %v", err)
	}
	if v3.Version != 3 || v3.ParentPlanID != v2.ID {
		t.Fatalf("v3 lineage wrong: version=%d parent=%s", v3.Version, v3.ParentPlanID)
	}

	// Accepted plans are frozen: no more edits.
	if _, err := store.AcceptPlan(ctx, v3.ID); err != nil {
		t.Fatalf("accept v3: %v", err)
	}
	if _, err := store.SaveVersion(ctx, v3, pinned, resolved.Solution, ""); !errors.Is(err, plans.ErrNotEditable) {
		t.Fatalf("editing an accepted plan: got %v, want ErrNotEditable", err)
	}
}
