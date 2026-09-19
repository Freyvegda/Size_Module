package pack2d

import (
	"context"
	"fmt"
	"time"

	"github.com/size-module/backend/internal/optimizer/core"
)

const polishVersion = "0.1.0"

// PolishSolver is beam search followed by local search. The beam builds a good
// plan in a few milliseconds; the local search then spends the rest of the time
// budget trying to dissolve and merge sheets. It is the first solver here that
// actually uses a multi-second budget, which matters for big orders.
type PolishSolver struct {
	Base *BeamSolver
}

func NewPolish() *PolishSolver {
	return &PolishSolver{Base: NewBeam()}
}

func (s *PolishSolver) Name() string    { return "polish-2d" }
func (s *PolishSolver) Version() string { return polishVersion }

func (s *PolishSolver) Capabilities() core.Capabilities {
	return core.Capabilities{
		Dimension:   core.Profile2D,
		CutMode:     core.CutGuillotine,
		Rotation:    true,
		Grain:       true,
		Remnants:    true,
		Rank:        3,
		Description: "Beam search plus local search: dissolves and merges sheets until the time budget runs out.",
	}
}

func (s *PolishSolver) Solve(ctx context.Context, p core.Problem, progress core.ProgressFunc) (core.Solution, error) {
	start := time.Now()
	beam := s.Base
	if beam == nil {
		beam = NewBeam()
	}

	base, err := beam.Solve(ctx, p, progress)
	if err != nil {
		return base, err
	}

	improved := Improve(ctx, p, base, p.Seed, progress)
	improved.Solver = s.Name()
	improved.SolverVersion = polishVersion
	improved.Metrics = core.Summarize(p, improved.Sheets, improved.Unplaced, time.Since(start).Milliseconds())

	if len(improved.Sheets) < len(base.Sheets) {
		improved.Notes = append(improved.Notes, fmt.Sprintf(
			"Local search removed %d sheet(s) from the beam plan (budget %d ms).",
			len(base.Sheets)-len(improved.Sheets), p.BudgetMS))
	} else {
		improved.Notes = append(improved.Notes, fmt.Sprintf(
			"Local search ran for %d ms without finding a better sheet arrangement; the beam plan stands.",
			improved.Metrics.ElapsedMS))
	}
	return improved, nil
}
