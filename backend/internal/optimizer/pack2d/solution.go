package pack2d

import (
	"time"

	"github.com/size-module/backend/internal/optimizer/core"
)

// assembleSolution builds the common part of a solution: name, seed, sheets,
// derived metrics and timing. Every 2D solver uses it so results are directly
// comparable.
func assembleSolution(name, version string, p core.Problem, sheets []core.SheetPlan, start time.Time, notes []string) core.Solution {
	return core.Solution{
		Solver:        name,
		SolverVersion: version,
		Seed:          p.Seed,
		Sheets:        sheets,
		Metrics:       core.Summarize(p, sheets, nil, time.Since(start).Milliseconds()),
		Notes:         notes,
	}
}
