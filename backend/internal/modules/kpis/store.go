// Package kpis reports realized-yield numbers across plans: how much material
// the shop actually consumed, how much of it became parts, and what the
// leftovers are worth. "Realized" means accepted plans; "created" covers every
// plan in the window, so the dashboard can compare the pipeline with what the
// shop committed to.
package kpis

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/size-module/backend/internal/platform/httpx"
)

// DefaultDays is the reporting window when none is given.
const DefaultDays = 90

// MaxDays caps the window so a report cannot scan the whole history by accident.
const MaxDays = 3650

// Bucket aggregates plans in a window.
type Bucket struct {
	Plans         int     `json:"plans"`
	Sheets        int     `json:"sheets"`
	PartsPlaced   int     `json:"partsPlaced"`
	RemnantSheets int     `json:"remnantSheets"`
	StockAreaM2   float64 `json:"stockAreaM2"`
	PartAreaM2    float64 `json:"partAreaM2"`
	ScrapAreaM2   float64 `json:"scrapAreaM2"`
	OffcutAreaM2  float64 `json:"offcutAreaM2"`
	YieldPct      float64 `json:"yieldPct"`
	WastePct      float64 `json:"wastePct"`
	Cost          float64 `json:"cost"`
	CostPerPart   float64 `json:"costPerPart"`
}

// SeriesPoint is one accepted plan on the trend line.
type SeriesPoint struct {
	PlanID      string    `json:"planId"`
	CreatedAt   time.Time `json:"createdAt"`
	YieldPct    float64   `json:"yieldPct"`
	WastePct    float64   `json:"wastePct"`
	Cost        float64   `json:"cost"`
	Sheets      int       `json:"sheets"`
	PartsPlaced int       `json:"partsPlaced"`
}

// Report is the dashboard payload.
type Report struct {
	From     time.Time     `json:"from"`
	To       time.Time     `json:"to"`
	Realized Bucket        `json:"realized"`
	Created  Bucket        `json:"created"`
	Series   []SeriesPoint `json:"series"`
}

// Store is implemented by the postgres package.
type Store interface {
	KPIs(ctx context.Context, from, to time.Time) (Report, error)
}

type Handler struct {
	store Store
}

func NewHandler(store Store) *Handler { return &Handler{store: store} }

func (h *Handler) Routes(r chi.Router) {
	r.Get("/kpis", h.get)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -DefaultDays)
	if raw := r.URL.Query().Get("days"); raw != "" {
		days, err := strconv.Atoi(raw)
		if err != nil || days < 1 || days > MaxDays {
			httpx.Error(w, http.StatusBadRequest, "invalid_filter", "days must be between 1 and 3650")
			return
		}
		from = to.AddDate(0, 0, -days)
	}
	if raw := r.URL.Query().Get("from"); raw != "" {
		parsed, err := parseTime(raw)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_filter", "from must be RFC3339 or YYYY-MM-DD")
			return
		}
		from = parsed
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		parsed, err := parseTime(raw)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_filter", "to must be RFC3339 or YYYY-MM-DD")
			return
		}
		to = parsed
	}
	if !from.Before(to) {
		httpx.Error(w, http.StatusBadRequest, "invalid_filter", "from must be before to")
		return
	}

	report, err := h.store.KPIs(r.Context(), from, to)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "kpis_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, report)
}

func parseTime(raw string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, errors.New("invalid time")
	}
	return parsed, nil
}
