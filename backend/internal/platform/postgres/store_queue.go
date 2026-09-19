package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/size-module/backend/internal/modules/jobs"
	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/db"
)

// Enqueue stores a queued job. The problem snapshot travels with the row, so a
// worker needs nothing else to run it, even after a restart.
func (s *Store) Enqueue(ctx context.Context, req jobs.EnqueueRequest) (string, error) {
	plant, err := s.queries.GetDefaultPlant(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("no plant configured; run db/scripts/seed.ps1")
		}
		return "", err
	}
	budget := req.BudgetMS
	if budget <= 0 {
		budget = 5000
	}
	row, err := s.queries.CreateJob(ctx, db.CreateJobParams{
		PlantID:   plant.ID,
		Solver:    req.Solver,
		Input:     marshalJSON(req.Problem),
		Seed:      int64(req.Seed),
		BudgetMs:  int32(budget),
		CreatedBy: pgtype.UUID{},
	})
	if err != nil {
		return "", err
	}
	return row.ID.String(), nil
}

// ClaimNext claims the oldest queued job with FOR UPDATE SKIP LOCKED, or
// reports nil when the queue is empty.
func (s *Store) ClaimNext(ctx context.Context) (*jobs.QueuedJob, error) {
	row, err := s.queries.ClaimNextJob(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	problem, err := decodeProblem(row.Input)
	if err != nil {
		return nil, fmt.Errorf("job %s: %w", row.ID, err)
	}
	return &jobs.QueuedJob{
		ID:       row.ID.String(),
		Problem:  problem,
		Solver:   row.Solver,
		Seed:     uint64(row.Seed),
		BudgetMS: int(row.BudgetMs),
	}, nil
}

// Complete archives the plan of a finished job and marks it done atomically.
func (s *Store) Complete(ctx context.Context, job jobs.QueuedJob, result optimizer.Result) (string, error) {
	plant, err := s.queries.GetDefaultPlant(ctx)
	if err != nil {
		return "", err
	}
	jobID, err := uuid.Parse(job.ID)
	if err != nil {
		return "", err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	planID, err := writePlan(ctx, q, planParams{
		PlantID: plant.ID,
		JobID:   pgtypeUUID(jobID),
		Version: 1,
		Status:  "draft",
		Name:    "Optimization run",
	}, job.Problem, result)
	if err != nil {
		return "", err
	}
	if err := q.MarkJobDone(ctx, db.MarkJobDoneParams{ID: jobID, Result: marshalJSON(result)}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return planID.String(), nil
}

func (s *Store) Fail(ctx context.Context, jobID string, cause error) error {
	id, err := uuid.Parse(jobID)
	if err != nil {
		return err
	}
	message := "job failed"
	if cause != nil {
		message = cause.Error()
	}
	return s.queries.MarkJobFailed(ctx, db.MarkJobFailedParams{ID: id, Error: message})
}

func (s *Store) Cancel(ctx context.Context, jobID string) (bool, error) {
	id, err := uuid.Parse(jobID)
	if err != nil {
		return false, err
	}
	rows, err := s.queries.CancelJob(ctx, id)
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *Store) Get(ctx context.Context, jobID string) (*jobs.JobView, error) {
	id, err := uuid.Parse(jobID)
	if err != nil {
		return nil, jobs.ErrJobNotFound
	}
	row, err := s.queries.GetJobView(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, jobs.ErrJobNotFound
	}
	if err != nil {
		return nil, err
	}

	view := &jobs.JobView{
		ID:        row.ID.String(),
		Status:    row.Status,
		Solver:    row.Solver,
		Error:     row.Error,
		CreatedAt: row.CreatedAt.Time,
	}
	if row.StartedAt.Valid {
		started := row.StartedAt.Time
		view.StartedAt = &started
	}
	if row.FinishedAt.Valid {
		finished := row.FinishedAt.Time
		view.FinishedAt = &finished
	}
	if row.PlanID.Valid {
		view.PlanID = uuid.UUID(row.PlanID.Bytes).String()
	}
	if len(row.PlanMetrics) > 0 {
		var metrics core.Metrics
		if err := json.Unmarshal(row.PlanMetrics, &metrics); err == nil {
			view.Metrics = &metrics
		}
	}
	if len(row.Result) > 0 {
		view.Result = json.RawMessage(row.Result)
	}
	return view, nil
}

// ReleaseStale fails jobs that a stopped worker left in "running".
func (s *Store) ReleaseStale(ctx context.Context, olderThan time.Duration) (int64, error) {
	seconds := int32(olderThan.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return s.queries.FailStaleJobs(ctx, seconds)
}

func decodeProblem(raw []byte) (core.Problem, error) {
	var problem core.Problem
	if err := json.Unmarshal(raw, &problem); err != nil {
		return core.Problem{}, err
	}
	return core.Normalize(problem), nil
}
