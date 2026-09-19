package plans

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/size-module/backend/internal/platform/httpx"
)

// DefaultLimit is the page size of GET /plans when none is given.
const DefaultLimit = 50

type Handler struct {
	store Store
}

func NewHandler(store Store) *Handler { return &Handler{store: store} }

func (h *Handler) Routes(r chi.Router) {
	r.Get("/plans", h.list)
	r.Get("/plans/{id}", h.get)
	r.Post("/plans/{id}/accept", h.accept)
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
	detail, err := h.store.GetPlan(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not_found", "unknown plan id")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "lookup_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

func (h *Handler) accept(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	result, err := h.store.AcceptPlan(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			httpx.Error(w, http.StatusNotFound, "not_found", "unknown plan id")
		case errors.Is(err, ErrNotAcceptable):
			httpx.Error(w, http.StatusConflict, "not_acceptable",
				"only draft or approved plans can be accepted; this plan was already accepted or archived")
		default:
			httpx.Error(w, http.StatusInternalServerError, "accept_failed", err.Error())
		}
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}
