// Package plans serves archived optimization plans and their acceptance.
//
// Accepting a plan is the point where a layout becomes shop reality: catalog
// on-hand quantities go down, physical pieces are consumed, and every reusable
// offcut is registered as a labelled remnant that later solves can pick up.
package plans

import (
	"context"
	"errors"
	"time"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
)

var (
	// ErrNotFound is returned when a plan id does not exist.
	ErrNotFound = errors.New("plan not found")
	// ErrNotAcceptable is returned when a plan is not draft or approved.
	ErrNotAcceptable = errors.New("plan is not in a state that can be accepted")
)

// Summary is one row of the plan list.
type Summary struct {
	ID            string       `json:"id"`
	JobID         string       `json:"jobId,omitempty"`
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
	Result optimizer.Result `json:"result"`
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
	GetPlan(ctx context.Context, id string) (Detail, error)
	AcceptPlan(ctx context.Context, id string) (AcceptResult, error)
}
