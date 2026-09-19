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

const maxRectsVersion = "0.1.0"

// MaxRectsSolver packs parts into maximal free rectangles. Unlike the beam
// solver it enforces no guillotine constraint at all, which is the right model
// for machines that cut freely — CNC routers, lasers, waterjets — and can pack
// a few percent tighter because of it.
//
// The trade-off is explicit in Capabilities.CutMode: layouts produced here are
// not guaranteed to have an edge-to-edge cut sequence, so the registry never
// hands a guillotine order to this solver.
type MaxRectsSolver struct{}

func NewMaxRects() *MaxRectsSolver { return &MaxRectsSolver{} }

func (s *MaxRectsSolver) Name() string    { return "maxrects-2d" }
func (s *MaxRectsSolver) Version() string { return maxRectsVersion }

func (s *MaxRectsSolver) Capabilities() core.Capabilities {
	return core.Capabilities{
		Dimension:   core.Profile2D,
		CutMode:     core.CutFree,
		Rotation:    true,
		Grain:       true,
		Remnants:    true,
		Rank:        10,
		Description: "MaxRects free-rectangle packer for CNC, laser and waterjet. Layouts need not be guillotine-cuttable.",
	}
}

func (s *MaxRectsSolver) Solve(ctx context.Context, p core.Problem, progress core.ProgressFunc) (core.Solution, error) {
	start := time.Now()
	rules := normalizeRules(p.Rules)
	instances := expand(p.Parts)
	sortInstances(instances)
	taken := make([]bool, len(instances))

	var sheets []core.SheetPlan
	notes := []string{}
	freeCutNoted := false

	for si := range p.Stocks {
		stock := p.Stocks[si]
		if stock.Quantity <= 0 || stock.Width <= 0 || stock.Height <= 0 {
			continue
		}
		for copy := 0; copy < stock.Quantity; copy++ {
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

			placements, free := s.packSheet(ctx, usable, stock.Defects, rules, instances, taken)
			if len(placements) == 0 {
				continue
			}
			commitPlacements(placements, instances, taken)

			sheet := core.SheetPlan{
				Index:      len(sheets),
				StockID:    stock.ID,
				StockCode:  stock.Code,
				Label:      stock.Label,
				Width:      stock.Width,
				Height:     stock.Height,
				Placements: placements,
				Offcuts:    freeOffcuts(free, rules, stock.Defects),
			}
			if sheet.Label == "" {
				sheet.Label = stock.Code
			}
			// A free layout only happens to have a guillotine sequence
			// sometimes; instructions are attached when they exist.
			if steps, ok := cutter.ForSheet(sheet, rules.Kerf); ok {
				sheet.CutSteps = steps
			} else if !freeCutNoted {
				notes = append(notes, "Free-cut layouts have no guillotine cut sequence; this plan is for a CNC, laser or waterjet.")
				freeCutNoted = true
			}
			sheets = append(sheets, sheet)

			if progress != nil {
				progress(assembleSolution("maxrects-2d", maxRectsVersion, p, sheets, start, notes))
			}
		}
	}

	sol := assembleSolution("maxrects-2d", maxRectsVersion, p, sheets, start, notes)
	sol.Unplaced = collectUnplaced(p, taken, instances)
	return sol, nil
}

// mrRule selects which MaxRects scoring rule drives a packing attempt. Trying
// several rules per sheet and keeping the best one costs a few milliseconds and
// removes the need to guess which rule suits a given order.
type mrRule int

const (
	mrBestAreaFit mrRule = iota
	mrBestShortSideFit
	mrBottomLeft
)

var mrRules = []mrRule{mrBestAreaFit, mrBestShortSideFit, mrBottomLeft}

// packSheet tries every scoring rule and keeps the arrangement that packs the
// most part area plus reusable offcut area.
func (s *MaxRectsSolver) packSheet(
	ctx context.Context,
	usable core.Rect,
	defects []core.Rect,
	rules core.Rules,
	instances []instance,
	taken []bool,
) ([]core.Placement, []core.Rect) {
	bestScore := 0.0
	var bestPlacements []core.Placement
	var bestFree []core.Rect

	for _, rule := range mrRules {
		usedInSheet := map[string]int{}
		placements, free := s.packSheetWith(ctx, rule, usable, defects, rules, instances, taken, usedInSheet)
		if len(placements) == 0 {
			continue
		}
		score := mrSheetScore(placements, free, rules, defects)
		if bestPlacements == nil ||
			score > bestScore+1e-9 ||
			(math.Abs(score-bestScore) <= 1e-9 && len(placements) > len(bestPlacements)) {
			bestScore = score
			bestPlacements = placements
			bestFree = free
		}
	}
	return bestPlacements, bestFree
}

