package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/size-module/backend/internal/modules/campaigns"
	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
)

// TestCampaignLifecycle plans two items against one shared stock budget: the
// first consumes the sheet and returns its offcuts, the second plans from what
// is left, and the campaign completes when nothing is pending.
func TestCampaignLifecycle(t *testing.T) {
	store := queueTestStore(t)
	ctx := context.Background()
	formatID, formatCode, width, height, cost := demoSheetFormat(t, store)

	detail, err := store.CreateCampaign(ctx, campaigns.CreateInput{
		Name:     "Integration campaign",
		BudgetMS: 500,
		Seed:     7,
		Stock: []core.StockItem{{
			ID: formatID, Code: formatCode,
			Width: width, Height: height, Quantity: 1, CostPerUnit: cost,
		}},
	})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	if detail.Status != campaigns.StatusDraft || detail.Progress.Items != 0 {
		t.Fatalf("fresh campaign = %+v", detail.Campaign)
	}
	if len(detail.InitialStock) != 1 || len(detail.Stock) != 1 {
		t.Fatalf("budget not stored: %+v", detail.Campaign)
	}

	itemA := campaigns.CreateItemInput{
		Name: "Item A",
		Parts: []core.Part{{
			ID: "camp-a", Code: "CAMP-A-600x400",
			Width: core.FromMM(600), Height: core.FromMM(400), Quantity: 2, AllowRotate: true,
		}},
	}
	itemB := campaigns.CreateItemInput{
		Name: "Item B",
		Parts: []core.Part{{
			ID: "camp-b", Code: "CAMP-B-600x400",
			Width: core.FromMM(600), Height: core.FromMM(400), Quantity: 1, AllowRotate: true,
		}},
	}
	if _, err := store.AddItem(ctx, detail.ID, itemA); err != nil {
		t.Fatalf("add item A: %v", err)
	}
	detail, err = store.AddItem(ctx, detail.ID, itemB)
	if err != nil {
		t.Fatalf("add item B: %v", err)
	}
	if detail.Progress.Pending != 2 || len(detail.Items) != 2 || detail.Items[1].Seq != 2 {
		t.Fatalf("items not queued in order: %+v", detail)
	}

	// Run the first item: the sheet is consumed and its offcuts re-enter the
	// budget as labelled remnants.
	detail = runCampaignItem(t, store, detail)
	if detail.Status != campaigns.StatusActive {
		t.Fatalf("status after item A = %q, want active", detail.Status)
	}
	if detail.Progress.Planned != 1 || detail.Progress.Pending != 1 {
		t.Fatalf("progress after item A: %+v", detail.Progress)
	}
	if detail.Items[0].PlanID == "" || detail.Items[0].JobID == "" {
		t.Fatalf("item A has no plan/job: %+v", detail.Items[0])
	}
	for _, entry := range detail.Stock {
		if entry.ID == formatID {
			t.Fatalf("the format should be used up: %+v", detail.Stock)
		}
	}
	newRemnants := 0
	for _, entry := range detail.Stock {
		if entry.IsRemnant {
			newRemnants++
			if !strings.HasPrefix(entry.Label, "CMP-") {
				t.Fatalf("campaign remnant label = %q", entry.Label)
			}
		}
	}
	if newRemnants == 0 {
		t.Fatalf("expected the offcuts to re-enter the budget: %+v", detail.Stock)
	}
	plan, err := store.GetPlan(ctx, detail.Items[0].PlanID)
	if err != nil {
		t.Fatalf("get item A plan: %v", err)
	}
	if plan.Name != "Item A" || plan.Status != "draft" {
		t.Fatalf("plan = %+v, want draft named Item A", plan.Summary)
	}
	if plan.Problem == nil {
		t.Fatal("campaign plans must carry their problem snapshot")
	}

	// Run the second item from the remaining budget and finish the campaign.
	detail = runCampaignItem(t, store, detail)
	if detail.Status != campaigns.StatusCompleted {
		t.Fatalf("status after item B = %q, want completed", detail.Status)
	}
	if detail.Progress.Planned != 2 || detail.Progress.Pending != 0 {
		t.Fatalf("progress after item B: %+v", detail.Progress)
	}
	if detail.CompletedAt == nil {
		t.Fatal("completed campaign has no completion timestamp")
	}

	// Completed campaigns reject new items and item deletion.
	if _, err := store.AddItem(ctx, detail.ID, itemA); err != campaigns.ErrNotEditable {
		t.Fatalf("add to completed campaign: got %v, want ErrNotEditable", err)
	}
	if _, err := store.DeleteItem(ctx, detail.ID, detail.Items[0].ID); err != campaigns.ErrNotFound {
		t.Fatalf("delete a planned item: got %v, want ErrNotFound", err)
	}
}

// runCampaignItem mirrors the handler: take the next pending item, solve it
// against the remaining budget, consume the budget and store the result.
func runCampaignItem(t *testing.T, store *Store, detail campaigns.Detail) campaigns.Detail {
	t.Helper()
	ctx := context.Background()
	item, err := store.NextItem(ctx, detail.ID)
	if err != nil {
		t.Fatalf("next item: %v", err)
	}
	problem := campaigns.BuildItemProblem(detail.Campaign, item, 0)
	result, err := optimizer.Solve(ctx, problem, "", optimizer.DefaultRegistry(), nil)
	if err != nil {
		t.Fatalf("solve item %d: %v", item.Seq, err)
	}
	stock := campaigns.ConsumeBudget(detail.Stock, result.Solution, problem.Rules,
		func(sheetIndex, offcutIndex int) string {
			return campaigns.RemnantLabel(detail.ID, item.Seq, sheetIndex, offcutIndex)
		})
	updated, err := store.CompleteItem(ctx, detail.ID, item.ID, problem, result, stock)
	if err != nil {
		t.Fatalf("complete item %d: %v", item.Seq, err)
	}
	return updated
}
