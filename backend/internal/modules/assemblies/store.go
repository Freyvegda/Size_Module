// Package assemblies owns products built from subparts: a window is an overall
// size plus components (glass panels, frame beams, mullions). The components
// are plain boxes in the assembly's local frame (origin bottom-left-front,
// x right, y up, z out of the wall), which is what the 2D elevation and the 3D
// view both render. Assemblies are a catalog definition; they are not cut by
// the optimizer yet.
package assemblies

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/size-module/backend/internal/platform/httpx"
)

// ErrNotFound is returned when an assembly id does not exist.
var ErrNotFound = errors.New("assembly not found")

// Component is one subpart of a product.
type Component struct {
	ID             string `json:"id"`
	Seq            int32  `json:"seq"`
	Role           string `json:"role"`
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	MaterialSpecID string `json:"materialSpecId,omitempty"`
	Quantity       int32  `json:"quantity"`
	WidthMicron    int64  `json:"widthMicron"`
	HeightMicron   int64  `json:"heightMicron"`
	DepthMicron    int64  `json:"depthMicron"`
	OffsetXMicron  int64  `json:"offsetXMicron"`
	OffsetYMicron  int64  `json:"offsetYMicron"`
	OffsetZMicron  int64  `json:"offsetZMicron"`
}

// Assembly is a product with its overall size and ordered components.
type Assembly struct {
	ID             string      `json:"id"`
	Code           string      `json:"code"`
	Name           string      `json:"name"`
	Kind           string      `json:"kind"`
	MaterialSpecID string      `json:"materialSpecId,omitempty"`
	WidthMicron    int64       `json:"widthMicron"`
	HeightMicron   int64       `json:"heightMicron"`
	DepthMicron    int64       `json:"depthMicron"`
	Components     []Component `json:"components"`
}

// CreateComponentInput defines one subpart on a new assembly.
type CreateComponentInput struct {
	Seq            int32  `json:"seq"`
	Role           string `json:"role,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Name           string `json:"name,omitempty"`
	MaterialSpecID string `json:"materialSpecId,omitempty"`
	Quantity       int32  `json:"quantity,omitempty"`
	WidthMicron    int64  `json:"widthMicron,omitempty"`
	HeightMicron   int64  `json:"heightMicron,omitempty"`
	DepthMicron    int64  `json:"depthMicron,omitempty"`
	OffsetXMicron  int64  `json:"offsetXMicron,omitempty"`
	OffsetYMicron  int64  `json:"offsetYMicron,omitempty"`
	OffsetZMicron  int64  `json:"offsetZMicron,omitempty"`
}

// CreateAssemblyInput is the body of POST /assemblies.
type CreateAssemblyInput struct {
	Code           string                 `json:"code"`
	Name           string                 `json:"name,omitempty"`
	Kind           string                 `json:"kind,omitempty"`
	MaterialSpecID string                 `json:"materialSpecId,omitempty"`
	WidthMicron    int64                  `json:"widthMicron,omitempty"`
	HeightMicron   int64                  `json:"heightMicron,omitempty"`
	DepthMicron    int64                  `json:"depthMicron,omitempty"`
	Components     []CreateComponentInput `json:"components,omitempty"`
}

// Store is implemented by the postgres package.
type Store interface {
	ListAssemblies(ctx context.Context) ([]Assembly, error)
	GetAssembly(ctx context.Context, id string) (Assembly, error)
	CreateAssembly(ctx context.Context, in CreateAssemblyInput) (Assembly, error)
}

type Handler struct {
	store Store
}

func NewHandler(store Store) *Handler { return &Handler{store: store} }

func (h *Handler) Routes(r chi.Router) {
	r.Get("/assemblies", h.list)
	r.Post("/assemblies", h.create)
	r.Get("/assemblies/{id}", h.get)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	list, err := h.store.ListAssemblies(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"assemblies": list})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	assembly, err := h.store.GetAssembly(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not_found", "unknown assembly id")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "lookup_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, assembly)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	var in CreateAssemblyInput
	if err := httpx.DecodeJSON(w, r, &in, 1<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if in.Code == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "code is required")
		return
	}
	if in.WidthMicron < 0 || in.HeightMicron < 0 || in.DepthMicron < 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "dimensions cannot be negative")
		return
	}
	if in.WidthMicron == 0 && in.HeightMicron == 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "set an overall width or height")
		return
	}
	if in.Kind == "" {
		in.Kind = "window"
	}
	if in.Kind != "window" && in.Kind != "door" && in.Kind != "generic" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "kind must be window, door or generic")
		return
	}
	for i := range in.Components {
		c := &in.Components[i]
		if c.Role == "" {
			c.Role = "part"
		}
		if c.Kind == "" {
			c.Kind = "panel"
		}
		if c.Kind != "beam" && c.Kind != "panel" && c.Kind != "custom" {
			httpx.Error(w, http.StatusBadRequest, "invalid_input", "component kind must be beam, panel or custom")
			return
		}
		if c.WidthMicron < 0 || c.HeightMicron < 0 || c.DepthMicron < 0 {
			httpx.Error(w, http.StatusBadRequest, "invalid_input", "component dimensions cannot be negative")
			return
		}
		if c.Quantity <= 0 {
			c.Quantity = 1
		}
		if c.Seq <= 0 {
			c.Seq = int32(i + 1)
		}
	}

	assembly, err := h.store.CreateAssembly(r.Context(), in)
	if err != nil {
		httpx.Error(w, http.StatusConflict, "create_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, assembly)
}
