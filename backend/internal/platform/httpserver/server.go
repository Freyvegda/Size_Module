package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/size-module/backend/internal/modules/assemblies"
	"github.com/size-module/backend/internal/modules/campaigns"
	"github.com/size-module/backend/internal/modules/catalog"
	"github.com/size-module/backend/internal/modules/jobs"
	"github.com/size-module/backend/internal/modules/kpis"
	"github.com/size-module/backend/internal/modules/parts"
	"github.com/size-module/backend/internal/modules/plans"
	"github.com/size-module/backend/internal/modules/rules"
	"github.com/size-module/backend/internal/modules/stock"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/config"
	"github.com/size-module/backend/internal/platform/httpx"
)

// Deps is everything the router needs. Store backed handlers are optional so
// the API can start without a database.
type Deps struct {
	Config     config.Config
	Registry   *core.Registry
	Jobs       *jobs.Handler
	Catalog    *catalog.Handler
	Parts      *parts.Handler
	Assemblies *assemblies.Handler
	Stock      *stock.Handler
	Plans      *plans.Handler
	Campaigns  *campaigns.Handler
	Kpis       *kpis.Handler
	Rules      *rules.Handler
	DBHealth   func(ctx context.Context) error
}

// New builds the HTTP handler.
func New(deps Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// Solver runs may take minutes; individual handlers apply tighter timeouts.
	r.Use(middleware.Timeout(10 * time.Minute))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   deps.Config.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-Id"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/healthz", health(deps))
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/healthz", health(deps))
		r.Get("/meta", meta(deps))
		if deps.Jobs != nil {
			deps.Jobs.Routes(r)
		}
		if deps.Catalog != nil {
			deps.Catalog.Routes(r)
		}
		if deps.Parts != nil {
			deps.Parts.Routes(r)
		}
		if deps.Assemblies != nil {
			deps.Assemblies.Routes(r)
		}
		if deps.Stock != nil {
			deps.Stock.Routes(r)
		}
		if deps.Plans != nil {
			deps.Plans.Routes(r)
		}
		if deps.Campaigns != nil {
			deps.Campaigns.Routes(r)
		}
		if deps.Kpis != nil {
			deps.Kpis.Routes(r)
		}
		if deps.Rules != nil {
			deps.Rules.Routes(r)
		}
	})
	return r
}

func health(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbStatus := "not_configured"
		status := http.StatusOK
		if deps.DBHealth != nil {
			if err := deps.DBHealth(r.Context()); err != nil {
				dbStatus = "down"
				status = http.StatusServiceUnavailable
			} else {
				dbStatus = "up"
			}
		}
		body := map[string]any{
			"status":  map[bool]string{true: "ok", false: "degraded"}[status == http.StatusOK],
			"db":      dbStatus,
			"version": config.Version,
			"env":     deps.Config.Env,
			"time":    time.Now().UTC().Format(time.RFC3339),
		}
		httpx.JSON(w, status, body)
	}
}

func meta(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var solvers []map[string]any
		if deps.Registry != nil {
			for _, s := range deps.Registry.List() {
				solvers = append(solvers, map[string]any{
					"name":         s.Name(),
					"version":      s.Version(),
					"capabilities": s.Capabilities(),
				})
			}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{
			"name":    "Size Module",
			"version": config.Version,
			"env":     deps.Config.Env,
			"solvers": solvers,
		})
	}
}
