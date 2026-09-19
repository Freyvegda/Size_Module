// Package catalog owns materials, material specs and stock formats.
package catalog

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/size-module/backend/internal/platform/httpx"
)

type Material struct {
	ID               string `json:"id"`
	Code             string `json:"code"`
	Name             string `json:"name"`
	DimensionProfile string `json:"dimensionProfile"`
	IsActive         bool   `json:"isActive"`
}

type StockFormat struct {
	ID               string  `json:"id"`
	Code             string  `json:"code"`
	MaterialCode     string  `json:"materialCode"`
	MaterialName     string  `json:"materialName"`
	SpecCode         string  `json:"specCode"`
	DimensionProfile string  `json:"dimensionProfile"`
	ThicknessMicron  int64   `json:"thicknessMicron"`
	LengthMicron     int64   `json:"lengthMicron"`
	WidthMicron      int64   `json:"widthMicron"`
	HeightMicron     int64   `json:"heightMicron"`
	OnHandQty        int32   `json:"onHandQty"`
	CostPerUnit      float64 `json:"costPerUnit"`
}

type CreateMaterialInput struct {
	Code             string         `json:"code"`
	Name             string         `json:"name"`
	DimensionProfile string         `json:"dimensionProfile"`
	Attributes       map[string]any `json:"attributes,omitempty"`
}

// Store is implemented by the postgres package. Handlers stay free of SQL types.
type Store interface {
	ListMaterials(ctx context.Context) ([]Material, error)
	CreateMaterial(ctx context.Context, in CreateMaterialInput) (Material, error)
	ListStockFormats(ctx context.Context) ([]StockFormat, error)
}

type Handler struct {
	store Store
}

func NewHandler(store Store) *Handler { return &Handler{store: store} }

func (h *Handler) Routes(r chi.Router) {
	r.Get("/materials", h.listMaterials)
	r.Post("/materials", h.createMaterial)
	r.Get("/stock-formats", h.listStockFormats)
}

func (h *Handler) listMaterials(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	materials, err := h.store.ListMaterials(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"materials": materials})
}

func (h *Handler) createMaterial(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	var in CreateMaterialInput
	if err := httpx.DecodeJSON(w, r, &in, 1<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if in.Code == "" || in.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "code and name are required")
		return
	}
	if in.DimensionProfile == "" {
		in.DimensionProfile = "2d"
	}
	material, err := h.store.CreateMaterial(r.Context(), in)
	if err != nil {
		httpx.Error(w, http.StatusConflict, "create_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, material)
}

func (h *Handler) listStockFormats(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	formats, err := h.store.ListStockFormats(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"stockFormats": formats})
}