// mrSheetScore mirrors the global objective at sheet level: packed part area
// plus a quarter of the reusable offcut area.
func mrSheetScore(placements []core.Placement, free []core.Rect, rules core.Rules, defects []core.Rect) float64 {
	partArea := 0.0
	for _, pl := range placements {
		partArea += float64(pl.W) * float64(pl.H)
	}
	offcutArea := 0.0
	for _, off := range freeOffcuts(free, rules, defects) {
		offcutArea += float64(off.W) * float64(off.H)
	}
	return partArea + 0.25*offcutArea
}

// packSheetWith fills one usable area with the MaxRects heuristic: maintain the
// maximal free rectangles, place the best-fitting piece in the corner of one of
// them, split the affected rectangles and prune the ones that are contained in
// another.
func (s *MaxRectsSolver) packSheetWith(
	ctx context.Context,
	rule mrRule,
	usable core.Rect,
	defects []core.Rect,
	rules core.Rules,
	instances []instance,
	taken []bool,
	usedInSheet map[string]int,
) ([]core.Placement, []core.Rect) {
	free := []core.Rect{usable}
	cands := remainingCandidates(instances, taken, defaultBeamCands)
	var placements []core.Placement

	for {
		if ctx.Err() != nil {
			break
		}
		if rules.MaxPartsPerSheet > 0 && len(placements) >= rules.MaxPartsPerSheet {
			break
		}

		best, ok := s.bestPlacement(free, cands, usedInSheet, defects, rules, rule)
		if !ok {
			break
		}
		piece := core.Rect{X: best.rect.X, Y: best.rect.Y, W: best.w, H: best.h}
		placements = append(placements, core.Placement{
			PartID:   best.part.ID,
			PartCode: best.part.Code,
			X:        piece.X,
			Y:        piece.Y,
			W:        best.w,
			H:        best.h,
			Rotated:  best.rot,
			Priority: best.part.Priority,
		})
		usedInSheet[best.part.ID]++

		// Inflate the used area by the kerf so the next piece cannot touch it.
		used := core.Rect{X: piece.X, Y: piece.Y, W: piece.W + rules.Kerf, H: piece.H + rules.Kerf}
		free = splitFree(free, used)
		free = pruneFree(free)
	}
	return placements, free
}

type mrPlacement struct {
	part core.Part
	rect core.Rect
	w, h core.Dim
	rot  bool
}

// bestPlacement picks the (piece, rectangle, orientation) with the best score
// under the active rule. All comparisons are total, so the result is
// deterministic.
func (s *MaxRectsSolver) bestPlacement(
	free []core.Rect,
	cands []candidatePart,
	usedInSheet map[string]int,
	defects []core.Rect,
	rules core.Rules,
	rule mrRule,
) (mrPlacement, bool) {
	best := mrPlacement{}
	found := false
	for _, cand := range cands {
		if usedInSheet[cand.part.ID] >= cand.remaining {
			continue
		}
		for _, o := range beamOrientations(cand.part, rules) {
			for _, rect := range free {
				if o.w > rect.W || o.h > rect.H {
					continue
				}
				piece := core.Rect{X: rect.X, Y: rect.Y, W: o.w, H: o.h}
				if hitsDefect(piece, defects) {
					continue
				}
				if !found || mrBetter(rule, o.w, o.h, rect, cand.part, best) {
					best = mrPlacement{part: cand.part, rect: rect, w: o.w, h: o.h, rot: o.rot}
					found = true
				}
			}
		}
	}
	return best, found
}

func mrBetter(rule mrRule, w, h core.Dim, rect core.Rect, part core.Part, best mrPlacement) bool {
	leftoverA := rect.W*rect.H - w*h
	leftoverB := best.rect.W*best.rect.H - best.w*best.h
	shortA := minDim(rect.W-w, rect.H-h)
	shortB := minDim(best.rect.W-best.w, best.rect.H-best.h)

	switch rule {
	case mrBestShortSideFit:
		if shortA != shortB {
			return shortA < shortB
		}
		if leftoverA != leftoverB {
			return leftoverA < leftoverB
		}
	case mrBottomLeft:
		if rect.Y != best.rect.Y {
			return rect.Y < best.rect.Y
		}
		if rect.X != best.rect.X {
			return rect.X < best.rect.X
		}
		if leftoverA != leftoverB {
			return leftoverA < leftoverB
		}
	default: // mrBestAreaFit
		if leftoverA != leftoverB {
			return leftoverA < leftoverB
		}
		if shortA != shortB {
			return shortA < shortB
		}
	}
	if rect.Y != best.rect.Y {
		return rect.Y < best.rect.Y
	}
	if rect.X != best.rect.X {
		return rect.X < best.rect.X
	}
	return part.Code < best.part.Code
}

