// Package parts owns the part catalog: finished sizes plus the routings that
// turn them into cut sizes.
package parts

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/size-module/backend/internal/platform/httpx"
)

type Part struct {
	ID                 string `json:"id"`
	Code               string `json:"code"`
	Name               string `json:"name"`
	MaterialSpecID     string `json:"materialSpecId,omitempty"`
	FinishedLengthMicron int64  `json:"finishedLengthMicron"`
	FinishedWidthMicron  int64  `json:"finishedWidthMicron"`
	FinishedHeightMicron int64  `json:"finishedHeightMicron"`
	Grain              string `json:"grain"`
	AllowRotate        bool   `json:"allowRotate"`
	Priority           int32  `json:"priority"`
}

type CreatePartInput struct {
	Code                 string `json:"code"`
	Name                 string `json:"name"`
	MaterialSpecID       string `json:"materialSpecId,omitempty"`
	FinishedLengthMicron int64  `json:"finishedLengthMicron"`
	FinishedWidthMicron  int64  `json:"finishedWidthMicron"`
	FinishedHeightMicron int64  `json:"finishedHeightMicron"`
	Grain                string `json:"grain,omitempty"`
	AllowRotate          *bool  `json:"allowRotate,omitempty"`
	Priority             int32  `json:"priority,omitempty"`
}

type Store interface {
	ListParts(ctx context.Context) ([]Part, error)
	CreatePart(ctx context.Context, in CreatePartInput) (Part, error)
}

type Handler struct {
	store Store
}

func NewHandler(store Store) *Handler { return &Handler{store: store} }

func (h *Handler) Routes(r chi.Router) {
	r.Get("/parts", h.listParts)
	r.Post("/parts", h.createPart)
}

func (h *Handler) listParts(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	list, err := h.store.ListParts(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"parts": list})
}

func (h *Handler) createPart(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	var in CreatePartInput
	if err := httpx.DecodeJSON(w, r, &in, 1<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if in.Code == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "code is required")
		return
	}
	if in.Grain == "" {
		in.Grain = "none"
	}
	if in.Priority == 0 {
		in.Priority = 100
	}
	part, err := h.store.CreatePart(r.Context(), in)
	if err != nil {
		httpx.Error(w, http.StatusConflict, "create_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, part)
}
