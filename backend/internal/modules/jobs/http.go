package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/events"
	"github.com/size-module/backend/internal/platform/httpx"
)

// Store archives completed runs. It is optional: the optimizer works without a
// database, results are simply not persisted.
type Store interface {
	SaveRun(ctx context.Context, req SaveRunRequest) (SavedRun, error)
}

// SavedRun identifies the archived job and the plan written with it.
type SavedRun struct {
	JobID  string
	PlanID string
}

type SaveRunRequest struct {
	Problem core.Problem
	Solver  string
	Result  optimizer.Result
}

// RemnantSource supplies the available labelled remnants a job should consider
// before fresh catalog stock. It is optional: without a database there is
// nothing to include.
type RemnantSource interface {
	ListRemnants(ctx context.Context, materialSpecID string) ([]core.StockItem, error)
}

// Canceller stops a job running in this process; the worker implements it.
type Canceller interface {
	Cancel(jobID string) bool
}

type Handler struct {
	svc       *Service
	store     Store
	queue     QueueStore
	hub       *events.Hub
	canceller Canceller
	remnants  RemnantSource
}

func NewHandler(svc *Service, store Store, queue QueueStore, hub *events.Hub, canceller Canceller, remnants RemnantSource) *Handler {
	return &Handler{svc: svc, store: store, queue: queue, hub: hub, canceller: canceller, remnants: remnants}
}

func (h *Handler) Routes(r chi.Router) {
	r.Post("/optimize", h.optimize)
	r.Get("/solvers", h.solvers)
	r.Get("/demo/plan", h.demoPlan)
	r.Get("/demo/bar-plan", h.demoBarPlan)

	// Asynchronous pipeline: submit, inspect, cancel, stream.
	r.Post("/jobs", h.submitJob)
	r.Get("/jobs/{id}", h.getJob)
	r.Post("/jobs/{id}/cancel", h.cancelJob)
	r.Get("/jobs/{id}/events", h.jobEvents)
}

type optimizeResponse struct {
	// ID is the archived job id, empty when no database is configured.
	ID string `json:"id,omitempty"`
	// PlanID is the archived plan id, empty when the run was not archived.
	PlanID string           `json:"planId,omitempty"`
	Result optimizer.Result `json:"result"`
}

// optimize runs a job synchronously. Kept for interactive use (the plan viewer
// and the solver comparison); long jobs belong on the queue.
func (h *Handler) optimize(w http.ResponseWriter, r *http.Request) {
	var problem core.Problem
	if err := httpx.DecodeJSON(w, r, &problem, 8<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	problem, err := h.withRemnants(r.Context(), r, problem)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "remnant_lookup_failed", err.Error())
		return
	}

	solverName := r.URL.Query().Get("solver")
	dryRun := r.URL.Query().Get("dryRun") == "true" || r.URL.Query().Get("dryRun") == "1"
	normalized := core.Normalize(problem)
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(normalized.BudgetMS)*time.Millisecond+5*time.Second)
	defer cancel()

	result, err := h.svc.Run(ctx, problem, solverName, nil)
	if err != nil {
		httpx.Error(w, http.StatusUnprocessableEntity, "optimization_failed", err.Error())
		return
	}

	response := optimizeResponse{Result: result}
	if h.store != nil && !dryRun {
		saved, err := h.store.SaveRun(ctx, SaveRunRequest{
			Problem: normalized,
			Solver:  result.Solution.Solver,
			Result:  result,
		})
		if err != nil {
			slog.Warn("could not archive optimization job", "error", err)
		} else {
			response.ID = saved.JobID
			response.PlanID = saved.PlanID
		}
	}
	httpx.JSON(w, http.StatusOK, response)
}

// withRemnants appends the available labelled remnants to a submitted problem
// when the caller asked for them (?includeRemnants=1). Remnants that do not
// match the problem's dimension profile are skipped, and a nil source (no
// database) leaves the problem untouched.
func (h *Handler) withRemnants(ctx context.Context, r *http.Request, problem core.Problem) (core.Problem, error) {
	raw := r.URL.Query().Get("includeRemnants")
	if raw != "true" && raw != "1" {
		return problem, nil
	}
	if h.remnants == nil {
		return problem, nil
	}
	items, err := h.remnants.ListRemnants(ctx, r.URL.Query().Get("materialSpecId"))
	if err != nil {
		return problem, err
	}
	profile := core.DetectProfile(problem)
	for _, item := range items {
		if profile == core.Profile1D && item.Length <= 0 {
			continue
		}
		if profile == core.Profile2D && (item.Width <= 0 || item.Height <= 0) {
			continue
		}
		problem.Stocks = append(problem.Stocks, item)
	}
	return problem, nil
}

