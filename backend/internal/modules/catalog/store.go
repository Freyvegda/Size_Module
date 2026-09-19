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

// MaterialSpec is a concrete variant of a family: "Oak 18 mm" under "Wood".
type MaterialSpec struct {
	ID               string `json:"id"`
	MaterialID       string `json:"materialId"`
	MaterialCode     string `json:"materialCode"`
	MaterialName     string `json:"materialName"`
	DimensionProfile string `json:"dimensionProfile"`
	Code             string `json:"code"`
	Name             string `json:"name"`
	ThicknessMicron  int64  `json:"thicknessMicron"`
	Finish           string `json:"finish"`
	Color            string `json:"color"`
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

type CreateMaterialSpecInput struct {
	MaterialID      string `json:"materialId"`
	Code            string `json:"code"`
	Name            string `json:"name,omitempty"`
	ThicknessMicron int64  `json:"thicknessMicron,omitempty"`
	Finish          string `json:"finish,omitempty"`
	Color           string `json:"color,omitempty"`
}

type CreateStockFormatInput struct {
	MaterialSpecID string  `json:"materialSpecId"`
	Code           string  `json:"code"`
	LengthMicron   int64   `json:"lengthMicron,omitempty"`
	WidthMicron    int64   `json:"widthMicron,omitempty"`
	HeightMicron   int64   `json:"heightMicron,omitempty"`
	OnHandQty      int32   `json:"onHandQty,omitempty"`
	CostPerUnit    float64 `json:"costPerUnit,omitempty"`
}

// Store is implemented by the postgres package. Handlers stay free of SQL types.
type Store interface {
	ListMaterials(ctx context.Context) ([]Material, error)
	CreateMaterial(ctx context.Context, in CreateMaterialInput) (Material, error)
	ListMaterialSpecs(ctx context.Context) ([]MaterialSpec, error)
	CreateMaterialSpec(ctx context.Context, in CreateMaterialSpecInput) (MaterialSpec, error)
	ListStockFormats(ctx context.Context) ([]StockFormat, error)
	CreateStockFormat(ctx context.Context, in CreateStockFormatInput) (StockFormat, error)
}

type Handler struct {
	store Store
}

func NewHandler(store Store) *Handler { return &Handler{store: store} }

func (h *Handler) Routes(r chi.Router) {
	r.Get("/materials", h.listMaterials)
	r.Post("/materials", h.createMaterial)
	r.Get("/material-specs", h.listMaterialSpecs)
	r.Post("/material-specs", h.createMaterialSpec)
	r.Get("/stock-formats", h.listStockFormats)
	r.Post("/stock-formats", h.createStockFormat)
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

func (h *Handler) listMaterialSpecs(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	specs, err := h.store.ListMaterialSpecs(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"materialSpecs": specs})
}

func (h *Handler) createMaterialSpec(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	var in CreateMaterialSpecInput
	if err := httpx.DecodeJSON(w, r, &in, 1<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if in.MaterialID == "" || in.Code == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "materialId and code are required")
		return
	}
	if in.ThicknessMicron < 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "thicknessMicron cannot be negative")
		return
	}
	spec, err := h.store.CreateMaterialSpec(r.Context(), in)
	if err != nil {
		httpx.Error(w, http.StatusConflict, "create_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, spec)
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

func (h *Handler) createStockFormat(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	var in CreateStockFormatInput
	if err := httpx.DecodeJSON(w, r, &in, 1<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if in.MaterialSpecID == "" || in.Code == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "materialSpecId and code are required")
		return
	}
	if in.LengthMicron < 0 || in.WidthMicron < 0 || in.HeightMicron < 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "dimensions cannot be negative")
		return
	}
	if in.LengthMicron == 0 && (in.WidthMicron == 0 || in.HeightMicron == 0) {
		httpx.Error(w, http.StatusBadRequest, "invalid_input",
			"a format needs a length (1d) or a width and height (2d)")
		return
	}
	if in.OnHandQty < 0 || in.CostPerUnit < 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "onHandQty and costPerUnit cannot be negative")
		return
	}
	format, err := h.store.CreateStockFormat(r.Context(), in)
	if err != nil {
		httpx.Error(w, http.StatusConflict, "create_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, format)
}
