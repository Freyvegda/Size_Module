package campaigns

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/httpx"
	"github.com/size-module/backend/internal/platform/id"
)

// DefaultLimit is the page size of GET /campaigns when none is given.
const DefaultLimit = 50

// SolverRunner runs an optimization problem; jobs.Service implements it.
type SolverRunner interface {
	Run(ctx context.Context, p core.Problem, solver string, progress core.ProgressFunc) (optimizer.Result, error)
}

// RemnantSource supplies the plant's available remnants when a campaign asks
// for them at creation time. It is optional.
type RemnantSource interface {
	ListRemnants(ctx context.Context, materialSpecID string) ([]core.StockItem, error)
}

type Handler struct {
	store    Store
	runner   SolverRunner
	remnants RemnantSource
}

func NewHandler(store Store, runner SolverRunner, remnants RemnantSource) *Handler {
	return &Handler{store: store, runner: runner, remnants: remnants}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/campaigns", h.list)
	r.Post("/campaigns", h.create)
	r.Get("/campaigns/{id}", h.get)
	r.Patch("/campaigns/{id}", h.update)
	r.Post("/campaigns/{id}/items", h.addItem)
	r.Delete("/campaigns/{id}/items/{itemID}", h.deleteItem)
	r.Post("/campaigns/{id}/run-next", h.runNext)
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
	items, err := h.store.ListCampaigns(r.Context(), limit, offset)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"campaigns": items})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	detail, err := h.store.GetCampaign(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.storeError(w, err, "lookup_failed")
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	var in CreateInput
	if err := httpx.DecodeJSON(w, r, &in, 4<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "name is required")
		return
	}
	if in.UseRemnants && h.remnants != nil {
		items, err := h.remnants.ListRemnants(r.Context(), "")
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "remnant_lookup_failed", err.Error())
			return
		}
		seen := map[string]bool{}
		for _, entry := range in.Stock {
			seen[entry.ID] = true
		}
		for _, item := range items {
			if !seen[item.ID] {
				in.Stock = append(in.Stock, item)
			}
		}
	}
	if in.BudgetMS <= 0 {
		in.BudgetMS = 5000
	}
	detail, err := h.store.CreateCampaign(r.Context(), in)
	if err != nil {
		httpx.Error(w, http.StatusConflict, "create_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, detail)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	var in UpdateInput
	if err := httpx.DecodeJSON(w, r, &in, 1<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			httpx.Error(w, http.StatusBadRequest, "invalid_input", "name cannot be emptied")
			return
		}
		in.Name = &name
	}
	if in.Status != nil {
		switch *in.Status {
		case StatusDraft, StatusActive, StatusCompleted, StatusCancelled:
		default:
			httpx.Error(w, http.StatusBadRequest, "invalid_input",
				"status must be draft, active, completed or cancelled")
			return
		}
	}
	detail, err := h.store.UpdateCampaign(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		h.storeError(w, err, "update_failed")
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

func (h *Handler) addItem(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	var in CreateItemInput
	if err := httpx.DecodeJSON(w, r, &in, 4<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "name is required")
		return
	}
	if len(in.Parts) == 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "an item needs at least one part")
		return
	}
	if _, _, err := itemMaterial(Item{Parts: in.Parts}); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_item", err.Error())
		return
	}
	detail, err := h.store.AddItem(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		h.storeError(w, err, "add_item_failed")
		return
	}
	httpx.JSON(w, http.StatusCreated, detail)
}

