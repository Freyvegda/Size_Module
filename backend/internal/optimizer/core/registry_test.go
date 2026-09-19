package core

import (
	"context"
	"testing"
)

// fakeSolver exists only to exercise registry selection.
type fakeSolver struct {
	name string
	caps Capabilities
}

func (f fakeSolver) Name() string               { return f.name }
func (f fakeSolver) Version() string            { return "test" }
func (f fakeSolver) Capabilities() Capabilities { return f.caps }
func (f fakeSolver) Solve(context.Context, Problem, ProgressFunc) (Solution, error) {
	return Solution{Solver: f.name}, nil
}

func registryWithFakes() *Registry {
	r := NewRegistry()
	r.Register(fakeSolver{name: "gui-2d", caps: Capabilities{Dimension: Profile2D, CutMode: CutGuillotine, Rank: 10}})
	r.Register(fakeSolver{name: "free-2d", caps: Capabilities{Dimension: Profile2D, CutMode: CutFree, Rank: 1}})
	r.Register(fakeSolver{name: "gui-1d", caps: Capabilities{Dimension: Profile1D, CutMode: CutGuillotine, Rank: 10}})
	return r
}

func sheetProblem(cutMode CutMode) Problem {
	return Normalize(Problem{
		Parts:  []Part{{ID: "p", Code: "P", Width: FromMM(100), Height: FromMM(100), Quantity: 1}},
		Stocks: []StockItem{{ID: "s", Code: "S", Width: FromMM(1000), Height: FromMM(1000), Quantity: 1}},
		Rules:  Rules{CutMode: cutMode},
	})
}

func TestForProblemHonoursCutMode(t *testing.T) {
	reg := registryWithFakes()

	guillotine, err := reg.ForProblem(sheetProblem(CutGuillotine))
	if err != nil {
		t.Fatalf("guillotine problem: %v", err)
	}
	if guillotine.Name() != "gui-2d" {
		t.Fatalf("a guillotine problem must not be handed to %s", guillotine.Name())
	}

	// Free cutting may use the guillotine solver (that layout is valid), and the
	// free solver outranks it.
	free, err := reg.ForProblem(sheetProblem(CutFree))
	if err != nil {
		t.Fatalf("free problem: %v", err)
	}
	if free.Name() != "free-2d" {
		t.Fatalf("expected the free-cutting solver to win on rank, got %s", free.Name())
	}
}

func TestForProblemFallsBackWhenCutModeHasNoSolver(t *testing.T) {
	reg := NewRegistry()
	reg.Register(fakeSolver{name: "gui-2d", caps: Capabilities{Dimension: Profile2D, CutMode: CutGuillotine}})

	// No free-cutting solver is registered; the guillotine one is still valid
	// for a free-cutting problem, so it must be returned rather than an error.
	solver, err := reg.ForProblem(sheetProblem(CutFree))
	if err != nil {
		t.Fatalf("expected a fallback solver: %v", err)
	}
	if solver.Name() != "gui-2d" {
		t.Fatalf("unexpected solver %s", solver.Name())
	}
}

func TestForProblemPicksByProfile(t *testing.T) {
	reg := registryWithFakes()

	problem := Normalize(Problem{
		Parts:  []Part{{ID: "bar", Code: "BAR", Length: FromMM(1000), Quantity: 2}},
		Stocks: []StockItem{{ID: "s", Code: "S", Length: FromMM(6000), Width: FromMM(50), Quantity: 1}},
	})
	solver, err := reg.ForProblem(problem)
	if err != nil {
		t.Fatalf("1d problem: %v", err)
	}
	if solver.Name() != "gui-1d" {
		t.Fatalf("expected the 1d solver, got %s", solver.Name())
	}
}

func TestCutModeCompatible(t *testing.T) {
	if !CutModeCompatible(CutGuillotine, CutFree) {
		t.Fatal("a guillotine layout is valid on a free-cutting machine")
	}
	if CutModeCompatible(CutFree, CutGuillotine) {
		t.Fatal("a free layout is not guillotine-cuttable")
	}
	if !CutModeCompatible(CutGuillotine, "") {
		t.Fatal("an empty requirement accepts any solver")
	}
}
