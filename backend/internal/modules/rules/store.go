// Package rules owns stored rules profiles: named, reusable constraint and
// objective presets.
//
// A rules profile is the only place vertical specifics live — there is no glass
// or wood code path in the engine, only different rule values. Keeping them in
// the database (instead of hardcoded defaults) lets a shop switch between a
// glass preset and a wood preset without a redeploy, and lets a job snapshot
// the exact rules it was solved with.
package rules

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/httpx"
)

// ErrNotFound is returned when a profile id does not exist.
var ErrNotFound = errors.New("rules profile not found")

// Profile is a named constraint + objective preset.
type Profile struct {
	ID        string         `json:"id"`
	Code      string         `json:"code"`
	Name      string         `json:"name"`
	Rules     core.Rules     `json:"rules"`
	Objective core.Objective `json:"objective"`
	IsDefault bool           `json:"isDefault"`
}

// SaveProfileInput is the create/update body. Code is immutable on update.
type SaveProfileInput struct {
	Code      string         `json:"code"`
	Name      string         `json:"name"`
	Rules     core.Rules     `json:"rules"`
	Objective core.Objective `json:"objective"`
	IsDefault bool           `json:"isDefault"`
}

// Store is implemented by the postgres package. Handlers stay free of SQL types.
type Store interface {
	ListProfiles(ctx context.Context) ([]Profile, error)
	GetProfile(ctx context.Context, id string) (Profile, error)
	CreateProfile(ctx context.Context, in SaveProfileInput) (Profile, error)
	UpdateProfile(ctx context.Context, id string, in SaveProfileInput) (Profile, error)
}

type Handler struct {
	store Store
}

func NewHandler(store Store) *Handler { return &Handler{store: store} }

func (h *Handler) Routes(r chi.Router) {
	r.Get("/rules-profiles", h.list)
	r.Post("/rules-profiles", h.create)
	r.Get("/rules-profiles/{id}", h.get)
	r.Put("/rules-profiles/{id}", h.update)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	profiles, err := h.store.ListProfiles(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"rulesProfiles": profiles})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	profile, err := h.store.GetProfile(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not_found", "unknown rules profile id")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "lookup_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, profile)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	var in SaveProfileInput
	if err := httpx.DecodeJSON(w, r, &in, 1<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := normalizeAndValidate(&in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", err.Error())
		return
	}
	profile, err := h.store.CreateProfile(r.Context(), in)
	if err != nil {
		httpx.Error(w, http.StatusConflict, "create_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, profile)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database_unavailable", "start PostgreSQL (db/scripts/up.ps1) and run the migrations")
		return
	}
	var in SaveProfileInput
	if err := httpx.DecodeJSON(w, r, &in, 1<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := normalizeAndValidate(&in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", err.Error())
		return
	}
	profile, err := h.store.UpdateProfile(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not_found", "unknown rules profile id")
			return
		}
		httpx.Error(w, http.StatusConflict, "update_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, profile)
}

// normalizeAndValidate fills engine defaults for an empty rules object and
// rejects values the engine cannot use, so a bad profile fails at the edge
// instead of producing invalid plans later.
func normalizeAndValidate(in *SaveProfileInput) error {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	if in.Code == "" {
		return errors.New("code is required")
	}
	if in.Rules == (core.Rules{}) {
		in.Rules = core.DefaultRules()
	}
	if in.Objective.Weights == (core.Weights{}) {
		in.Objective.Weights = core.DefaultWeights()
	}
	if in.Rules.Kerf < 0 || in.Rules.Trim < 0 {
		return errors.New("kerf and trim cannot be negative")
	}
	if in.Rules.OffcutMinW < 0 || in.Rules.OffcutMinH < 0 || in.Rules.OffcutMinLength < 0 {
		return errors.New("offcut minimum sizes cannot be negative")
	}
	if in.Rules.MinPartDim < 0 {
		return errors.New("minPartDim cannot be negative")
	}
	if in.Rules.MaxCutStages < 0 || in.Rules.MaxPartsPerSheet < 0 {
		return errors.New("maxCutStages and maxPartsPerSheet cannot be negative")
	}
	if in.Rules.OversAllowedPct < 0 {
		return errors.New("oversAllowedPct cannot be negative")
	}
	switch in.Rules.CutMode {
	case "", core.CutGuillotine, core.CutFree:
	default:
		return errors.New("cutMode must be guillotine or free")
	}
	switch in.Rules.GrainMode {
	case "", core.GrainNone, core.GrainAlongX, core.GrainAlongY:
	default:
		return errors.New("grainMode must be none, along_x or along_y")
	}
	return nil
}
