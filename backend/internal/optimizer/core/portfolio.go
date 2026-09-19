package core

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// Portfolio runs several strategies on the same problem and returns the plan
// with the highest objective score. It is how "best of both worlds" becomes the
// default without hardcoding a winner: the benchmark decides.
//
// Strategies that cannot serve the problem (wrong dimension profile or a cut
// mode stricter than the rules allow) are skipped, so one portfolio can be
// registered per dimension and stay honest for every rules profile.
type Portfolio struct {
	SolverName string
	SolverVer  string
	Cap        Capabilities
	Inner      []Solver
}

func NewPortfolio(name, version string, capabilities Capabilities, solvers ...Solver) *Portfolio {
	inner := make([]Solver, 0, len(solvers))
	for _, s := range solvers {
		if s != nil {
			inner = append(inner, s)
		}
	}
	return &Portfolio{SolverName: name, SolverVer: version, Cap: capabilities, Inner: inner}
}

func (p *Portfolio) Name() string               { return p.SolverName }
func (p *Portfolio) Version() string            { return p.SolverVer }
func (p *Portfolio) Capabilities() Capabilities { return p.Cap }

// Strategies lists the wrapped solver names, in execution order.
func (p *Portfolio) Strategies() []string {
	out := make([]string, 0, len(p.Inner))
	for _, sub := range p.Inner {
		out = append(out, sub.Name())
	}
	return out
}

func (p *Portfolio) suitable(s Solver, problem Problem) bool {
	caps := s.Capabilities()
	if caps.Dimension != DetectProfile(problem) {
		return false
	}
	return CutModeCompatible(caps.CutMode, problem.Rules.CutMode)
}

func (p *Portfolio) Solve(ctx context.Context, problem Problem, progress ProgressFunc) (Solution, error) {
	if len(p.Inner) == 0 {
		return Solution{}, errors.New("portfolio has no strategies")
	}

	applicable := make([]Solver, 0, len(p.Inner))
	for _, s := range p.Inner {
		if p.suitable(s, problem) {
			applicable = append(applicable, s)
		}
	}
	if len(applicable) == 0 {
		return Solution{}, fmt.Errorf("no strategy in %s can serve this problem", p.SolverName)
	}

	// Split the budget so the total wall clock stays inside the request budget.
	budget := problem.BudgetMS / len(applicable)
	if budget < 500 {
		budget = 500
	}

	var best Solution
	bestScore := math.Inf(-1)
	bestSeen := math.Inf(-1)
	tried := make([]string, 0, len(applicable))

	for _, sub := range applicable {
		if ctx.Err() != nil {
			break
		}
		subCtx, cancel := context.WithTimeout(ctx, time.Duration(budget)*time.Millisecond)

		var forward ProgressFunc
		if progress != nil {
			// Only forward improving partial plans, so the UI sees a monotonic
			// best-so-far across strategies.
			forward = func(sol Solution) {
				if score := Score(problem, sol.Metrics); score > bestSeen {
					bestSeen = score
					progress(sol)
				}
			}
		}

		sol, err := sub.Solve(subCtx, problem, forward)
		cancel()
		if err != nil {
			continue
		}
		tried = append(tried, sub.Name())
		if score := Score(problem, sol.Metrics); score > bestScore {
			best, bestScore = sol, score
		}
	}

	if best.Solver == "" {
		return Solution{}, fmt.Errorf("no strategy in %s produced a plan", p.SolverName)
	}
	winner := best.Solver
	// Report the portfolio as the solver (that is what the caller asked for and
	// what gets archived), and name the winning strategy in the notes.
	best.Solver = p.SolverName
	best.SolverVersion = p.SolverVer
	best.Notes = append(best.Notes, fmt.Sprintf(
		"Portfolio compared %s and kept %s (score %.2f).",
		strings.Join(tried, ", "), winner, bestScore))
	return best, nil
}
