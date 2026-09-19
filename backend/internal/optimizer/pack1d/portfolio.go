package pack1d

import (
	"github.com/size-module/backend/internal/optimizer/core"
)

const portfolioVersion = "0.1.0"

// NewPortfolio returns the 1D portfolio: the FFD baseline and column
// generation, best objective score wins. The generic machinery lives in
// core.Portfolio.
func NewPortfolio(solvers ...core.Solver) *core.Portfolio {
	return core.NewPortfolio("best-1d", portfolioVersion, core.Capabilities{
		Dimension:   core.Profile1D,
		CutMode:     core.CutGuillotine,
		Rank:        1,
		Description: "Runs the 1D strategies (FFD baseline and column generation) and keeps the best plan.",
	}, solvers...)
}
