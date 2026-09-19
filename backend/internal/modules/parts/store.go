// Package parts owns the part catalog: finished sizes plus the routings that
// turn them into cut sizes.
//
// A part is always defined by its **finished** size. The solver only ever sees
// the **cut** size: finished size + the allowances of every routing operation.
// CutSize below is the single place that conversion happens, so no caller can
// type a cut size twice (or forget an allowance).
package parts

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/size-module/backend/internal/platform/httpx"
)

// Routing is one manufacturing operation applied to a part. AllowanceMicron is
// the material the operation needs: with AllowancePerEdge it is added on both
// sides of every active dimension (2D: width and height; 1D: both ends), a
// non-per-edge allowance is added once.
type Routing struct {
	Seq              int32  `json:"seq"`
	Operation        string `json:"operation"`
	AllowanceMicron  int64  `json:"allowanceMicron"`
	AllowancePerEdge bool   `json:"allowancePerEdge"`
	Notes            string `json:"notes,omitempty"`
}

// Part is a catalog part. Finished sizes are what the drawing says; cut sizes
// are derived from the routings and are what the solver cuts.
type Part struct {
	ID                   string    `json:"id"`
	Code                 string    `json:"code"`
	Name                 string    `json:"name"`
	MaterialSpecID       string    `json:"materialSpecId,omitempty"`
	FinishedLengthMicron int64     `json:"finishedLengthMicron"`
	FinishedWidthMicron  int64     `json:"finishedWidthMicron"`
	FinishedHeightMicron int64     `json:"finishedHeightMicron"`
	CutLengthMicron      int64     `json:"cutLengthMicron"`
	CutWidthMicron       int64     `json:"cutWidthMicron"`
	CutHeightMicron      int64     `json:"cutHeightMicron"`
	Grain                string    `json:"grain"`
	AllowRotate          bool      `json:"allowRotate"`
	Priority             int32     `json:"priority"`
	Routings             []Routing `json:"routings"`
}

// CreateRoutingInput defines one operation when a part is created.
type CreateRoutingInput struct {
	Seq              int32  `json:"seq,omitempty"`
	Operation        string `json:"operation"`
	AllowanceMicron  int64  `json:"allowanceMicron,omitempty"`
	AllowancePerEdge bool   `json:"allowancePerEdge,omitempty"`
	Notes            string `json:"notes,omitempty"`
}

type CreatePartInput struct {
	Code                 string               `json:"code"`
	Name                 string               `json:"name"`
	MaterialSpecID       string               `json:"materialSpecId,omitempty"`
	FinishedLengthMicron int64                `json:"finishedLengthMicron"`
	FinishedWidthMicron  int64                `json:"finishedWidthMicron"`
	FinishedHeightMicron int64                `json:"finishedHeightMicron"`
	Grain                string               `json:"grain,omitempty"`
	AllowRotate          *bool                `json:"allowRotate,omitempty"`
	Priority             int32                `json:"priority,omitempty"`
	Routings             []CreateRoutingInput `json:"routings,omitempty"`
}

// Store is implemented by the postgres package.
type Store interface {
	ListParts(ctx context.Context) ([]Part, error)
	CreatePart(ctx context.Context, in CreatePartInput) (Part, error)
}

// CutSize applies routing allowances to a finished size. A per-edge allowance
// grows both sides of every active dimension; a total allowance grows it once.
// A 1D part (Length > 0) only grows along its length.
func CutSize(finishedLength, finishedWidth, finishedHeight int64, routings []Routing) (length, width, height int64) {
	length, width, height = finishedLength, finishedWidth, finishedHeight
	is1D := finishedLength > 0 && finishedWidth == 0 && finishedHeight == 0
	for _, r := range routings {
		if r.AllowanceMicron <= 0 {
			continue
		}
		allowance := r.AllowanceMicron
		if r.AllowancePerEdge {
			allowance *= 2
		}
		if is1D {
			length += allowance
			continue
		}
		width += allowance
		height += allowance
	}
	return length, width, height
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
	if in.FinishedLengthMicron < 0 || in.FinishedWidthMicron < 0 || in.FinishedHeightMicron < 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "finished dimensions cannot be negative")
		return
	}
	if in.FinishedLengthMicron == 0 && (in.FinishedWidthMicron == 0 || in.FinishedHeightMicron == 0) {
		httpx.Error(w, http.StatusBadRequest, "invalid_input",
			"set a finished length, or both a finished width and height")
		return
	}
	if in.Grain == "" {
		in.Grain = "none"
	}
	if in.Priority == 0 {
		in.Priority = 100
	}
	for i := range in.Routings {
		rt := &in.Routings[i]
		if rt.Seq <= 0 {
			rt.Seq = int32(i + 1)
		}
		if rt.AllowanceMicron < 0 {
			httpx.Error(w, http.StatusBadRequest, "invalid_input", "allowances cannot be negative")
			return
		}
	}
	part, err := h.store.CreatePart(r.Context(), in)
	if err != nil {
		httpx.Error(w, http.StatusConflict, "create_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, part)
}
