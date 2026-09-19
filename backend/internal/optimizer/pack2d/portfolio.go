package pack2d

import (
	"github.com/size-module/backend/internal/optimizer/core"
)

const portfolioVersion = "0.1.0"

// NewPortfolio returns the 2D portfolio: every 2D strategy, best score wins.
// The generic machinery (budget split, monotonic progress, strategy filtering)
// lives in core.Portfolio.
func NewPortfolio(solvers ...core.Solver) *core.Portfolio {
	return core.NewPortfolio("best-2d", portfolioVersion, core.Capabilities{
		Dimension:   core.Profile2D,
		CutMode:     core.CutGuillotine,
		Rotation:    true,
		Grain:       true,
		Remnants:    true,
		Rank:        1,
		Description: "Runs every 2D strategy and keeps the best plan by objective score.",
	}, solvers...)
}
