package jobs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/events"
)

// ProgressEvent is the compact payload streamed while a job runs. The full
// plan is fetched separately once the job is done.
type ProgressEvent struct {
	Sheets    int     `json:"sheets"`
	Placed    int     `json:"placed"`
	Requested int     `json:"requested"`
	YieldPct  float64 `json:"yieldPct"`
	WastePct  float64 `json:"wastePct"`
	ElapsedMS int64   `json:"elapsedMs"`
}

// DoneEvent is the payload of a terminal "done" event.
type DoneEvent struct {
	JobID   string       `json:"jobId"`
	PlanID  string       `json:"planId"`
	Metrics core.Metrics `json:"metrics"`
}

// Worker runs queued optimization jobs. Several workers can share one process
// and several processes can share one database: claiming uses
// FOR UPDATE SKIP LOCKED, so no job is ever picked up twice.
type Worker struct {
	store   QueueStore
	service *Service
	hub     *events.Hub
	workers int

	poll          time.Duration
	progressEvery time.Duration

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	stop    chan struct{}
	stopped bool
	wg      sync.WaitGroup
}

func NewWorker(store QueueStore, service *Service, hub *events.Hub, workers int) *Worker {
	if workers <= 0 {
		workers = 1
	}
	return &Worker{
		store:         store,
		service:       service,
		hub:           hub,
		workers:       workers,
		poll:          500 * time.Millisecond,
		progressEvery: 250 * time.Millisecond,
		cancels:       map[string]context.CancelFunc{},
		stop:          make(chan struct{}),
	}
}

// Start launches the worker goroutines and returns immediately.
func (w *Worker) Start(ctx context.Context) {
	if w.store == nil || w.service == nil {
		return
	}
	w.wg.Add(1)
	go w.recoverStale(ctx)
	for i := 0; i < w.workers; i++ {
		w.wg.Add(1)
		go w.loop(ctx)
	}
	slog.Info("job workers started", "workers", w.workers, "poll", w.poll.String())
}

// Stop cancels every running job and waits for the workers to exit.
func (w *Worker) Stop() {
	w.mu.Lock()
	if !w.stopped {
		close(w.stop)
		w.stopped = true
	}
	cancels := make([]context.CancelFunc, 0, len(w.cancels))
	for _, cancel := range w.cancels {
		cancels = append(cancels, cancel)
	}
	w.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	w.wg.Wait()
}

// Cancel stops a job running in this process. It reports whether the job was
// running here; a job running elsewhere is cancelled through the database.
func (w *Worker) Cancel(jobID string) bool {
	w.mu.Lock()
	cancel, ok := w.cancels[jobID]
	w.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (w *Worker) recoverStale(ctx context.Context) {
	defer w.wg.Done()
	released, err := w.store.ReleaseStale(ctx, 10*time.Minute)
	if err != nil {
		slog.Warn("could not release stale jobs", "error", err)
		return
	}
	if released > 0 {
		slog.Warn("stale running jobs were failed after a restart", "count", released)
	}
}

func (w *Worker) loop(ctx context.Context) {
	defer w.wg.Done()
	for {
		if w.stopping(ctx) {
			return
		}
		job, err := w.store.ClaimNext(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("claiming a job failed", "error", err)
			w.sleep(ctx, w.poll)
			continue
		}
		if job == nil {
			w.sleep(ctx, w.poll)
			continue
		}
		w.run(ctx, *job)
	}
}

func (w *Worker) stopping(ctx context.Context) bool {
	if ctx.Err() != nil {
		return true
	}
	select {
	case <-w.stop:
		return true
	default:
		return false
	}
}

func (w *Worker) sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-w.stop:
	case <-timer.C:
	}
}

// run executes one job: run the solver, publish progress, archive the plan.
func (w *Worker) run(parent context.Context, job QueuedJob) {
	budget := job.BudgetMS
	if budget <= 0 {
		budget = 5000
	}
	// The solvers honour the budget themselves; this timeout only stops a job
	// that ignored it completely.
	ctx, cancel := context.WithTimeout(parent, time.Duration(budget)*time.Millisecond+10*time.Second)
	defer cancel()

	w.mu.Lock()
	w.cancels[job.ID] = cancel
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		delete(w.cancels, job.ID)
		w.mu.Unlock()
	}()

	slog.Info("job started", "job", job.ID, "solver", job.Solver, "budgetMs", budget)
	w.publish(job.ID, events.Event{Type: "running", JobID: job.ID})

	var seq int64
	// Allow the first best-so-far plan through immediately: showing a valid
	// plan early is the point of streaming, even if the solver then pauses.
	last := time.Now().Add(-w.progressEvery)
	progress := func(solution core.Solution) {
		if w.progressEvery > 0 && time.Since(last) < w.progressEvery {
			return
		}
		last = time.Now()
		seq++
		m := solution.Metrics
		w.publish(job.ID, events.Event{
			Type:  "progress",
			JobID: job.ID,
			Seq:   seq,
			Data: ProgressEvent{
				Sheets:    m.SheetCount,
				Placed:    m.PartsPlaced,
				Requested: m.PartsRequested,
				YieldPct:  m.YieldPct,
				WastePct:  m.WastePct,
				ElapsedMS: m.ElapsedMS,
			},
		})
	}

	result, err := w.service.Run(ctx, job.Problem, job.Solver, progress)
	if err != nil {
		if errors.Is(err, context.Canceled) || parent.Err() != nil {
			w.finishCancelled(job.ID)
			return
		}
		if failErr := w.store.Fail(context.WithoutCancel(parent), job.ID, err); failErr != nil {
			slog.Error("marking a job failed", "job", job.ID, "error", failErr)
		}
		slog.Warn("job failed", "job", job.ID, "error", err)
		w.publish(job.ID, events.Event{Type: "failed", JobID: job.ID, Data: map[string]string{"error": err.Error()}})
		return
	}
	if ctx.Err() == context.Canceled {
		w.finishCancelled(job.ID)
		return
	}

	planID, err := w.store.Complete(context.WithoutCancel(parent), job, result)
	if err != nil {
		slog.Error("archiving a finished job", "job", job.ID, "error", err)
		if failErr := w.store.Fail(context.WithoutCancel(parent), job.ID, err); failErr != nil {
			slog.Error("marking a job failed", "job", job.ID, "error", failErr)
		}
		w.publish(job.ID, events.Event{Type: "failed", JobID: job.ID, Data: map[string]string{"error": err.Error()}})
		return
	}
	slog.Info("job done", "job", job.ID, "plan", planID,
		"sheets", result.Solution.Metrics.SheetCount, "yieldPct", result.Solution.Metrics.YieldPct)
	w.publish(job.ID, events.Event{
		Type:  "done",
		JobID: job.ID,
		Data:  DoneEvent{JobID: job.ID, PlanID: planID, Metrics: result.Solution.Metrics},
	})
}

func (w *Worker) finishCancelled(jobID string) {
	if _, err := w.store.Cancel(context.Background(), jobID); err != nil {
		slog.Warn("cancelling a job", "job", jobID, "error", err)
	}
	slog.Info("job cancelled", "job", jobID)
	w.publish(jobID, events.Event{Type: "cancelled", JobID: jobID})
}

func (w *Worker) publish(jobID string, event events.Event) {
	if w.hub != nil {
		w.hub.Publish(jobID, event)
	}
}