type solverInfo struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Capabilities core.Capabilities `json:"capabilities"`
}

func (h *Handler) solvers(w http.ResponseWriter, r *http.Request) {
	solvers := h.svc.Registry().List()
	out := make([]solverInfo, 0, len(solvers))
	for _, s := range solvers {
		out = append(out, solverInfo{Name: s.Name(), Version: s.Version(), Capabilities: s.Capabilities()})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"solvers": out})
}

func (h *Handler) demoPlan(w http.ResponseWriter, r *http.Request) {
	h.runDemo(w, r, DemoProblem())
}

func (h *Handler) demoBarPlan(w http.ResponseWriter, r *http.Request) {
	h.runDemo(w, r, DemoBarProblem())
}

func (h *Handler) runDemo(w http.ResponseWriter, r *http.Request, problem core.Problem) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result, err := h.svc.Run(ctx, problem, "", nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "demo_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, optimizeResponse{Result: result})
}

// ------------------------------------------------------------- async jobs ---

// submitJob queues a job and returns immediately with its id.
func (h *Handler) submitJob(w http.ResponseWriter, r *http.Request) {
	if h.queue == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "queue_unavailable",
			"the async job queue needs a database; use POST /api/v1/optimize instead")
		return
	}
	var problem core.Problem
	if err := httpx.DecodeJSON(w, r, &problem, 8<<20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	problem, err := h.withRemnants(r.Context(), r, problem)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "remnant_lookup_failed", err.Error())
		return
	}
	normalized := core.Normalize(problem)
	id, err := h.queue.Enqueue(r.Context(), EnqueueRequest{
		Problem:  normalized,
		Solver:   r.URL.Query().Get("solver"),
		Seed:     normalized.Seed,
		BudgetMS: normalized.BudgetMS,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "enqueue_failed", err.Error())
		return
	}
	slog.Info("job queued", "job", id)
	httpx.JSON(w, http.StatusAccepted, map[string]any{"id": id, "status": "queued"})
}

func (h *Handler) getJob(w http.ResponseWriter, r *http.Request) {
	if h.queue == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "queue_unavailable", "the async job queue needs a database")
		return
	}
	view, err := h.queue.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, ErrJobNotFound) {
			httpx.Error(w, http.StatusNotFound, "not_found", "unknown job id")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "job_lookup_failed", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, view)
}

// cancelJob stops a running job when it runs in this process and always marks
// it cancelled in the database.
func (h *Handler) cancelJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	cancelled := false
	if h.canceller != nil {
		cancelled = h.canceller.Cancel(jobID)
	}
	if h.queue != nil {
		if ok, err := h.queue.Cancel(r.Context(), jobID); err == nil && ok {
			cancelled = true
		}
	}
	if h.hub != nil && cancelled {
		h.hub.Publish(jobID, events.Event{Type: "cancelled", JobID: jobID})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"cancelled": cancelled})
}

// jobEvents streams a job's progress as Server-Sent Events: an immediate
// snapshot, then live progress from the worker, with a two second database poll
// as a fallback (the worker may run in another process) and a heartbeat so
// proxies keep the connection open.
func (h *Handler) jobEvents(w http.ResponseWriter, r *http.Request) {
	if h.queue == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "queue_unavailable", "job events need a database")
		return
	}
	jobID := chi.URLParam(r, "id")
	view, err := h.queue.Get(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, ErrJobNotFound) {
			httpx.Error(w, http.StatusNotFound, "not_found", "unknown job id")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "job_lookup_failed", err.Error())
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Error(w, http.StatusInternalServerError, "streaming_unsupported", "the server cannot stream events")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	writeEvent := func(event events.Event) {
		payload, err := json.Marshal(event)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
			return
		}
		flusher.Flush()
	}

	writeEvent(events.Event{Type: "snapshot", JobID: jobID, Data: view})
	if IsTerminal(view.Status) {
		return
	}

	var live <-chan events.Event
	if h.hub != nil {
		channel, unsubscribe := h.hub.Subscribe(jobID)
		defer unsubscribe()
		live = channel
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	poll := time.NewTicker(2 * time.Second)
	defer poll.Stop()
	lastStatus := view.Status

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case event := <-live:
			writeEvent(event)
			if IsTerminal(event.Type) {
				return
			}
		case <-poll.C:
			current, err := h.queue.Get(r.Context(), jobID)
			if err != nil {
				continue
			}
			if current.Status != lastStatus || IsTerminal(current.Status) {
				lastStatus = current.Status
				writeEvent(events.Event{Type: "snapshot", JobID: jobID, Data: current})
				if IsTerminal(current.Status) {
					return
				}
			}
		}
	}
}
