// Command cutoptics is the Size Module API server and, with flags, a small CLI
// for smoke-testing the optimizer without any infrastructure.
//
//	go run ./cmd/cutoptics                 # start the API on :8080
//	go run ./cmd/cutoptics -demo           # print a 2D demo plan as JSON
//	go run ./cmd/cutoptics -demo-bar       # print a 1D demo plan as JSON
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/size-module/backend/internal/modules/assemblies"
	"github.com/size-module/backend/internal/modules/campaigns"
	"github.com/size-module/backend/internal/modules/catalog"
	"github.com/size-module/backend/internal/modules/jobs"
	"github.com/size-module/backend/internal/modules/kpis"
	"github.com/size-module/backend/internal/modules/parts"
	"github.com/size-module/backend/internal/modules/plans"
	"github.com/size-module/backend/internal/modules/stock"
	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/platform/config"
	"github.com/size-module/backend/internal/platform/events"
	"github.com/size-module/backend/internal/platform/httpserver"
	"github.com/size-module/backend/internal/platform/postgres"
)

func main() {
	// Subcommand: benchmark harness (kept out of the flag set below so the
	// common path stays simple).
	if len(os.Args) > 1 && os.Args[1] == "bench" {
		os.Exit(runBench(os.Args[2:]))
	}

	var (
		demo    = flag.Bool("demo", false, "print a demo 2D optimization result as JSON and exit")
		demoBar = flag.Bool("demo-bar", false, "print a demo 1D optimization result as JSON and exit")
		addr    = flag.String("addr", "", "listen address, overrides CUTOPTICS_ADDR")
	)
	flag.Parse()

	cfg := config.Load()
	if *addr != "" {
		cfg.Addr = *addr
	}
	setupLogging(cfg.Env)

	registry := optimizer.DefaultRegistry()
	service := jobs.NewService(registry)

	if *demo || *demoBar {
		problem := jobs.DemoProblem()
		if *demoBar {
			problem = jobs.DemoBarProblem()
		}
		result, err := optimizer.Solve(context.Background(), problem, "", registry, nil)
		if err != nil {
			slog.Error("demo failed", "error", err)
			os.Exit(1)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			slog.Error("encoding demo result", "error", err)
			os.Exit(1)
		}
		return
	}

	ctx := context.Background()
	var pool *pgxpool.Pool
	if db, err := postgres.Connect(ctx, cfg.DatabaseURL); err != nil {
		slog.Warn("database unavailable, starting in optimizer-only mode", "error", err)
	} else {
		pool = db
		defer pool.Close()
		slog.Info("database connected")
	}

	hub := events.NewHub()

	var (
		jobsStore     jobs.Store
		catalogStore  catalog.Store
		partsStore    parts.Store
		assemblyStore assemblies.Store
		stockStore    stock.Store
		plansStore    plans.Store
		campaignStore campaigns.Store
		kpisStore     kpis.Store
		remnants      jobs.RemnantSource
		queueStore    jobs.QueueStore
		worker        *jobs.Worker
		dbHealth      func(context.Context) error
	)
	if pool != nil {
		store := postgres.NewStore(pool)
		jobsStore, catalogStore, partsStore = store, store, store
		assemblyStore = store
		stockStore, plansStore, remnants = store, store, store
		campaignStore = store
		kpisStore = store
		queueStore = store
		dbHealth = func(c context.Context) error { return postgres.Healthy(c, pool) }
		worker = jobs.NewWorker(store, service, hub, cfg.Workers)
		worker.Start(ctx)
		defer worker.Stop()
	}

	var canceller jobs.Canceller
	if worker != nil {
		canceller = worker
	}

	handler := httpserver.New(httpserver.Deps{
		Config:     cfg,
		Registry:   registry,
		Jobs:       jobs.NewHandler(service, jobsStore, queueStore, hub, canceller, remnants),
		Catalog:    catalog.NewHandler(catalogStore),
		Parts:      parts.NewHandler(partsStore),
		Assemblies: assemblies.NewHandler(assemblyStore),
		Stock:      stock.NewHandler(stockStore),
		Plans:      plans.NewHandler(plansStore, service),
		Campaigns:  campaigns.NewHandler(campaignStore, service, remnants),
		Kpis:       kpis.NewHandler(kpisStore),
		DBHealth:   dbHealth,
	})

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("listening", "addr", cfg.Addr, "env", cfg.Env, "version", config.Version)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
}

func setupLogging(env string) {
	var handler slog.Handler
	if env == "dev" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	slog.SetDefault(slog.New(handler))
}