func minDim(a, b core.Dim) core.Dim {
	if a < b {
		return a
	}
	return b
}

// splitFree replaces every free rectangle that intersects used with the up to
// four rectangles around it (the classic MaxRects split).
func splitFree(free []core.Rect, used core.Rect) []core.Rect {
	out := make([]core.Rect, 0, len(free)+4)
	for _, node := range free {
		if !geom.Intersects(node, used) {
			out = append(out, node)
			continue
		}
		// Above the used area.
		if used.Y > node.Y && used.Y < node.Bottom() {
			candidate := node
			candidate.H = used.Y - node.Y
			if candidate.W > 0 && candidate.H > 0 {
				out = append(out, candidate)
			}
		}
		// Below the used area.
		if used.Bottom() < node.Bottom() {
			candidate := node
			candidate.Y = used.Bottom()
			candidate.H = node.Bottom() - used.Bottom()
			if candidate.W > 0 && candidate.H > 0 {
				out = append(out, candidate)
			}
		}
		// Left of the used area.
		if used.X > node.X && used.X < node.Right() {
			candidate := node
			candidate.W = used.X - node.X
			if candidate.W > 0 && candidate.H > 0 {
				out = append(out, candidate)
			}
		}
		// Right of the used area.
		if used.Right() < node.Right() {
			candidate := node
			candidate.X = used.Right()
			candidate.W = node.Right() - used.Right()
			if candidate.W > 0 && candidate.H > 0 {
				out = append(out, candidate)
			}
		}
	}
	return out
}

// pruneFree drops any free rectangle contained in another one.
func pruneFree(free []core.Rect) []core.Rect {
	keep := make([]bool, len(free))
	for i := range keep {
		keep[i] = true
	}
	for i := range free {
		if !keep[i] {
			continue
		}
		for j := range free {
			if i == j || !keep[i] || free[i] == free[j] {
				continue
			}
			if containsRect(free[j], free[i]) {
				keep[i] = false
			}
		}
	}
	out := make([]core.Rect, 0, len(free))
	for i := range free {
		if keep[i] {
			out = append(out, free[i])
		}
	}
	return out
}

func containsRect(outer, inner core.Rect) bool {
	return inner.X >= outer.X && inner.Y >= outer.Y &&
		inner.Right() <= outer.Right() && inner.Bottom() <= outer.Bottom()
}

// freeOffcuts reports the largest free rectangles that are big enough to be
// reused. MaxRects free rectangles overlap, so they are picked greedily and
// any rectangle intersecting an already chosen offcut is skipped: the reported
// offcut area is never double counted.
func freeOffcuts(free []core.Rect, rules core.Rules, defects []core.Rect) []core.Rect {
	if rules.OffcutMinW <= 0 || rules.OffcutMinH <= 0 {
		return nil
	}
	candidates := make([]core.Rect, 0, len(free))
	for _, r := range free {
		if r.W >= rules.OffcutMinW && r.H >= rules.OffcutMinH && !hitsDefect(r, defects) {
			candidates = append(candidates, r)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		areaI, areaJ := candidates[i].W*candidates[i].H, candidates[j].W*candidates[j].H
		if areaI != areaJ {
			return areaI > areaJ
		}
		if candidates[i].Y != candidates[j].Y {
			return candidates[i].Y < candidates[j].Y
		}
		return candidates[i].X < candidates[j].X
	})

	var chosen []core.Rect
	for _, r := range candidates {
		overlap := false
		for _, c := range chosen {
			if geom.Intersects(r, c) {
				overlap = true
				break
			}
		}
		if !overlap {
			chosen = append(chosen, r)
		}
	}
	sort.SliceStable(chosen, func(i, j int) bool {
		if chosen[i].Y != chosen[j].Y {
			return chosen[i].Y < chosen[j].Y
		}
		return chosen[i].X < chosen[j].X
	})
	return chosen
}
