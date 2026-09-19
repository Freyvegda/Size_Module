package pack2d

import (
	"context"
	"sort"
	"time"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/cutter"
)

const beamVersion = "0.1.0"

const (
	// Width 24 measured best on the benchmark set: wider beams are not
	// monotonically better because a broader search keeps states that fill
	// worse later. The portfolio exists for the same reason.
	defaultBeamWidth  = 24
	defaultBeamCands  = 8
	defaultBeamRects  = 8
	maxBeamCandidates = 64
)

// BeamSolver explores many guillotine cut trees per sheet instead of committing
// to one strip order. Every placement is made in the lower-left corner of a
// free region and the two possible guillotine splits (cut above the piece or
// beside it) become separate search states. A beam of the best states is kept
// per level, scored by packed area, unusable fragments and fragmentation.
//
// The solver is any-time: when the context expires it returns the best state
// found so far.
type BeamSolver struct {
	// Width is the number of states kept per expansion level.
	Width int
	// CandidatesPerStep caps how many distinct part groups are tried per step
	// (the largest by area, plus the two smallest to fill slivers).
	CandidatesPerStep int
}

func NewBeam() *BeamSolver {
	return &BeamSolver{Width: defaultBeamWidth, CandidatesPerStep: defaultBeamCands}
}

func (s *BeamSolver) Name() string    { return "beam-2d" }
func (s *BeamSolver) Version() string { return beamVersion }

func (s *BeamSolver) Capabilities() core.Capabilities {
	return core.Capabilities{
		Dimension:   core.Profile2D,
		CutMode:     core.CutGuillotine,
		Rotation:    true,
		Grain:       true,
		Remnants:    true,
		Rank:        10,
		Description: "Beam search over guillotine cut trees. Better packing than shelf on mixed orders, still fully guillotine-safe.",
	}
}

func (s *BeamSolver) beamWidth() int {
	if s.Width > 0 {
		return s.Width
	}
	return defaultBeamWidth
}

func (s *BeamSolver) maxCandidates() int {
	if s.CandidatesPerStep > 0 {
		return s.CandidatesPerStep
	}
	return defaultBeamCands
}

// candidatePart is one distinct part type with its remaining quantity.
type candidatePart struct {
	part      core.Part
	remaining int
	area      core.Dim
}

// beamNode is one search state. Placements are linked through parent pointers so
// copying a state costs one small slice (the free rectangles), not the whole
// history.
type beamNode struct {
	free   []core.Rect
	placed int
	area   core.Dim
	last   core.Placement
	parent *beamNode
	score  float64
}

func (n *beamNode) placements() []core.Placement {
	out := make([]core.Placement, 0, n.placed)
	for node := n; node != nil && node.placed > 0; node = node.parent {
		out = append(out, node.last)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func (n *beamNode) countPart(partID string) int {
	count := 0
	for node := n; node != nil && node.placed > 0; node = node.parent {
		if node.last.PartID == partID {
			count++
		}
	}
	return count
}

func (s *BeamSolver) Solve(ctx context.Context, p core.Problem, progress core.ProgressFunc) (core.Solution, error) {
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
	var sheets []core.SheetPlan
	var notes []string

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

			node, ok := s.packSheet(ctx, usable, stock.Defects, rules, instances, taken)
			if !ok {
				continue
			}
			placements := node.placements()
			commitPlacements(placements, instances, taken)

			sheet := sheetFromNode(stock, rules, node, len(sheets))
			if rules.CutMode == core.CutGuillotine && len(sheet.CutSteps) == 0 {
				notes = append(notes, "sheet "+itoa(sheet.Index+1)+
					": beam layout failed guillotine verification, which should be impossible")
			}
			sheets = append(sheets, sheet)

			if progress != nil {
				progress(assembleSolution("beam-2d", beamVersion, p, sheets, start, notes))
			}
		}
	}

	sol := assembleSolution("beam-2d", beamVersion, p, sheets, start, notes)
	sol.Unplaced = collectUnplaced(p, taken, instances)
	return sol, nil
}

