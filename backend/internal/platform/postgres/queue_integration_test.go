package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/size-module/backend/internal/modules/jobs"
	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
)

// The queue integration test needs a real database. It is skipped unless
// CUTOPTICS_TEST_DATABASE_URL is set, so `go test ./...` stays infrastructure
// free:
//
//	$env:CUTOPTICS_TEST_DATABASE_URL = "postgres://cutoptics:cutoptics@localhost:5433/cutoptics?sslmode=disable"
//	go test ./internal/platform/postgres/ -run TestQueue -v
func queueTestStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("CUTOPTICS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set CUTOPTICS_TEST_DATABASE_URL to run the queue integration test")
	}
	ctx := context.Background()
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewStore(pool)
}

func testProblem() core.Problem {
	return core.Normalize(core.Problem{
		Parts: []core.Part{
			{ID: "pane", Code: "PANE-600x400", Width: core.FromMM(600), Height: core.FromMM(400), Quantity: 4, AllowRotate: true},
		},
		Stocks: []core.StockItem{
			{ID: "sheet", Code: "SHEET-2440x1220", Width: core.FromMM(2440), Height: core.FromMM(1220), Quantity: 2},
		},
		Rules: core.DefaultRules(),
	})
}

// claimOurJob claims until the given job id appears. Leftovers from earlier
// runs are failed so they do not stay in "running".
func claimOurJob(t *testing.T, ctx context.Context, store *Store, jobID string) jobs.QueuedJob {
	t.Helper()
	for attempt := 0; attempt < 20; attempt++ {
		claimed, err := store.ClaimNext(ctx)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if claimed == nil {
			break
		}
		if claimed.ID == jobID {
			return *claimed
		}
		_ = store.Fail(ctx, claimed.ID, err2("integration test cleanup"))
	}
	t.Fatalf("job %s was never claimed", jobID)
	return jobs.QueuedJob{}
}

type err2 string

func (e err2) Error() string { return string(e) }

func TestQueueLifecycle(t *testing.T) {
	store := queueTestStore(t)
	ctx := context.Background()

	jobID, err := store.Enqueue(ctx, jobs.EnqueueRequest{
		Problem:  testProblem(),
		Solver:   "shelf-2d",
		BudgetMS: 500,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	view, err := store.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("get queued job: %v", err)
	}
	if view.Status != "queued" {
		t.Fatalf("expected a queued job, got %q", view.Status)
	}

	claimed := claimOurJob(t, ctx, store, jobID)
	if claimed.Solver != "shelf-2d" || claimed.BudgetMS != 500 {
		t.Fatalf("claimed job lost its settings: %+v", claimed)
	}
	if len(claimed.Problem.Parts) != 1 || claimed.Problem.Parts[0].Width != core.FromMM(600) {
		t.Fatalf("claimed job lost its problem snapshot: %+v", claimed.Problem)
	}

	// Running jobs cannot be claimed twice.
	if again, err := store.ClaimNext(ctx); err != nil {
		t.Fatalf("second claim: %v", err)
	} else if again != nil && again.ID == jobID {
		t.Fatal("the same job was claimed twice")
	}

	result := optimizer.Result{
		Solution: core.Solution{
			Solver: "shelf-2d", SolverVersion: "test", Seed: 1,
			Sheets: []core.SheetPlan{{
				Index: 0, StockID: "sheet", StockCode: "SHEET-2440x1220",
				Width: core.FromMM(2440), Height: core.FromMM(1220),
				Placements: []core.Placement{{
					PartID: "pane", PartCode: "PANE-600x400",
					X: core.FromMM(10), Y: core.FromMM(10),
					W: core.FromMM(600), H: core.FromMM(400),
				}},
			}},
		},
		Score: 1.5,
	}
	result.Solution.Metrics = core.Summarize(claimed.Problem, result.Solution.Sheets, nil, 0)

	planID, err := store.Complete(ctx, claimed, result)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if planID == "" {
		t.Fatal("complete returned no plan id")
	}

	finished, err := store.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("get finished job: %v", err)
	}
	if finished.Status != "done" {
		t.Fatalf("expected a done job, got %q (%s)", finished.Status, finished.Error)
	}
	if finished.PlanID != planID {
		t.Fatalf("plan id mismatch: %q vs %q", finished.PlanID, planID)
	}
	if finished.Metrics == nil || finished.Metrics.PartsPlaced != 1 {
		t.Fatalf("expected plan metrics, got %+v", finished.Metrics)
	}
	if len(finished.Result) == 0 {
		t.Fatal("expected the archived result JSON")
	}
	if finished.StartedAt == nil || finished.FinishedAt == nil {
		t.Fatalf("expected start and finish timestamps, got %+v %+v", finished.StartedAt, finished.FinishedAt)
	}
}

func TestQueueCancelAndFail(t *testing.T) {
	store := queueTestStore(t)
	ctx := context.Background()

	queuedID, err := store.Enqueue(ctx, jobs.EnqueueRequest{Problem: testProblem(), BudgetMS: 500})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	cancelled, err := store.Cancel(ctx, queuedID)
	if err != nil || !cancelled {
		t.Fatalf("cancel queued job: cancelled=%v err=%v", cancelled, err)
	}
	view, err := store.Get(ctx, queuedID)
	if err != nil {
		t.Fatalf("get cancelled job: %v", err)
	}
	if view.Status != "cancelled" {
		t.Fatalf("expected cancelled, got %q", view.Status)
	}
	// Cancelling twice is a no-op.
	if cancelled, err := store.Cancel(ctx, queuedID); err != nil || cancelled {
		t.Fatalf("second cancel should not change anything: cancelled=%v err=%v", cancelled, err)
	}

	// A claimed job that fails gets the reason recorded.
	failingID, err := store.Enqueue(ctx, jobs.EnqueueRequest{Problem: testProblem(), BudgetMS: 500})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed := claimOurJob(t, ctx, store, failingID)
	if err := store.Fail(ctx, claimed.ID, err2("integration test failure")); err != nil {
		t.Fatalf("fail: %v", err)
	}
	failedView, err := store.Get(ctx, failingID)
	if err != nil {
		t.Fatalf("get failed job: %v", err)
	}
	if failedView.Status != "failed" || failedView.Error != "integration test failure" {
		t.Fatalf("expected a failed job with a reason, got %q / %q", failedView.Status, failedView.Error)
	}

	// Stale running jobs are failed by the recovery sweep. The sweep only
	// touches jobs older than a second, so let this one age first.
	staleID, err := store.Enqueue(ctx, jobs.EnqueueRequest{Problem: testProblem(), BudgetMS: 500})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	_ = claimOurJob(t, ctx, store, staleID)
	time.Sleep(1100 * time.Millisecond)
	if _, err := store.ReleaseStale(ctx, 0); err != nil {
		t.Fatalf("release stale: %v", err)
	}
	staleView, err := store.Get(ctx, staleID)
	if err != nil {
		t.Fatalf("get stale job: %v", err)
	}
	if staleView.Status != "failed" {
		t.Fatalf("expected the stale job to be failed, got %q", staleView.Status)
	}
}

func TestQueueUnknownJob(t *testing.T) {
	store := queueTestStore(t)
	ctx := context.Background()
	if _, err := store.Get(ctx, "00000000-0000-0000-0000-000000000000"); err != jobs.ErrJobNotFound {
		t.Fatalf("expected ErrJobNotFound, got %v", err)
	}
}
