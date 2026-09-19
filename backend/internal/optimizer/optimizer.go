// Package optimizer wires the pure algorithm packages together and is the only
// place the rest of the backend talks to. Swapping in a new solver (beam
// search, column generation, CP-SAT sidecar) means registering it here.
package optimizer

import (
	"context"
	"fmt"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/costing"
	"github.com/size-module/backend/internal/optimizer/explain"
	"github.com/size-module/backend/internal/optimizer/pack1d"
	"github.com/size-module/backend/internal/optimizer/pack2d"
	"github.com/size-module/backend/internal/optimizer/validator"
)

// Result is a complete answer: the layout, its scorecard, its cost breakdown
// and any violations.
type Result struct {
	Solution   core.Solution         `json:"solution"`
	Violations []validator.Violation `json:"violations,omitempty"`
	Score      float64               `json:"score"`
	Cost       costing.Report        `json:"cost"`
}

// DefaultRegistry registers the solvers shipped with the baseline. Rank order
// decides which one the job layer picks when the caller does not name a solver.
func DefaultRegistry() *core.Registry {
	r := core.NewRegistry()
	r.Register(pack1d.New())
	r.Register(pack1d.NewColumn())
	r.Register(pack1d.NewPinned())
	r.Register(pack1d.NewPortfolio(pack1d.New(), pack1d.NewColumn()))
	r.Register(pack2d.New())
	r.Register(pack2d.NewBeam())
	r.Register(pack2d.NewColumn())
	r.Register(pack2d.NewPolish())
	r.Register(pack2d.NewMaxRects())
	r.Register(pack2d.NewPinned())
	r.Register(pack2d.NewPortfolio(pack2d.New(), pack2d.NewBeam(), pack2d.NewColumn(), pack2d.NewPolish(), pack2d.NewMaxRects()))
	return r
}

// Solve normalizes the problem, picks a solver, runs it, validates the result
// and attaches explanatory notes. progress may be nil.
func Solve(ctx context.Context, p core.Problem, solverName string, reg *core.Registry, progress core.ProgressFunc) (Result, error) {
	p = core.Normalize(p)

	var solver core.Solver
	var err error
	if solverName != "" {
		var ok bool
		solver, ok = reg.Get(solverName)
		if !ok {
			return Result{}, fmt.Errorf("unknown solver %q (available: %v)", solverName, reg.Names())
		}
		// A named solver must actually be able to serve this problem: a 1D
		// solver handed a 2D problem (or a free-cutting solver handed a
		// guillotine order) would silently return an empty plan.
		caps := solver.Capabilities()
		if profile := core.DetectProfile(p); caps.Dimension != profile {
			return Result{}, fmt.Errorf("solver %q handles %s problems, but this problem is %s",
				solverName, caps.Dimension, profile)
		}
		if !core.CutModeCompatible(caps.CutMode, p.Rules.CutMode) {
			return Result{}, fmt.Errorf("solver %q produces %s layouts, but the rules require %s",
				solverName, caps.CutMode, p.Rules.CutMode)
		}
		if len(p.Pinned) > 0 && !caps.Pinned {
			return Result{}, fmt.Errorf("solver %q does not support pinned (locked) placements", solverName)
		}
	} else {
		solver, err = reg.ForProblem(p)
		if err != nil {
			return Result{}, err
		}
	}

	sol, err := solver.Solve(ctx, p, progress)
	if err != nil {
		return Result{}, fmt.Errorf("solver %s failed: %w", solver.Name(), err)
	}
	violations := validator.Validate(p, sol)
	sol.Notes = append(sol.Notes, explain.Notes(p, sol)...)

	return Result{
		Solution:   sol,
		Violations: violations,
		Score:      core.Score(p, sol.Metrics),
		Cost:       costing.Breakdown(p, sol),
	}, nil
}

// DetectProfile is kept for callers outside the engine (bench, tests); the
// implementation lives in core so the registry can use it too.
func DetectProfile(p core.Problem) core.DimensionProfile {
	return core.DetectProfile(p)
}
