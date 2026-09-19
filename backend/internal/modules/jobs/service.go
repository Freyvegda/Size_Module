package jobs

import (
	"context"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
)

// Service runs optimization requests. It is deliberately synchronous for now:
// the async queue with progress streaming is the next milestone, and the
// ProgressFunc seam is already in place for it.
type Service struct {
	registry *core.Registry
}

func NewService(registry *core.Registry) *Service {
	return &Service{registry: registry}
}

func (s *Service) Registry() *core.Registry { return s.registry }

func (s *Service) Run(ctx context.Context, p core.Problem, solver string, progress core.ProgressFunc) (optimizer.Result, error) {
	return optimizer.Solve(ctx, p, solver, s.registry, progress)
}

// DemoProblem is the out-of-the-box sample: a mixed glass order on two sheet
// formats. The frontend uses it to render the 2D/3D viewer before any data has
// been entered.
func DemoProblem() core.Problem {
	parts := []core.Part{
		{ID: "demo-window-a", Code: "WINDOW-1200x1400", Width: core.FromMM(1200), Height: core.FromMM(1400), Quantity: 6, AllowRotate: true, Priority: 1},
		{ID: "demo-window-b", Code: "WINDOW-800x1000", Width: core.FromMM(800), Height: core.FromMM(1000), Quantity: 6, AllowRotate: true, Priority: 1},
		{ID: "demo-shelf", Code: "SHELF-500x250", Width: core.FromMM(500), Height: core.FromMM(250), Quantity: 10, AllowRotate: true, Priority: 2},
	}
	stocks := []core.StockItem{
		{ID: "demo-sheet-large", Code: "SHEET-3210x2250", Width: core.FromMM(3210), Height: core.FromMM(2250), Quantity: 4, CostPerUnit: 62.5},
		{ID: "demo-sheet-small", Code: "SHEET-2440x1220", Width: core.FromMM(2440), Height: core.FromMM(1220), Quantity: 3, CostPerUnit: 28},
	}
	return core.Normalize(core.Problem{
		Parts:  parts,
		Stocks: stocks,
		Rules:  core.DefaultRules(),
	})
}

// DemoBarProblem demonstrates the 1D solver with aluminium profiles.
func DemoBarProblem() core.Problem {
	parts := []core.Part{
		{ID: "demo-rail-long", Code: "RAIL-2400", Length: core.FromMM(2400), Quantity: 4},
		{ID: "demo-rail-mid", Code: "RAIL-1100", Length: core.FromMM(1100), Quantity: 6},
		{ID: "demo-spacer", Code: "SPACER-450", Length: core.FromMM(450), Quantity: 8},
	}
	stocks := []core.StockItem{
		{ID: "demo-bar", Code: "BAR-6000", Length: core.FromMM(6000), Width: core.FromMM(60), Quantity: 5, CostPerUnit: 14.75},
	}
	return core.Normalize(core.Problem{
		Parts:  parts,
		Stocks: stocks,
		Rules:  core.DefaultRules(),
	})
}
