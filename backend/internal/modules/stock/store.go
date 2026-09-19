// Package stock owns physical stock pieces: full sheets and bars cut from a
// catalog format, and the labelled remnants left over after a plan is accepted.
//
// A catalog stock_format is a *size* with an on-hand quantity; a stock item is
// one *physical piece* that can be scanned, moved and consumed exactly once.
package stock

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/httpx"
)

// Statuses of a physical piece. Consumed is set by plan acceptance only.
const (
	StatusAvailable = "available"
	StatusReserved  = "reserved"
	StatusConsumed  = "consumed"
	StatusRetired   = "retired"
)

// ErrNotFound is returned when a stock item id does not exist.
var ErrNotFound = errors.New("stock item not found")

// Item is one physical piece in the shop.
type Item struct {
	ID               string     `json:"id"`
	FormatID         string     `json:"formatId,omitempty"`
	FormatCode       string     `json:"formatCode,omitempty"`
	Code             string     `json:"code"`
	Label            string     `json:"label"`
	MaterialCode     string     `json:"materialCode,omitempty"`
	SpecCode         string     `json:"specCode,omitempty"`
	DimensionProfile string     `json:"dimensionProfile,omitempty"`
	LengthMicron     int64      `json:"lengthMicron,omitempty"`
	WidthMicron      int64      `json:"widthMicron,omitempty"`
	HeightMicron     int64      `json:"heightMicron,omitempty"`
	IsRemnant        bool       `json:"isRemnant"`
	Status           string     `json:"status"`
	Location         string     `json:"location"`
	CostPerUnit      float64    `json:"costPerUnit"`
	Notes            string     `json:"notes,omitempty"`
	ParentPlanID     string     `json:"parentPlanId,omitempty"`
	ParentSheetIndex *int32     `json:"parentSheetIndex,omitempty"`
	ConsumedByPlanID string     `json:"consumedByPlanId,omitempty"`
	ConsumedAt       *time.Time `json:"consumedAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	// Defects are unusable regions of the piece (knots, cracks, scratches).
	// Solvers avoid them and the validator rejects any piece placed over one.
	Defects []core.Rect `json:"defects,omitempty"`
}

// ListFilter narrows the stock pool listing.
type ListFilter struct {
	Status    string
	IsRemnant *bool
}

// CreateInput registers a physical piece. Dimensions are micrometers; a piece
// is either 1D (length) or 2D (width and height).
type CreateInput struct {
	FormatID     string  `json:"formatId"`
	Code         string  `json:"code"`
	Label        string  `json:"label"`
	LengthMicron int64   `json:"lengthMicron"`
	WidthMicron  int64   `json:"widthMicron"`
	HeightMicron int64   `json:"heightMicron"`
	IsRemnant    *bool   `json:"isRemnant"`
	Location     string  `json:"location"`
	CostPerUnit  float64 `json:"costPerUnit"`
	Notes        string  `json:"notes"`
	// Defects are unusable regions of the piece, in piece-local coordinates.
	Defects []core.Rect `json:"defects,omitempty"`
}

// UpdateInput patches the mutable fields of a piece. Nil fields are left
// unchanged; status may only move between the pool states.
type UpdateInput struct {
	Label    *string `json:"label"`
	Location *string `json:"location"`
	Status   *string `json:"status"`
	Notes    *string `json:"notes"`
	// Defects replaces the piece's defect map when present (use an empty array
	// to clear it). Omitting the field leaves the current defects unchanged.
	Defects *[]core.Rect `json:"defects,omitempty"`
}

// Store is implemented by the postgres package.
type Store interface {
	ListItems(ctx context.Context, filter ListFilter) ([]Item, error)
	GetItem(ctx context.Context, id string) (Item, error)
	CreateItem(ctx context.Context, in CreateInput) (Item, error)
	UpdateItem(ctx context.Context, id string, in UpdateInput) (Item, error)
}

type Handler struct {
	store Store
}

func NewHandler(store Store) *Handler { return &Handler{store: store} }

func (h *Handler) Routes(r chi.Router) {
	r.Get("/stock-items", h.list)
	r.Post("/stock-items", h.create)
	r.Get("/stock-items/{id}", h.get)
	r.Patch("/stock-items/{id}", h.update)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	filter := ListFilter{Status: r.URL.Query().Get("status")}
	if filter.Status != "" && !knownStatus(filter.Status) {
		httpx.Error(w, http.StatusBadRequest, "invalid_filter", "unknown status "+filter.Status)
		return
	}
	if raw := r.URL.Query().Get("isRemnant"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_filter", "isRemnant must be true or false")
			return
		}
		filter.IsRemnant = &value
	}
	items, err := h.store.ListItems(r.Context(), filter)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"stockItems": items})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	item, err := h.store.GetItem(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not_found", "unknown stock item id")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "lookup_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	var in CreateInput
	if err := httpx.DecodeJSON(w, r, &in, 1<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "label is required: it is the shop-floor identifier of the piece")
		return
	}
	if !validDimensions(in.LengthMicron, in.WidthMicron, in.HeightMicron) {
		httpx.Error(w, http.StatusBadRequest, "invalid_input",
			"a piece needs a positive length (1d) or a positive width and height (2d)")
		return
	}
	if err := ValidateDefects(in.Defects, in.WidthMicron, in.HeightMicron); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", err.Error())
		return
	}
	if in.CostPerUnit < 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "costPerUnit cannot be negative")
		return
	}
	item, err := h.store.CreateItem(r.Context(), in)
	if err != nil {
		httpx.Error(w, http.StatusConflict, "create_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, item)
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
	if in.Label != nil {
		label := strings.TrimSpace(*in.Label)
		if label == "" {
			httpx.Error(w, http.StatusBadRequest, "invalid_input", "label cannot be emptied")
			return
		}
		in.Label = &label
	}
	if in.Status != nil {
		if !knownStatus(*in.Status) || *in.Status == StatusConsumed {
			httpx.Error(w, http.StatusBadRequest, "invalid_input",
				"status must be available, reserved or retired; consumed is set by accepting a plan")
			return
		}
	}
	item, err := h.store.UpdateItem(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not_found", "unknown stock item id")
			return
		}
		httpx.Error(w, http.StatusConflict, "update_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func knownStatus(status string) bool {
	switch status {
	case StatusAvailable, StatusReserved, StatusConsumed, StatusRetired:
		return true
	default:
		return false
	}
}

func validDimensions(length, width, height int64) bool {
	return length > 0 || (width > 0 && height > 0)
}

// ValidateDefects checks that every defect rectangle is well formed, and — when
// the piece dimensions are known — that it lies inside the piece. A defect is a
// 2D region: a 1D piece (a bar) cannot carry one.
func ValidateDefects(defects []core.Rect, width, height int64) error {
	hasBounds := width > 0 && height > 0
	for i, d := range defects {
		if d.W <= 0 || d.H <= 0 {
			return fmt.Errorf("defect %d must have a positive width and height", i+1)
		}
		if d.X < 0 || d.Y < 0 {
			return fmt.Errorf("defect %d has a negative position", i+1)
		}
		if hasBounds && (d.X+d.W > width || d.Y+d.H > height) {
			return fmt.Errorf("defect %d extends beyond the piece", i+1)
		}
	}
	return nil
}
