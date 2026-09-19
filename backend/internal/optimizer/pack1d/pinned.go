package pack1d

import (
	"context"
	"sort"
	"time"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/cutter"
)

const pinnedVersion = "0.1.0"

// PinnedSolver packs residual demand around planner-locked placements on bars.
//
// Each core.PinnedSheet is a locked bar layout: the pinned pieces keep their
// exact position along the bar, the free intervals between them are filled
// first-fit, and the rest of the stock (one bar per pinned sheet is consumed)
// is packed like the FFD baseline. Remaining intervals long enough to reuse
// become offcuts. Without pins it behaves like ffd-1d.
type PinnedSolver struct{}

func NewPinned() *PinnedSolver { return &PinnedSolver{} }

func (s *PinnedSolver) Name() string    { return "pinned-1d" }
func (s *PinnedSolver) Version() string { return pinnedVersion }

func (s *PinnedSolver) Capabilities() core.Capabilities {
	return core.Capabilities{
		Dimension: core.Profile1D,
		CutMode:   core.CutGuillotine,
		Rotation:  true,
		Grain:     true,
		Remnants:  true,
		Pinned:    true,
		Rank:      40,
		Description: "First-Fit-Decreasing packer that honours locked placements: keeps them " +
			"exactly in place and fills the free bar intervals around them.",
	}
}

