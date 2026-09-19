// Package campaigns plans an ordered set of jobs against one shared stock
// budget. Running an item consumes the sheets and remnants it uses and returns
// its offcuts to the campaign pool, so the items that follow can reuse them —
// which is exactly how a shop batches orders ("campaign") through the saw.
//
// A campaign is a planning sandbox: accepting a plan is still what changes the
// plant's real stock. The budget is the "what is left to plan with" view.
package campaigns

import (
	"context"
	"errors"
	"time"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
)

var (
	// ErrNotFound is returned for unknown campaigns or items.
	ErrNotFound = errors.New("campaign not found")
	// ErrNotEditable is returned when a campaign is completed or cancelled.
	ErrNotEditable = errors.New("campaign is completed or cancelled")
	// ErrNoPendingItem is returned when every item has already been planned.
	ErrNoPendingItem = errors.New("campaign has no pending items")
)

// Statuses a campaign moves through.
const (
	StatusDraft     = "draft"
	StatusActive    = "active"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
)

// Campaign is the shared planning context plus its remaining stock budget.
type Campaign struct {
	ID           string           `json:"id"`
	Code         string           `json:"code"`
	Name         string           `json:"name"`
	Status       string           `json:"status"`
	Rules        core.Rules       `json:"rules"`
	Objective    core.Objective   `json:"objective"`
	BudgetMS     int              `json:"budgetMs"`
	Seed         uint64           `json:"seed"`
	Stock        []core.StockItem `json:"stock"`
	InitialStock []core.StockItem `json:"initialStock"`
	CreatedAt    time.Time        `json:"createdAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
	CompletedAt  *time.Time       `json:"completedAt,omitempty"`
}

// Item is one job inside a campaign.
type Item struct {
	ID       string      `json:"id"`
	Seq      int         `json:"seq"`
	Name     string      `json:"name"`
	Parts    []core.Part `json:"parts"`
	DueDate  *time.Time  `json:"dueDate,omitempty"`
	Priority int         `json:"priority"`
	Status   string      `json:"status"`
	PlanID   string      `json:"planId,omitempty"`
	JobID    string      `json:"jobId,omitempty"`
	Error    string      `json:"error,omitempty"`
}

// Progress summarises how far a campaign has come.
type Progress struct {
	Items   int `json:"items"`
	Pending int `json:"pending"`
	Planned int `json:"planned"`
}

// Detail is a campaign with its items and progress.
type Detail struct {
	Campaign
	Items    []Item   `json:"items"`
	Progress Progress `json:"progress"`
}

// CreateInput is the body of POST /campaigns.
type CreateInput struct {
	Code        string           `json:"code,omitempty"`
	Name        string           `json:"name"`
	BudgetMS    int              `json:"budgetMs,omitempty"`
	Seed        uint64           `json:"seed,omitempty"`
	Rules       core.Rules       `json:"rules,omitempty"`
	Objective   core.Objective   `json:"objective,omitempty"`
	Stock       []core.StockItem `json:"stock,omitempty"`
	UseRemnants bool             `json:"useRemnants,omitempty"`
}

// UpdateInput is the body of PATCH /campaigns/{id}.
type UpdateInput struct {
	Name   *string `json:"name,omitempty"`
	Status *string `json:"status,omitempty"`
}

// CreateItemInput is the body of POST /campaigns/{id}/items.
type CreateItemInput struct {
	Name     string      `json:"name"`
	Parts    []core.Part `json:"parts"`
	DueDate  string      `json:"dueDate,omitempty"` // YYYY-MM-DD
	Priority int         `json:"priority,omitempty"`
}

// RunNextRequest is the body of POST /campaigns/{id}/run-next.
type RunNextRequest struct {
	Solver   string `json:"solver,omitempty"`
	BudgetMS int    `json:"budgetMs,omitempty"`
}

// Store is implemented by the postgres package.
type Store interface {
	ListCampaigns(ctx context.Context, limit, offset int) ([]Campaign, error)
	GetCampaign(ctx context.Context, id string) (Detail, error)
	CreateCampaign(ctx context.Context, in CreateInput) (Detail, error)
	UpdateCampaign(ctx context.Context, id string, in UpdateInput) (Detail, error)
	AddItem(ctx context.Context, campaignID string, in CreateItemInput) (Detail, error)
	DeleteItem(ctx context.Context, campaignID, itemID string) (Detail, error)
	// NextItem returns the earliest pending item.
	NextItem(ctx context.Context, campaignID string) (Item, error)
	// CompleteItem archives the solved item as a job + plan version and stores
	// the remaining stock budget in one transaction.
	CompleteItem(ctx context.Context, campaignID, itemID string, problem core.Problem, result optimizer.Result, stock []core.StockItem) (Detail, error)
}

// Editable reports whether a campaign status still accepts items and runs.
func Editable(status string) bool { return status == StatusDraft || status == StatusActive }
