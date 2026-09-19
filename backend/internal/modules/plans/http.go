package plans

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/export"
	"github.com/size-module/backend/internal/optimizer/validator"
	"github.com/size-module/backend/internal/platform/httpx"
)

// DefaultLimit is the page size of GET /plans when none is given.
const DefaultLimit = 50

// SolverRunner runs an optimization problem; jobs.Service implements it.
type SolverRunner interface {
	Run(ctx context.Context, p core.Problem, solver string, progress core.ProgressFunc) (optimizer.Result, error)
}

type Handler struct {
	store  Store
	runner SolverRunner
}

func NewHandler(store Store, runner SolverRunner) *Handler {
	return &Handler{store: store, runner: runner}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/plans", h.list)
	r.Get("/plans/{id}", h.get)
	r.Post("/plans/{id}/accept", h.accept)
	r.Post("/plans/{id}/edit", h.edit)
	r.Post("/plans/{id}/reoptimize", h.reoptimize)
	r.Get("/plans/{id}/exports", h.export)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	limit := DefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			httpx.Error(w, http.StatusBadRequest, "invalid_filter", "limit must be between 1 and 500")
			return
		}
		limit = parsed
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			httpx.Error(w, http.StatusBadRequest, "invalid_filter", "offset must be zero or positive")
			return
		}
		offset = parsed
	}
	items, err := h.store.ListPlans(r.Context(), limit, offset)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"plans": items})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	data, err := h.store.GetPlan(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.storeError(w, err, "lookup_failed")
		return
	}
	httpx.JSON(w, http.StatusOK, DetailOf(data))
}

func (h *Handler) accept(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	result, err := h.store.AcceptPlan(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.storeError(w, err, "accept_failed")
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

// edit applies hand edits and stores them as the next plan version.
func (h *Handler) edit(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	var req EditRequest
	if err := httpx.DecodeJSON(w, r, &req, 4<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	data, err := h.store.GetPlan(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.storeError(w, err, "lookup_failed")
		return
	}
	if !Editable(data.Status) {
		httpx.Error(w, http.StatusConflict, "not_editable", ErrNotEditable.Error())
		return
	}
	if data.Problem == nil {
		httpx.Error(w, http.StatusConflict, "no_problem", ErrNoProblem.Error())
		return
	}

	sol := data.Solution
	applied, violations, err := ApplyOperations(&sol, *data.Problem, req.Operations)
	if err != nil {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_operation", err.Error())
		return
	}
	Finalize(*data.Problem, &sol)
	violations = append(violations, validator.Validate(*data.Problem, sol)...)
	if HasErrors(violations) {
		httpx.JSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":      map[string]string{"code": "edit_invalid", "message": "the edited layout is not manufacturable"},
			"violations": violations,
		})
		return
	}

	sol.Solver = "manual-edit"
	sol.SolverVersion = "1"
	sol.Notes = append(sol.Notes, fmt.Sprintf("Edited by hand: %d operation(s), %d locked placement(s).",
		applied, countLocked(sol)))

	saved, err := h.store.SaveVersion(r.Context(), data, *data.Problem, sol, req.Name)
	if err != nil {
		h.storeError(w, err, "save_failed")
		return
	}
	httpx.JSON(w, http.StatusOK, DetailOf(saved))
}

// reoptimize re-solves a plan, keeping locked placements exactly in place.
func (h *Handler) reoptimize(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	if h.runner == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "solver_unavailable", "the solver is not configured")
		return
	}
	var req ReoptimizeRequest
	if err := httpx.DecodeJSON(w, r, &req, 4<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	data, err := h.store.GetPlan(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.storeError(w, err, "lookup_failed")
		return
	}
	if !Editable(data.Status) {
		httpx.Error(w, http.StatusConflict, "not_editable", ErrNotEditable.Error())
		return
	}
	if data.Problem == nil {
		httpx.Error(w, http.StatusConflict, "no_problem", ErrNoProblem.Error())
		return
	}

	sol := data.Solution
	if len(req.Operations) > 0 {
		_, violations, err := ApplyOperations(&sol, *data.Problem, req.Operations)
		if err != nil {
			httpx.Error(w, http.StatusUnprocessableEntity, "invalid_operation", err.Error())
			return
		}
		Finalize(*data.Problem, &sol)
		violations = append(violations, validator.Validate(*data.Problem, sol)...)
		if HasErrors(violations) {
			httpx.JSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error":      map[string]string{"code": "edit_invalid", "message": "the edited layout is not manufacturable"},
				"violations": violations,
			})
			return
		}
	}

	problem := *data.Problem
	if req.BudgetMS > 0 {
		problem.BudgetMS = req.BudgetMS
	}
	problem = BuildPinnedProblem(problem, sol)

	budget := problem.BudgetMS
	if budget <= 0 {
		budget = 5000
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(budget)*time.Millisecond+10*time.Second)
	defer cancel()

	result, err := h.runner.Run(ctx, problem, req.Solver, nil)
	if err != nil {
		httpx.Error(w, http.StatusUnprocessableEntity, "optimization_failed", err.Error())
		return
	}
	if len(problem.Pinned) > 0 {
		result.Solution.Notes = append(result.Solution.Notes,
			fmt.Sprintf("Re-solved around %d pinned sheet(s) with %d locked placement(s).",
				len(problem.Pinned), countLocked(result.Solution)))
	}

	saved, err := h.store.SaveVersion(r.Context(), data, problem, result.Solution, req.Name)
	if err != nil {
		h.storeError(w, err, "save_failed")
		return
	}
	httpx.JSON(w, http.StatusOK, DetailOf(saved))
}

// export streams a plan as CSV, SVG, DXF or PDF.
func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	format := export.Format(r.URL.Query().Get("format"))
	if !export.Supported(string(format)) {
		httpx.Error(w, http.StatusBadRequest, "invalid_format", "format must be one of csv, svg, dxf, pdf")
		return
	}
	opts := export.Options{}
	if raw := r.URL.Query().Get("sheet"); raw != "" && raw != "all" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			httpx.Error(w, http.StatusBadRequest, "invalid_filter", "sheet must be a zero-based sheet index or 'all'")
			return
		}
		opts.Sheet = &parsed
	}

	data, err := h.store.GetPlan(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.storeError(w, err, "lookup_failed")
		return
	}
	var buf bytes.Buffer
	if err := export.Write(&buf, format, data.Solution, opts); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "export_failed", err.Error())
		return
	}

	filename := fmt.Sprintf("plan-%s.%s", shortID(data.ID), export.Extension(format))
	w.Header().Set("Content-Type", export.ContentType(format))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// storeError maps the module's sentinel errors onto HTTP responses.
func (h *Handler) storeError(w http.ResponseWriter, err error, fallbackCode string) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "unknown plan id")
	case errors.Is(err, ErrNotAcceptable):
		httpx.Error(w, http.StatusConflict, "not_acceptable",
			"only draft or approved plans can be accepted; this plan was already accepted or archived")
	case errors.Is(err, ErrNotEditable):
		httpx.Error(w, http.StatusConflict, "not_editable",
			"only draft or approved plans can be edited; this plan was accepted or archived")
	case errors.Is(err, ErrNoProblem):
		httpx.Error(w, http.StatusConflict, "no_problem", ErrNoProblem.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, fallbackCode, err.Error())
	}
}

func countLocked(sol core.Solution) int {
	count := 0
	for _, sheet := range sol.Sheets {
		for _, pl := range sheet.Placements {
			if pl.Locked {
				count++
			}
		}
	}
	return count
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