// packSheet runs the beam for one sheet and returns the best final state.
func (s *BeamSolver) packSheet(ctx context.Context, usable core.Rect, defects []core.Rect, rules core.Rules, instances []instance, taken []bool) (*beamNode, bool) {
	cands := remainingCandidates(instances, taken, s.maxCandidates())
	if len(cands) == 0 {
		return nil, false
	}

	root := &beamNode{free: []core.Rect{usable}}
	best := root
	beam := []*beamNode{root}

	for {
		if ctx.Err() != nil {
			break
		}
		children := make([]*beamNode, 0, len(beam)*maxBeamCandidates)
		for _, node := range beam {
			children = append(children, s.expand(node, usable, defects, rules, cands, rules.MaxPartsPerSheet)...)
		}
		if len(children) == 0 {
			break
		}
		sortChildren(children)
		if len(children) > s.beamWidth() {
			children = children[:s.beamWidth()]
		}
		beam = children
		if beam[0].score > best.score {
			best = beam[0]
		}
	}
	if best.placed == 0 {
		return nil, false
	}
	return best, true
}

func (s *BeamSolver) expand(node *beamNode, usable core.Rect, defects []core.Rect, rules core.Rules, cands []candidatePart, maxParts int) []*beamNode {
	if maxParts > 0 && node.placed >= maxParts {
		return nil
	}

	seen := map[string]bool{}
	children := make([]*beamNode, 0, maxBeamCandidates)
	for _, ri := range orderRects(node.free, defaultBeamRects) {
		rect := node.free[ri]
		for ci := range cands {
			cand := cands[ci]
			if node.countPart(cand.part.ID) >= cand.remaining {
				continue
			}
			for _, o := range beamOrientations(cand.part, rules) {
				if o.w > rect.W || o.h > rect.H {
					continue
				}
				piece := core.Rect{X: rect.X, Y: rect.Y, W: o.w, H: o.h}
				if hitsDefect(piece, defects) {
					continue
				}
				pl := core.Placement{
					PartID:   cand.part.ID,
					PartCode: cand.part.Code,
					X:        piece.X,
					Y:        piece.Y,
					W:        o.w,
					H:        o.h,
					Rotated:  o.rot,
					Priority: cand.part.Priority,
				}
				for mode := 0; mode < 2; mode++ {
					key := rectMoveKey(ri, o.w, o.h, mode)
					if seen[key] {
						continue
					}
					seen[key] = true
					child := buildChild(node, ri, piece, pl, mode, rules, usable, cands)
					if child != nil {
						children = append(children, child)
					}
				}
			}
		}
	}
	return children
}

// buildChild applies one placement to a free rectangle and creates the two
// guillotine residual layouts.
//
//	mode 0: crosscut above the piece first, then rip beside it
//	mode 1: rip beside the piece first, then crosscut above it
func buildChild(parent *beamNode, rectIndex int, piece core.Rect, pl core.Placement, mode int, rules core.Rules, usable core.Rect, cands []candidatePart) *beamNode {
	rect := parent.free[rectIndex]
	free := make([]core.Rect, 0, len(parent.free)+1)
	free = append(free, parent.free[:rectIndex]...)
	free = append(free, parent.free[rectIndex+1:]...)

	var first, second core.Rect
	if mode == 0 {
		first = core.Rect{X: rect.X, Y: piece.Bottom() + rules.Kerf, W: rect.W, H: rect.Bottom() - piece.Bottom() - rules.Kerf}
		second = core.Rect{X: piece.Right() + rules.Kerf, Y: rect.Y, W: rect.Right() - piece.Right() - rules.Kerf, H: piece.H}
	} else {
		first = core.Rect{X: piece.Right() + rules.Kerf, Y: rect.Y, W: rect.Right() - piece.Right() - rules.Kerf, H: rect.H}
		second = core.Rect{X: rect.X, Y: piece.Bottom() + rules.Kerf, W: piece.W, H: rect.Bottom() - piece.Bottom() - rules.Kerf}
	}
	if first.W > 0 && first.H > 0 {
		free = append(free, first)
	}
	if second.W > 0 && second.H > 0 {
		free = append(free, second)
	}

	child := &beamNode{
		free:   free,
		placed: parent.placed + 1,
		area:   parent.area + piece.W*piece.H,
		last:   pl,
		parent: parent,
	}
	child.score = scoreNode(child, usable, cands)
	return child
}

