package core

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Capabilities describes what a solver can handle, so the job layer can pick
// the right one (and the UI can explain why a job was rejected).
type Capabilities struct {
	Dimension DimensionProfile
	CutMode   CutMode
	Rotation  bool
	Grain     bool
	Remnants  bool
	// Pinned marks solvers that can honour Problem.Pinned (planner-locked
	// placements). A re-solve only considers solvers with this capability.
	Pinned   bool
	MaxParts int
	// Rank orders solvers that serve the same dimension profile: lower is
	// preferred when the caller does not name a solver. 0 means unranked and
	// sorts last.
	Rank        int
	Description string
}

// ProgressFunc receives the best solution found so far. Solvers implementing
// any-time behaviour call it while they still have time budget left; the job
// layer forwards these to the UI over SSE.
type ProgressFunc func(Solution)

// Solver is the single seam every algorithm plugs into. Adding a beam search,
// a column generation engine or a CP-SAT sidecar later means implementing this
// interface, nothing else.
type Solver interface {
	Name() string
	Version() string
	Capabilities() Capabilities
	// Solve must return the best solution it found when ctx is done or the
	// problem budget expires, rather than an error.
	Solve(ctx context.Context, p Problem, progress ProgressFunc) (Solution, error)
}

// Registry holds the available solvers.
type Registry struct {
	mu      sync.RWMutex
	solvers map[string]Solver
	order   []string
}

func NewRegistry() *Registry {
	return &Registry{solvers: map[string]Solver{}}
}

func (r *Registry) Register(s Solver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.solvers[s.Name()]; !exists {
		r.order = append(r.order, s.Name())
	}
	r.solvers[s.Name()] = s
}

func (r *Registry) Get(name string) (Solver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.solvers[name]
	return s, ok
}

// Names returns registered solver names in registration order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// List returns the registered solvers in registration order.
func (r *Registry) List() []Solver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Solver, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.solvers[name])
	}
	return out
}

// ForProfile returns the preferred solver for a dimension profile. Solvers are
// ordered by Capabilities.Rank, then by name, so a portfolio can be promoted
// over the individual strategies it runs without renaming anything.
func (r *Registry) ForProfile(profile DimensionProfile) (Solver, error) {
	candidates := r.candidates(profile, "")
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no solver registered for dimension profile %q", profile)
	}
	return candidates[0], nil
}

// ForProblem returns the preferred solver for a problem: same ranking as
// ForProfile, but it also honours the cut mode the rules ask for. A problem
// that allows free cutting may still be served by a guillotine solver (that
// layout is always valid); a guillotine problem must not be served by a
// free-cutting solver, whose layouts it cannot produce.
func (r *Registry) ForProblem(p Problem) (Solver, error) {
	profile := DetectProfile(p)
	candidates := r.candidates(profile, p.Rules.CutMode)
	if len(p.Pinned) > 0 {
		candidates = pinnedCandidates(candidates)
	}
	if len(candidates) == 0 {
		// Nothing matches the cut mode exactly; fall back to the best solver
		// for the profile rather than refusing to solve.
		candidates = r.candidates(profile, "")
		if len(p.Pinned) > 0 {
			candidates = pinnedCandidates(candidates)
		}
	}
	if len(candidates) == 0 {
		if len(p.Pinned) > 0 {
			return nil, fmt.Errorf("no solver registered for %q problems with pinned placements", profile)
		}
		return nil, fmt.Errorf("no solver registered for dimension profile %q", profile)
	}
	return candidates[0], nil
}

// pinnedCandidates keeps only solvers that can honour locked placements.
func pinnedCandidates(candidates []Solver) []Solver {
	out := candidates[:0:0]
	for _, s := range candidates {
		if s.Capabilities().Pinned {
			out = append(out, s)
		}
	}
	return out
}

// candidates returns the solvers serving a profile, optionally restricted to a
// cut mode, ordered by rank then name.
func (r *Registry) candidates(profile DimensionProfile, mode CutMode) []Solver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Solver, 0, len(r.order))
	for _, name := range r.order {
		s := r.solvers[name]
		caps := s.Capabilities()
		if caps.Dimension != profile {
			continue
		}
		if !CutModeCompatible(caps.CutMode, mode) {
			continue
		}
		out = append(out, s)
	}
	rank := func(s Solver) int {
		if r := s.Capabilities().Rank; r > 0 {
			return r
		}
		return 1000
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank(out[i]), rank(out[j])
		if ri != rj {
			return ri < rj
		}
		return out[i].Name() < out[j].Name()
	})
	return out
}