func (h *Handler) deleteItem(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	detail, err := h.store.DeleteItem(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "itemID"))
	if err != nil {
		h.storeError(w, err, "delete_item_failed")
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

// runNext solves the earliest pending item against the remaining budget,
// stores the plan and returns the campaign with its updated stock.
func (h *Handler) runNext(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	if h.runner == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "solver_unavailable", "the solver is not configured")
		return
	}
	req := RunNextRequest{}
	if r.ContentLength > 0 {
		if err := httpx.DecodeJSON(w, r, &req, 1<<20); err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
	}

	detail, err := h.store.GetCampaign(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.storeError(w, err, "lookup_failed")
		return
	}
	if !Editable(detail.Status) {
		httpx.Error(w, http.StatusConflict, "not_editable", ErrNotEditable.Error())
		return
	}
	item, err := h.store.NextItem(r.Context(), detail.ID)
	if err != nil {
		h.storeError(w, err, "lookup_failed")
		return
	}

	problem, err := BuildItemProblem(detail.Campaign, item, req.BudgetMS)
	if err != nil {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_item", err.Error())
		return
	}
	budget := problem.BudgetMS
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(budget)*time.Millisecond+10*time.Second)
	defer cancel()

	result, err := h.runner.Run(ctx, problem, req.Solver, nil)
	if err != nil {
		httpx.Error(w, http.StatusUnprocessableEntity, "optimization_failed", err.Error())
		return
	}

	// Offcuts get real physical ids here: the budget references them and
	// CompleteItem registers them as available stock_items, so accepting a plan
	// that used one actually consumes it instead of silently missing it.
	newRemnantIDs := map[string]string{}
	stock := ConsumeBudgetWithIDs(detail.Stock, result.Solution, problem.Rules,
		func(sheetIndex, offcutIndex int) string {
			return RemnantLabel(detail.ID, item.Seq, sheetIndex, offcutIndex)
		},
		func(sheetIndex, offcutIndex int) string {
			key := strconv.Itoa(sheetIndex) + ":" + strconv.Itoa(offcutIndex)
			if existing, ok := newRemnantIDs[key]; ok {
				return existing
			}
			created := id.New()
			newRemnantIDs[key] = created
			return created
		})

	updated, err := h.store.CompleteItem(r.Context(), detail.ID, item.ID, problem, result, stock)
	if err != nil {
		h.storeError(w, err, "complete_failed")
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

// itemMaterial reports the single material and dimension profile an item is
// about. An item that mixes materials or dimensions cannot be solved as one
// problem, so it is rejected instead of being silently half-planned.
func itemMaterial(item Item) (material, profile string, err error) {
	has2D, has1D := false, false
	for _, p := range item.Parts {
		switch {
		case p.Width > 0 && p.Height > 0:
			has2D = true
		case p.Length > 0:
			has1D = true
		}
		if p.MaterialSpecID != "" {
			if material != "" && material != p.MaterialSpecID {
				return "", "", errors.New("an item must use a single material")
			}
			material = p.MaterialSpecID
		}
	}
	switch {
	case has2D && has1D:
		return "", "", errors.New("an item must contain parts of a single dimension profile")
	case has2D:
		profile = "2d"
	case has1D:
		profile = "1d"
	}
	return material, profile, nil
}

// BuildItemProblem turns one campaign item into a self-contained problem: the
// item's parts, the campaign's remaining stock budget for the item's material,
// its rules/objective and a per-item seed so a campaign is reproducible.
func BuildItemProblem(c Campaign, item Item, budgetMS int) (core.Problem, error) {
	if budgetMS <= 0 {
		budgetMS = c.BudgetMS
	}
	material, _, err := itemMaterial(item)
	if err != nil {
		return core.Problem{}, err
	}
	stock := make([]core.StockItem, 0, len(c.Stock))
	for _, entry := range c.Stock {
		if material != "" && entry.MaterialSpecID != "" && entry.MaterialSpecID != material {
			continue
		}
		stock = append(stock, entry)
	}
	problem := core.Normalize(core.Problem{
		Parts:     item.Parts,
		Stocks:    stock,
		Rules:     c.Rules,
		Objective: c.Objective,
		BudgetMS:  budgetMS,
		Seed:      c.Seed + uint64(item.Seq),
	})
	if len(problem.Stocks) == 0 {
		return core.Problem{}, errors.New("campaign budget has no stock for the item's material")
	}
	return problem, nil
}

// storeError maps the module's sentinel errors onto HTTP responses.
func (h *Handler) storeError(w http.ResponseWriter, err error, fallbackCode string) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "unknown campaign or item")
	case errors.Is(err, ErrNoPendingItem):
		httpx.Error(w, http.StatusConflict, "no_pending_items", ErrNoPendingItem.Error())
	case errors.Is(err, ErrNotEditable):
		httpx.Error(w, http.StatusConflict, "not_editable", ErrNotEditable.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, fallbackCode, err.Error())
	}
}
