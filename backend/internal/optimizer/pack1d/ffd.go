// Package pack1d contains 1D solvers for bars, profiles and tubes. FFDSolver is
// the classic First-Fit-Decreasing baseline: sort by length, then place each
// piece in the first bar with room for it.
package pack1d

import (
	"context"
	"sort"
	"time"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/cutter"
)

const solverVersion = "0.1.0"

type FFDSolver struct{}

func New() *FFDSolver { return &FFDSolver{} }

func (s *FFDSolver) Name() string    { return "ffd-1d" }
func (s *FFDSolver) Version() string { return solverVersion }

func (s *FFDSolver) Capabilities() core.Capabilities {
	return core.Capabilities{
		Dimension:   core.Profile1D,
		CutMode:     core.CutGuillotine,
		Description: "First-Fit-Decreasing packer for bars, profiles and tubes, with reusable offcuts.",
	}
}

type inst struct {
	part core.Part
}

func (s *FFDSolver) Solve(ctx context.Context, p core.Problem, progress core.ProgressFunc) (core.Solution, error) {
	start := time.Now()
	rules := p.Rules
	if rules.Kerf < 0 {
		rules.Kerf = 0
	}
	if rules.Trim < 0 {
		rules.Trim = 0
	}

	var parts []core.Part
	for _, part := range p.Parts {
		if part.Length <= 0 {
			continue
		}
		for q := 0; q < part.Quantity; q++ {
			parts = append(parts, part)
		}
	}
	sort.SliceStable(parts, func(i, j int) bool {
		a, b := parts[i], parts[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if a.Length != b.Length {
			return a.Length > b.Length
		}
		return a.Code < b.Code
	})

	taken := make([]bool, len(parts))
	var sheets []core.SheetPlan
	var notes []string
	var stockLenM, usedLenM float64

	for si := range p.Stocks {
		stock := p.Stocks[si]
		if stock.Quantity <= 0 || stock.Length <= 0 {
			continue
		}
		barH := stock.Width
		if barH <= 0 {
			barH = core.Millimeter // keeps area metrics meaningful for 1d
		}
		for copy := 0; copy < stock.Quantity; copy++ {
			if ctx.Err() != nil || allTaken(taken) {
				break
			}
			usableStart := rules.Trim
			usableEnd := stock.Length - rules.Trim
			if usableEnd-usableStart <= 0 {
				continue
			}

			cursor := usableStart
			var placements []core.Placement
			var usedLen core.Dim
			for i := range parts {
				if taken[i] {
					continue
				}
				gap := core.Dim(0)
				if len(placements) > 0 {
					gap = rules.Kerf
				}
				if cursor+gap+parts[i].Length > usableEnd {
					continue // a shorter piece later may still fit
				}
				pos := cursor + gap
				placements = append(placements, core.Placement{
					PartID:   parts[i].ID,
					PartCode: parts[i].Code,
					X:        pos,
					Y:        0,
					W:        parts[i].Length,
					H:        barH,
					Priority: parts[i].Priority,
				})
				cursor = pos + parts[i].Length
				usedLen += parts[i].Length
				taken[i] = true
			}
			if len(placements) == 0 {
				continue
			}

			plan := core.SheetPlan{
				Index:      len(sheets),
				StockID:    stock.ID,
				StockCode:  stock.Code,
				Label:      stock.Label,
				Width:      stock.Length,
				Height:     barH,
				Placements: placements,
			}
			if plan.Label == "" {
				plan.Label = stock.Code
			}
			if rem := stock.Length - rules.Trim - (cursor + rules.Kerf); rem >= rules.OffcutMinLength && rules.OffcutMinLength > 0 && rem > 0 {
				plan.Offcuts = []core.Rect{{X: cursor + rules.Kerf, Y: 0, W: rem, H: barH}}
			}
			if steps, ok := cutter.ForSheetStages(plan, rules.Kerf, rules.MaxCutStages); ok {
				plan.CutSteps = steps
			} else {
				notes = append(notes, "bar "+itoa(plan.Index+1)+": could not derive a cut sequence")
			}
			sheets = append(sheets, plan)
			stockLenM += core.LengthM(stock.Length)
			usedLenM += core.LengthM(usedLen)

			if progress != nil {
				progress(core.Solution{
					Solver:        "ffd-1d",
					SolverVersion: solverVersion,
					Seed:          p.Seed,
					Sheets:        sheets,
					Metrics:       core.Summarize(p, sheets, nil, time.Since(start).Milliseconds()),
					Notes:         notes,
				})
			}
		}
	}

	sol := core.Solution{
		Solver:        "ffd-1d",
		SolverVersion: solverVersion,
		Seed:          p.Seed,
		Sheets:        sheets,
		Metrics:       core.Summarize(p, sheets, nil, time.Since(start).Milliseconds()),
		Notes:         notes,
	}
	sol.Metrics.StockLengthM = stockLenM
	sol.Metrics.UsedLengthM = usedLenM
	sol.Unplaced = collectUnplaced(p, taken, parts)
	return sol, nil
}

func collectUnplaced(p core.Problem, taken []bool, parts []core.Part) []core.UnplacedPart {
	type agg struct {
		code  string
		count int
		big   bool
	}
	counts := map[string]*agg{}
	for i := range parts {
		if taken[i] {
			continue
		}
		part := parts[i]
		a, ok := counts[part.ID]
		if !ok {
			a = &agg{code: part.Code}
			counts[part.ID] = a
		}
		a.count++
		fits := false
		for _, st := range p.Stocks {
			if st.Length-2*p.Rules.Trim >= part.Length {
				fits = true
				break
			}
		}
		if !fits {
			a.big = true
		}
	}
	out := make([]core.UnplacedPart, 0, len(counts))
	for id, a := range counts {
		reason := "no stock or remaining capacity"
		if a.big {
			reason = "part is longer than any usable stock length"
		}
		out = append(out, core.UnplacedPart{PartID: id, PartCode: a.code, Quantity: a.count, Reason: reason})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PartCode < out[j].PartCode })
	return out
}

func allTaken(taken []bool) bool {
	for _, t := range taken {
		if !t {
			return false
		}
	}
	return true
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
