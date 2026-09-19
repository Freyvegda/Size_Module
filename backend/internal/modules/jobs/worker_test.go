package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/events"
)

type fakeQueue struct {
	mu        sync.Mutex
	pending   []QueuedJob
	completed []QueuedJob
	cancelled []string
	failed    []string
}

func newFakeQueue() *fakeQueue { return &fakeQueue{} }

func (f *fakeQueue) Enqueue(context.Context, EnqueueRequest) (string, error) { return "unused", nil }

func (f *fakeQueue) ClaimNext(context.Context) (*QueuedJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return nil, nil
	}
	job := f.pending[0]
	f.pending = f.pending[1:]
	return &job, nil
}

func (f *fakeQueue) Complete(_ context.Context, job QueuedJob, _ optimizer.Result) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completed = append(f.completed, job)
	return "plan-" + job.ID, nil
}

func (f *fakeQueue) Fail(_ context.Context, jobID string, _ error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = append(f.failed, jobID)
	return nil
}

func (f *fakeQueue) Cancel(_ context.Context, jobID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelled = append(f.cancelled, jobID)
	return true, nil
}

func (f *fakeQueue) Get(_ context.Context, jobID string) (*JobView, error) {
	return &JobView{ID: jobID, Status: "queued"}, nil
}

func (f *fakeQueue) ReleaseStale(context.Context, time.Duration) (int64, error) { return 0, nil }

func (f *fakeQueue) snapshot() (completed, cancelled, failed []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, job := range f.completed {
		completed = append(completed, job.ID)
	}
	cancelled = append(cancelled, f.cancelled...)
	failed = append(failed, f.failed...)
	return
}

// waitForEvent reads events until one of the wanted types arrives.
func waitForEvent(t *testing.T, ch <-chan events.Event, wanted string, timeout time.Duration) events.Event {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case event := <-ch:
			if event.Type == wanted {
				return event
			}
		case <-deadline:
			t.Fatalf("no %q event within %s", wanted, timeout)
		}
	}
}

// eventLog drains a channel in the background and records what arrived.
type eventLog struct {
	mu     sync.Mutex
	events []events.Event
}

func collect(ch <-chan events.Event) *eventLog {
	log := &eventLog{}
	go func() {
		for event := range ch {
			log.add(event)
		}
	}()
	return log
}

func (l *eventLog) add(event events.Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *eventLog) count(eventType string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	seen := 0
	for _, event := range l.events {
		if event.Type == eventType {
			seen++
		}
	}
	return seen
}

func (l *eventLog) snapshot() []events.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]events.Event, len(l.events))
	copy(out, l.events)
	return out
}

func TestWorkerRunsQueuedJobAndPublishesProgress(t *testing.T) {
	queue := newFakeQueue()
	queue.pending = append(queue.pending, QueuedJob{
		ID:       "job-1",
		Problem:  DemoProblem(),
		BudgetMS: 300,
	})
	hub := events.NewHub()
	ch, unsubscribe := hub.Subscribe("job-1")
	defer unsubscribe()
	log := collect(ch)

	worker := NewWorker(queue, NewService(optimizer.DefaultRegistry()), hub, 1)
	worker.poll = 10 * time.Millisecond
	worker.progressEvery = 0 // publish every progress callback
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.Start(ctx)

	deadline := time.Now().Add(15 * time.Second)
	for {
		completed, _, _ := queue.snapshot()
		if len(completed) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job was not completed in time; events: %+v", log.snapshot())
		}
		time.Sleep(20 * time.Millisecond)
	}
	worker.Stop()

	if got := log.count("running"); got != 1 {
		t.Fatalf("expected exactly one running event, got %d", got)
	}
	if got := log.count("done"); got != 1 {
		t.Fatalf("expected exactly one done event, got %d", got)
	}
	if got := log.count("progress"); got == 0 {
		t.Fatalf("expected progress events, got none (%+v)", log.snapshot())
	}
	completed, cancelled, failed := queue.snapshot()
	if len(completed) != 1 || len(cancelled) != 0 || len(failed) != 0 {
		t.Fatalf("unexpected lifecycle: completed=%v cancelled=%v failed=%v", completed, cancelled, failed)
	}
}

func TestWorkerCancelsRunningJob(t *testing.T) {
	queue := newFakeQueue()
	queue.pending = append(queue.pending, QueuedJob{
		ID:       "job-1",
		Problem:  DemoProblem(),
		BudgetMS: 10000, // long enough that it cannot finish before the cancel
	})
	hub := events.NewHub()
	ch, unsubscribe := hub.Subscribe("job-1")
	defer unsubscribe()

	worker := NewWorker(queue, NewService(optimizer.DefaultRegistry()), hub, 1)
	worker.poll = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.Start(ctx)
	defer worker.Stop()

	waitForEvent(t, ch, "running", 10*time.Second)

	cancelled := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if worker.Cancel("job-1") {
			cancelled = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !cancelled {
		t.Fatal("the running job was not registered for cancellation")
	}
	waitForEvent(t, ch, "cancelled", 10*time.Second)

	completed, cancelledJobs, failed := queue.snapshot()
	if len(completed) != 0 {
		t.Fatalf("a cancelled job must not archive a plan, got %v", completed)
	}
	if len(cancelledJobs) != 1 || len(failed) != 0 {
		t.Fatalf("unexpected lifecycle: cancelled=%v failed=%v", cancelledJobs, failed)
	}
}

// failingSolver exists to exercise the failure path.
type failingSolver struct{}

func (failingSolver) Name() string               { return "boom-2d" }
func (failingSolver) Version() string            { return "test" }
func (failingSolver) Capabilities() core.Capabilities {
	return core.Capabilities{Dimension: core.Profile2D, CutMode: core.CutGuillotine}
}
func (failingSolver) Solve(context.Context, core.Problem, core.ProgressFunc) (core.Solution, error) {
	return core.Solution{}, errors.New("solver exploded")
}

func TestWorkerMarksFailedJobs(t *testing.T) {
	registry := core.NewRegistry()
	registry.Register(failingSolver{})

	queue := newFakeQueue()
	queue.pending = append(queue.pending, QueuedJob{
		ID:       "job-1",
		Problem:  DemoProblem(),
		Solver:   "boom-2d",
		BudgetMS: 200,
	})
	hub := events.NewHub()
	ch, unsubscribe := hub.Subscribe("job-1")
	defer unsubscribe()

	worker := NewWorker(queue, NewService(registry), hub, 1)
	worker.poll = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.Start(ctx)

	waitForEvent(t, ch, "failed", 10*time.Second)
	worker.Stop()

	completed, cancelled, failed := queue.snapshot()
	if len(completed) != 0 || len(cancelled) != 0 || len(failed) != 1 {
		t.Fatalf("unexpected lifecycle: completed=%v cancelled=%v failed=%v", completed, cancelled, failed)
	}
}
