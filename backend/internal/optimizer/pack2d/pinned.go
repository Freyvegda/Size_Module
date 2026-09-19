package pack2d

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/cutter"
	"github.com/size-module/backend/internal/optimizer/geom"
)

const pinnedVersion = "0.1.0"

// PinnedSolver packs the residual demand around planner-locked placements.
//
// Each core.PinnedSheet is materialised exactly as locked: the solver builds a
// guillotine cut tree over the locked pieces, treats the leftover leaves as
// free regions and fills them shelf-style with the remaining parts. The rest of
// the stock (one copy per pinned sheet is consumed) is packed like the shelf
// baseline. Without pins it behaves exactly like shelf-2d.
type PinnedSolver struct{}

func NewPinned() *PinnedSolver { return &PinnedSolver{} }

func (s *PinnedSolver) Name() string    { return "pinned-2d" }
func (s *PinnedSolver) Version() string { return pinnedVersion }

func (s *PinnedSolver) Capabilities() core.Capabilities {
	return core.Capabilities{
		Dimension: core.Profile2D,
		CutMode:   core.CutGuillotine,
		Rotation:  true,
		Grain:     true,
		Remnants:  true,
		Pinned:    true,
		Rank:      40,
		Description: "Shelf/strip packer that honours locked placements: keeps them exactly " +
			"in place and packs the remaining parts around them. Used for re-solving edited plans.",
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

	instances := expand(p.Parts)
	sortInstances(instances)
	taken := make([]bool, len(instances))

	// The locked placements already produce that many pieces of demand.
	for _, pin := range p.Pinned {
		for _, pl := range pin.Placements {
			if i := matchInstance(instances, taken, pl); i >= 0 {
				taken[i] = true
			}
		}
	}

	var sheets []core.SheetPlan
	var notes []string
	lockedCount := 0

	for _, pin := range p.Pinned {
		sheet, free, ok := buildPinnedSheet(pin, rules, instances, taken)
		if !ok {
			notes = append(notes, "pinned sheet "+itoa(len(sheets)+1)+
				": the locked pieces have no valid guillotine cut sequence on their own; they were kept as-is")
		}
		sheet.Index = len(sheets)
		lockedCount += len(pin.Placements)
		_ = free

		if rules.CutMode == core.CutGuillotine && len(sheet.Placements) > 0 {
			steps, ok := cutter.ForSheetStages(sheet, rules.Kerf, rules.MaxCutStages)
			if ok {
				sheet.CutSteps = steps
			} else {
				notes = append(notes, "sheet "+itoa(sheet.Index+1)+
					": no valid guillotine cut sequence found; use free cut mode or adjust rules")
			}
		}
		sheets = append(sheets, sheet)
		if progress != nil {
			progress(assembleSolution("pinned-2d", pinnedVersion, p, sheets, start, notes))
		}
	}

	// The remaining copies of every stock: a pinned sheet occupies one copy.
	usedCopies := map[string]int{}
	for _, pin := range p.Pinned {
		if pin.StockID != "" {
			usedCopies[pin.StockID]++
		}
	}
	for si := range p.Stocks {
		stock := p.Stocks[si]
		if stock.Quantity <= 0 || stock.Width <= 0 || stock.Height <= 0 {
			continue
		}
		copies := stock.Quantity - usedCopies[stock.ID]
		for copy := 0; copy < copies; copy++ {
			if ctx.Err() != nil || allTaken(taken) {
				break
			}
			usable := core.Rect{
				X: rules.Trim,
				Y: rules.Trim,
				W: stock.Width - 2*rules.Trim,
				H: stock.Height - 2*rules.Trim,
			}
			if usable.W <= 0 || usable.H <= 0 {
				continue
			}
			plan, placedAny := packSheet(usable, stock.Defects, rules, instances, taken)
			if !placedAny {
				continue
			}
			plan.Index = len(sheets)
			plan.StockID = stock.ID
			plan.StockCode = stock.Code
			plan.Label = stock.Label
			if plan.Label == "" {
				plan.Label = stock.Code
			}
			plan.Width = stock.Width
			plan.Height = stock.Height
			plan.Offcuts = findOffcuts(usable, plan.Placements, rules)

			if rules.CutMode == core.CutGuillotine {
				steps, ok := cutter.ForSheetStages(plan, rules.Kerf, rules.MaxCutStages)
				if ok {
					plan.CutSteps = steps
				} else {
					notes = append(notes, "sheet "+itoa(plan.Index+1)+
						": no valid guillotine cut sequence found; use free cut mode or adjust rules")
				}
			}
			sheets = append(sheets, plan)
			if progress != nil {
				progress(assembleSolution("pinned-2d", pinnedVersion, p, sheets, start, notes))
			}
		}
	}

	if lockedCount > 0 {
		notes = append(notes, itoa(lockedCount)+" locked placement(s) were kept exactly in place.")
	}

	sol := assembleSolution("pinned-2d", pinnedVersion, p, sheets, start, notes)
	sol.Unplaced = collectUnplaced(p, taken, instances)
	sol.Metrics.ScrapAreaM2 = math.Max(0,
		sol.Metrics.StockAreaM2-sol.Metrics.PartAreaM2-sol.Metrics.OffcutAreaM2)
	sol.Metrics.TrimAreaM2, sol.Metrics.KerfAreaM2 = overheads(sheets, rules)
	return sol, nil
}

// matchInstance finds the not-yet-taken instance that a pinned placement
// produces, by part id first and code as a fallback (archived placements only
// carry the code).
func matchInstance(instances []instance, taken []bool, pl core.Placement) int {
	for i := range instances {
		if taken[i] {
			continue
		}
		if pl.PartID != "" && instances[i].part.ID == pl.PartID {
			return i
		}
	}
	for i := range instances {
		if taken[i] {
			continue
		}
		if instances[i].part.Code == pl.PartCode {
			return i
		}
	}
	return -1
}

// buildPinnedSheet turns one locked layout into a sheet: the locked pieces keep
// their exact rectangles, the free leaves of their cut tree are filled with
// remaining demand, and leftovers become offcuts. The bool is false when the
// locked pieces alone are not guillotine-separable; the sheet is then returned
// without any filling so the validator can explain the problem.
func buildPinnedSheet(pin core.PinnedSheet, rules core.Rules, instances []instance, taken []bool) (core.SheetPlan, []core.Rect, bool) {
	sheet := core.SheetPlan{
		StockID:   pin.StockID,
		StockCode: pin.StockCode,
		Label:     pin.Label,
		Width:     pin.Width,
		Height:    pin.Height,
	}
	if sheet.Label == "" {
		sheet.Label = sheet.StockCode
	}
	sheet.Placements = append(sheet.Placements, pin.Placements...)
	if pin.Width <= 0 || pin.Height <= 0 || len(pin.Placements) == 0 {
		return sheet, nil, true
	}

	usable := core.Rect{
		X: rules.Trim,
		Y: rules.Trim,
		W: pin.Width - 2*rules.Trim,
		H: pin.Height - 2*rules.Trim,
	}
	rects := make([]core.Rect, len(pin.Placements))
	ids := make([]string, len(pin.Placements))
	for i, pl := range pin.Placements {
		rects[i] = core.Rect{X: pl.X, Y: pl.Y, W: pl.W, H: pl.H}
		ids[i] = pl.PartCode
	}
	tree, ok := geom.BuildCutTree(usable, rects, ids, rules.Kerf, 0)
	if !ok {
		return sheet, nil, false
	}

	free := freeRegions(tree, rules.Kerf)
	var offcuts []core.Rect
	for _, leaf := range free {
		if leaf.W <= 0 || leaf.H <= 0 {
			continue
		}
		sub, placedAny := packSheet(leaf, nil, rules, instances, taken)
		if !placedAny {
			if leaf.W >= rules.OffcutMinW && leaf.H >= rules.OffcutMinH {
				offcuts = append(offcuts, leaf)
			}
			continue
		}
		sheet.Placements = append(sheet.Placements, sub.Placements...)
		offcuts = append(offcuts, findOffcuts(leaf, sub.Placements, rules)...)
	}
	sheet.Offcuts = offcuts
	return sheet, free, true
}

// freeRegions returns the leftover material of a cut tree whose leaves are
// locked pieces. A leaf region is a rectangle produced by the tree's cuts, so
// any further guillotine cut inside it is valid; a locked piece is subtracted
// from its leaf with the five disjoint bands around it (left/right full
// height, top/bottom of the piece column), which keeps the whole layout
// cuttable. The result is ordered largest-first for deterministic filling.
func freeRegions(root *geom.CutNode, kerf core.Dim) []core.Rect {
	var out []core.Rect
	var walk func(n *geom.CutNode)
	walk = func(n *geom.CutNode) {
		if n == nil {
			return
		}
		if !n.IsLeaf() {
			for _, child := range n.Children {
				walk(child)
			}
			return
		}
		if n.Part == nil {
			if n.Region.W > 0 && n.Region.H > 0 {
				out = append(out, n.Region)
			}
			return
		}
		out = append(out, subtractRects(n.Region, *n.Part, kerf)...)
	}
	walk(root)
	sort.SliceStable(out, func(i, j int) bool {
		areaI := out[i].W * out[i].H
		areaJ := out[j].W * out[j].H
		if areaI != areaJ {
			return areaI > areaJ
		}
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].X < out[j].X
	})
	return out
}

// subtractRects returns the region minus a piece (plus its kerf margin) as
// disjoint rectangles that can be produced by guillotine cuts: the bands left
// and right of the piece, and the bands above and below inside its column.
func subtractRects(region, part core.Rect, kerf core.Dim) []core.Rect {
	var out []core.Rect
	add := func(r core.Rect) {
		if r.W > 0 && r.H > 0 {
			out = append(out, r)
		}
	}
	leftEdge := part.X - kerf
	rightEdge := part.Right() + kerf
	topEdge := part.Y - kerf
	bottomEdge := part.Bottom() + kerf

	add(core.Rect{X: region.X, Y: region.Y, W: leftEdge - region.X, H: region.H})
	add(core.Rect{X: rightEdge, Y: region.Y, W: region.Right() - rightEdge, H: region.H})
	add(core.Rect{X: leftEdge, Y: region.Y, W: rightEdge - leftEdge, H: topEdge - region.Y})
	add(core.Rect{X: leftEdge, Y: bottomEdge, W: rightEdge - leftEdge, H: region.Bottom() - bottomEdge})
	return out
}