// packParts packs a fixed list of parts into one usable area. It reports false
// unless every single piece finds a place, which is what the local search needs
// to know before committing a move.
func (s *BeamSolver) packParts(ctx context.Context, usable core.Rect, defects []core.Rect, rules core.Rules, parts []core.Part) (*beamNode, bool) {
	instances := make([]instance, 0, len(parts))
	for i := range parts {
		quantity := parts[i].Quantity
		if quantity <= 0 {
			quantity = 1
		}
		for q := 0; q < quantity; q++ {
			instances = append(instances, instance{part: parts[i]})
		}
	}
	if len(instances) == 0 {
		return nil, false
	}
	taken := make([]bool, len(instances))
	node, ok := s.packSheet(ctx, usable, defects, rules, instances, taken)
	if !ok || node.placed != len(instances) {
		return nil, false
	}
	return node, true
}

// sheetFromNode builds a sheet plan from a finished search state, including
// offcuts and the guillotine cut sequence.
func sheetFromNode(stock core.StockItem, rules core.Rules, node *beamNode, index int) core.SheetPlan {
	sheet := core.SheetPlan{
		Index:      index,
		StockID:    stock.ID,
		StockCode:  stock.Code,
		Label:      stock.Label,
		Width:      stock.Width,
		Height:     stock.Height,
		Placements: node.placements(),
		Offcuts:    offcutsFromFree(node.free, rules),
	}
	if sheet.Label == "" {
		sheet.Label = stock.Code
	}
	if steps, ok := cutter.ForSheetStages(sheet, rules.Kerf, rules.MaxCutStages); ok {
		sheet.CutSteps = steps
	}
	return sheet
}

// scoreNode is the beam heuristic: packed part area, minus a penalty for
// fragments that can no longer hold anything, minus a small penalty for every
// extra free rectangle (fragmentation hurts later placements and offcut value).
func scoreNode(node *beamNode, usable core.Rect, cands []candidatePart) float64 {
	fragments := len(node.free) - 1
	if fragments < 0 {
		fragments = 0
	}
	fragPenalty := 0.01 * float64(usable.W) * float64(usable.H) * float64(fragments)
	return float64(node.area) - 0.25*float64(unusableFree(node.free, cands)) - fragPenalty
}

func unusableFree(free []core.Rect, cands []candidatePart) core.Dim {
	var total core.Dim
	for _, r := range free {
		if !fitsAnyCandidate(r, cands) {
			total += r.W * r.H
		}
	}
	return total
}

// fitsAnyCandidate ignores per-part rotation flags on purpose: it only estimates
// whether a region is worth keeping in the search.
func fitsAnyCandidate(r core.Rect, cands []candidatePart) bool {
	for _, c := range cands {
		if c.part.Width <= r.W && c.part.Height <= r.H {
			return true
		}
		if c.part.Height <= r.W && c.part.Width <= r.H {
			return true
		}
	}
	return false
}

func sortChildren(children []*beamNode) {
	sort.SliceStable(children, func(i, j int) bool {
		if children[i].score != children[j].score {
			return children[i].score > children[j].score
		}
		if len(children[i].free) != len(children[j].free) {
			return len(children[i].free) < len(children[j].free)
		}
		return children[i].last.PartCode < children[j].last.PartCode
	})
}