func (s *PinnedSolver) Solve(ctx context.Context, p core.Problem, progress core.ProgressFunc) (core.Solution, error) {
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
	for _, pin := range p.Pinned {
		for _, pl := range pin.Placements {
			if i := matchPart(parts, taken, pl); i >= 0 {
				taken[i] = true
			}
		}
	}

	var sheets []core.SheetPlan
	var stockLenM, usedLenM float64
	lockedCount := 0

	for _, pin := range p.Pinned {
		plan := buildPinnedBar(pin, parts, taken, rules)
		if plan.Width <= 0 || plan.Height <= 0 {
			continue
		}
		plan.Index = len(sheets)
		lockedCount += len(pin.Placements)
		if len(plan.Placements) > 0 {
			if steps, ok := cutter.ForSheet(plan, rules.Kerf); ok {
				plan.CutSteps = steps
			}
		}
		sheets = append(sheets, plan)
		stockLenM += core.LengthM(plan.Width)
		usedLenM += core.LengthM(usedLength(plan.Placements))

		if progress != nil {
			progress(assembleBars("pinned-1d", p, sheets, stockLenM, usedLenM, start, nil))
		}
	}

	// Remaining bar copies: a pinned sheet occupies one copy of its stock.
	usedCopies := map[string]int{}
	for _, pin := range p.Pinned {
		if pin.StockID != "" {
			usedCopies[pin.StockID]++
		}
	}
	for si := range p.Stocks {
		stock := p.Stocks[si]
		if stock.Quantity <= 0 || stock.Length <= 0 {
			continue
		}
		barH := stock.Width
		if barH <= 0 {
			barH = core.Millimeter
		}
		copies := stock.Quantity - usedCopies[stock.ID]
		for copy := 0; copy < copies; copy++ {
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
			for i := range parts {
				if taken[i] {
					continue
				}
				gap := core.Dim(0)
				if len(placements) > 0 {
					gap = rules.Kerf
				}
				if cursor+gap+parts[i].Length > usableEnd {
					continue
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
			if steps, ok := cutter.ForSheet(plan, rules.Kerf); ok {
				plan.CutSteps = steps
			}
			sheets = append(sheets, plan)
			stockLenM += core.LengthM(stock.Length)
			usedLenM += core.LengthM(usedLength(placements))

			if progress != nil {
				progress(assembleBars("pinned-1d", p, sheets, stockLenM, usedLenM, start, nil))
			}
		}
	}

	notes := []string{}
	if lockedCount > 0 {
		notes = append(notes, itoa(lockedCount)+" locked placement(s) were kept exactly in place.")
	}
	sol := assembleBars("pinned-1d", p, sheets, stockLenM, usedLenM, start, notes)
	sol.Unplaced = collectUnplaced(p, taken, parts)
	return sol, nil
}

func assembleBars(name string, p core.Problem, sheets []core.SheetPlan, stockLenM, usedLenM float64, start time.Time, notes []string) core.Solution {
	sol := core.Solution{
		Solver:        name,
		SolverVersion: pinnedVersion,
		Seed:          p.Seed,
		Sheets:        sheets,
		Metrics:       core.Summarize(p, sheets, nil, time.Since(start).Milliseconds()),
		Notes:         notes,
	}
	sol.Metrics.StockLengthM = stockLenM
	sol.Metrics.UsedLengthM = usedLenM
	return sol
}

func usedLength(placements []core.Placement) core.Dim {
	var used core.Dim
	for _, pl := range placements {
		used += pl.W
	}
	return used
}

// buildPinnedBar turns one locked bar into a sheet: the pins keep their X, the
// gaps between them are filled first-fit, and the remainders that meet the
// offcut policy stay in circulation.
func buildPinnedBar(pin core.PinnedSheet, parts []core.Part, taken []bool, rules core.Rules) core.SheetPlan {
	length := pin.Width
	if length <= 0 {
		return core.SheetPlan{}
	}
	barH := pin.Height
	if barH <= 0 {
		barH = core.Millimeter
	}
	plan := core.SheetPlan{
		StockID:   pin.StockID,
		StockCode: pin.StockCode,
		Label:     pin.Label,
		Width:     length,
		Height:    barH,
	}
	if plan.Label == "" {
		plan.Label = plan.StockCode
	}

	pins := append([]core.Placement(nil), pin.Placements...)
	for i := range pins {
		if pins[i].H <= 0 {
			pins[i].H = barH
		}
	}
	sort.SliceStable(pins, func(i, j int) bool {
		if pins[i].X != pins[j].X {
			return pins[i].X < pins[j].X
		}
		return pins[i].PartCode < pins[j].PartCode
	})
	plan.Placements = append(plan.Placements, pins...)

	usableStart := rules.Trim
	usableEnd := length - rules.Trim
	if usableEnd <= usableStart {
		return plan
	}

	addOffcut := func(rem core.Rect) {
		if rem.W > 0 && rem.H > 0 && rem.W >= rules.OffcutMinLength {
			plan.Offcuts = append(plan.Offcuts, rem)
		}
	}
	// The free interval before the first pin and between pins. The interval
	// ends at pin.X - kerf, so anything placed inside keeps the kerf.
	cursor := usableStart
	for _, pinPl := range pins {
		if pinPl.X-rules.Kerf-cursor > 0 {
			remainder := fillInterval(&plan, core.Rect{
				X: cursor, Y: 0, W: pinPl.X - rules.Kerf - cursor, H: barH,
			}, parts, taken, rules)
			addOffcut(remainder)
		}
		if end := pinPl.X + pinPl.W + rules.Kerf; end > cursor {
			cursor = end
		}
	}
	if usableEnd > cursor {
		remainder := fillInterval(&plan, core.Rect{X: cursor, Y: 0, W: usableEnd - cursor, H: barH},
			parts, taken, rules)
		addOffcut(remainder)
	}
	// Intervals left empty inside the pinned layout (between pins) are reported
	// by the loop above as remainders; the tail is covered by the last fill.
	return plan
}

// fillInterval places untaken parts left to right inside one free interval and
// returns what is left of the interval after the last placed piece, already
// kerf-adjusted.
func fillInterval(plan *core.SheetPlan, interval core.Rect, parts []core.Part, taken []bool, rules core.Rules) core.Rect {
	if interval.W <= 0 {
		return core.Rect{}
	}
	cursor := interval.X
	used := false
	for i := range parts {
		if taken[i] {
			continue
		}
		gap := core.Dim(0)
		if used {
			gap = rules.Kerf
		}
		if cursor+gap+parts[i].Length > interval.Right() {
			continue
		}
		pos := cursor + gap
		plan.Placements = append(plan.Placements, core.Placement{
			PartID:   parts[i].ID,
			PartCode: parts[i].Code,
			X:        pos,
			Y:        interval.Y,
			W:        parts[i].Length,
			H:        interval.H,
			Priority: parts[i].Priority,
		})
		cursor = pos + parts[i].Length
		used = true
		taken[i] = true
	}
	rem := interval.Right() - cursor
	if used {
		rem -= rules.Kerf
		cursor += rules.Kerf
	}
	if rem <= 0 {
		return core.Rect{}
	}
	return core.Rect{X: cursor, Y: interval.Y, W: rem, H: interval.H}
}

func matchPart(parts []core.Part, taken []bool, pl core.Placement) int {
	for i := range parts {
		if taken[i] {
			continue
		}
		if pl.PartID != "" && parts[i].ID == pl.PartID {
			return i
		}
	}
	for i := range parts {
		if taken[i] {
			continue
		}
		if parts[i].Code == pl.PartCode {
			return i
		}
	}
	return -1
}
