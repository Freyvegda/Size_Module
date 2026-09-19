package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
)

// ErrJobNotFound is returned when a job id does not exist.
var ErrJobNotFound = errors.New("job not found")

// EnqueueRequest is a job submission. Everything needed to run the job must be
// in here so a worker can pick it up later, from another process or after a
// restart.
type EnqueueRequest struct {
	Name     string
	Problem  core.Problem
	Solver   string
	Seed     uint64
	BudgetMS int
}

// QueuedJob is a claimed job, ready to run.
type QueuedJob struct {
	ID       string
	Problem  core.Problem
	Solver   string
	Seed     uint64
	BudgetMS int
}

// JobView is the API representation of a job, including its plan when finished.
type JobView struct {
	ID         string          `json:"id"`
	Status     string          `json:"status"`
	Solver     string          `json:"solver"`
	Error      string          `json:"error,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
	StartedAt  *time.Time      `json:"startedAt,omitempty"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
	PlanID     string          `json:"planId,omitempty"`
	Metrics    *core.Metrics   `json:"metrics,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
}

// IsTerminal reports whether a job status is final.
func IsTerminal(status string) bool {
	switch status {
	case "done", "failed", "cancelled":
		return true
	default:
		return false
	}
}

// QueueStore is the persistence side of the asynchronous job pipeline. The
// Postgres implementation lives in internal/platform/postgres.
type QueueStore interface {
	// Enqueue stores a queued job and returns its id.
	Enqueue(ctx context.Context, req EnqueueRequest) (string, error)
	// ClaimNext atomically claims the oldest queued job, or returns nil when
	// the queue is empty. Concurrent workers must never claim the same job.
	ClaimNext(ctx context.Context) (*QueuedJob, error)
	// Complete archives the plan of a finished job and marks it done.
	Complete(ctx context.Context, job QueuedJob, result optimizer.Result) (planID string, err error)
	// Fail marks a job failed with a reason.
	Fail(ctx context.Context, jobID string, cause error) error
	// Cancel marks a queued or running job cancelled. It reports whether the
	// status actually changed.
	Cancel(ctx context.Context, jobID string) (bool, error)
	// Get returns one job with its plan metrics and result when finished.
	Get(ctx context.Context, jobID string) (*JobView, error)
	// ReleaseStale fails jobs left running by a crashed worker.
	ReleaseStale(ctx context.Context, olderThan time.Duration) (int64, error)
}