// orderRects returns the indexes of the smallest free rectangles first: filling
// the tightest gaps keeps large contiguous areas available for large pieces.
func orderRects(free []core.Rect, limit int) []int {
	idx := make([]int, len(free))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ra, rb := free[idx[a]], free[idx[b]]
		areaA, areaB := ra.W*ra.H, rb.W*rb.H
		if areaA != areaB {
			return areaA < areaB
		}
		if ra.Y != rb.Y {
			return ra.Y < rb.Y
		}
		if ra.X != rb.X {
			return ra.X < rb.X
		}
		return idx[a] < idx[b]
	})
	if limit > 0 && len(idx) > limit {
		idx = idx[:limit]
	}
	return idx
}

func rectMoveKey(rectIndex int, w, h core.Dim, mode int) string {
	return itoa(rectIndex) + ":" + dimKey(w) + ":" + dimKey(h) + ":" + itoa(mode)
}

func dimKey(v core.Dim) string {
	return itoa(int(v))
}

// remainingCandidates aggregates untaken instances by part type, keeps the
// largest ones and always adds the two smallest so slivers can still be filled.
func remainingCandidates(instances []instance, taken []bool, max int) []candidatePart {
	type agg struct {
		part  core.Part
		count int
	}
	byID := map[string]*agg{}
	var order []string
	for i := range instances {
		if taken[i] {
			continue
		}
		part := instances[i].part
		a, ok := byID[part.ID]
		if !ok {
			a = &agg{part: part}
			byID[part.ID] = a
			order = append(order, part.ID)
		}
		a.count++
	}

	list := make([]candidatePart, 0, len(order))
	for _, id := range order {
		a := byID[id]
		list = append(list, candidatePart{
			part:      a.part,
			remaining: a.count,
			area:      a.part.Width * a.part.Height,
		})
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].area != list[j].area {
			return list[i].area > list[j].area
		}
		return list[i].part.Code < list[j].part.Code
	})

	if max <= 0 || len(list) <= max {
		return list
	}
	head := max - 2
	if head < 1 {
		head = max
	}
	if head > len(list) {
		head = len(list)
	}
	selected := make([]candidatePart, 0, max)
	selected = append(selected, list[:head]...)
	for _, c := range list[len(list)-2:] {
		dup := false
		for i := range selected {
			if selected[i].part.ID == c.part.ID {
				dup = true
				break
			}
		}
		if !dup {
			selected = append(selected, c)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].area != selected[j].area {
			return selected[i].area > selected[j].area
		}
		return selected[i].part.Code < selected[j].part.Code
	})
	return selected
}

func beamOrientations(part core.Part, rules core.Rules) []orientation {
	out := []orientation{{w: part.Width, h: part.Height}}
	if mayRotate(part, rules) && part.Width != part.Height {
		out = append(out, orientation{w: part.Height, h: part.Width, rot: true})
	}
	return out
}

func mayRotate(part core.Part, rules core.Rules) bool {
	return rules.AllowRotate && part.AllowRotate && part.Grain == core.GrainNone
}

// commitPlacements marks the matching instances as taken.
func commitPlacements(placements []core.Placement, instances []instance, taken []bool) {
	for _, pl := range placements {
		for i := range instances {
			if !taken[i] && instances[i].part.ID == pl.PartID {
				taken[i] = true
				break
			}
		}
	}
}

// offcutsFromFree reports the free rectangles that are big enough to be put
// back into stock as remnants.
func offcutsFromFree(free []core.Rect, rules core.Rules) []core.Rect {
	if rules.OffcutMinW <= 0 || rules.OffcutMinH <= 0 {
		return nil
	}
	var out []core.Rect
	for _, r := range free {
		if r.W >= rules.OffcutMinW && r.H >= rules.OffcutMinH {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].X < out[j].X
	})
	return out
}
