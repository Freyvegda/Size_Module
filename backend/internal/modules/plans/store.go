// Package plans serves archived optimization plans: reading them back with
// editable placement identities, accepting them into the shop, hand edits with
// locked placements, re-solving around those locks, and exports.
package plans

import (
	"context"
	"errors"
	"time"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/costing"
	"github.com/size-module/backend/internal/optimizer/validator"
)

var (
	// ErrNotFound is returned when a plan id does not exist.
	ErrNotFound = errors.New("plan not found")
	// ErrNotAcceptable is returned when a plan is not draft or approved.
	ErrNotAcceptable = errors.New("plan is not in a state that can be accepted")
	// ErrNotEditable is returned when an edit or re-solve targets a plan that
	// was already accepted or archived.
	ErrNotEditable = errors.New("plan is not in a state that can be edited")
	// ErrNoProblem is returned when the plan has no problem snapshot to edit
	// or re-solve against.
	ErrNoProblem = errors.New("plan has no problem snapshot")
)

// Summary is one row of the plan list.
type Summary struct {
	ID            string       `json:"id"`
	JobID         string       `json:"jobId,omitempty"`
	ParentPlanID  string       `json:"parentPlanId,omitempty"`
	Status        string       `json:"status"`
	Version       int          `json:"version"`
	Name          string       `json:"name,omitempty"`
	Solver        string       `json:"solver"`
	SolverVersion string       `json:"solverVersion"`
	Seed          uint64       `json:"seed"`
	Metrics       core.Metrics `json:"metrics"`
	CreatedAt     time.Time    `json:"createdAt"`
	AcceptedAt    *time.Time   `json:"acceptedAt,omitempty"`
}

// Detail is one plan with its full layout, rebuilt into the same shape the
// optimizer returns so the viewer can render it without a second endpoint.
type Detail struct {
	Summary
	// Rules are the constraint values the plan was created with, so an editor
	// can clamp moves to the trim and show them.
	Rules  core.Rules       `json:"rules"`
	Result optimizer.Result `json:"result"`
}

// PlanData is the persistence view of a plan: the summary, the problem
// snapshot behind it and the layout with per-placement ids and lock flags.
// Handlers convert it into the API Detail; edit and re-solve operate on it.
type PlanData struct {
	Summary
	Problem    *core.Problem
	Rules      core.Rules
	Solution   core.Solution
	Score      float64
	Violations []validator.Violation
	Cost       costing.Report
}

// RemnantRef identifies a remnant created by accepting a plan.
type RemnantRef struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	LengthMicron int64  `json:"lengthMicron,omitempty"`
	WidthMicron  int64  `json:"widthMicron,omitempty"`
	HeightMicron int64  `json:"heightMicron,omitempty"`
}

// AcceptResult summarises what acceptance changed.
type AcceptResult struct {
	PlanID string `json:"planId"`
	Status string `json:"status"`
	Sheets int    `json:"sheets"`
	// RemnantsCreated lists the labelled offcuts that entered the stock pool.
	RemnantsCreated []RemnantRef `json:"remnantsCreated"`
	// StockConsumed lists the labels of physical pieces that were used up.
	StockConsumed []string `json:"stockConsumed"`
	// StockShortages counts physical pieces that were already consumed by
	// another plan and could not be taken again.
	StockShortages int `json:"stockShortages"`
	// FormatDecrements counts sheets taken from catalog on-hand quantities.
	FormatDecrements int `json:"formatDecrements"`
	// FormatShortages counts sheets whose format had no on-hand stock left to
	// decrement (the plan still stands; the catalog was already empty).
	FormatShortages int `json:"formatShortages"`
}

// Store is implemented by the postgres package. The method names are prefixed
// because the same store also serves the job queue.
type Store interface {
	ListPlans(ctx context.Context, limit, offset int) ([]Summary, error)
	GetPlan(ctx context.Context, id string) (PlanData, error)
	AcceptPlan(ctx context.Context, id string) (AcceptResult, error)
	// SaveVersion stores an edited or re-solved layout as the next version of
	// the source plan and archives the source.
	SaveVersion(ctx context.Context, src PlanData, problem core.Problem, sol core.Solution, name string) (PlanData, error)
}

// Editable reports whether a plan status can be edited or re-solved.
func Editable(status string) bool { return status == "draft" || status == "approved" }

// DetailOf converts the persistence view into the API response.
func DetailOf(data PlanData) Detail {
	return Detail{
		Summary: data.Summary,
		Rules:   data.Rules,
		Result: optimizer.Result{
			Solution:   data.Solution,
			Violations: data.Violations,
			Score:      data.Score,
			Cost:       data.Cost,
		},
	}
}
