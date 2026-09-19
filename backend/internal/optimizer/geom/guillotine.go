package geom

import (
	"sort"

	"github.com/size-module/backend/internal/optimizer/core"
)

// CutNode is one node of a guillotine cut tree. A nil Children slice marks a
// leaf: either a placed part (Part != nil) or leftover material.
type CutNode struct {
	Region   core.Rect
	PartID   string
	PartCode string
	Part     *core.Rect
	Axis     string // "v" for a vertical cut, "h" for a horizontal cut
	Pos      core.Dim
	Children []*CutNode
}

// IsLeaf reports whether the node is a piece, not a cut.
func (n *CutNode) IsLeaf() bool { return n == nil || len(n.Children) == 0 }

// BuildCutTree tries to reconstruct a valid guillotine cutting sequence for the
// given part rectangles inside region. It returns false when no guillotine
// sequence exists, which is the difference between a layout that looks fine and
// a layout a panel saw can actually produce.
//
// The search probes cut lines that run along part edges (left edge minus kerf
// and right edge) so a found tree is directly translatable into instructions.
//
// It places no limit on the number of guillotine stages; use
// BuildCutTreeStages to enforce a machine's stage limit.
func BuildCutTree(region core.Rect, rects []core.Rect, ids []string, kerf core.Dim, budget int) (*CutNode, bool) {
	return BuildCutTreeStages(region, rects, ids, kerf, budget, 0)
}

// BuildCutTreeStages is BuildCutTree with a guillotine stage limit. maxStages
// counts cuts along a root-to-leaf path: shelf packing (rip strips, then
// crosscut each strip) is 2 stages. 0 means unlimited.
func BuildCutTreeStages(region core.Rect, rects []core.Rect, ids []string, kerf core.Dim, budget, maxStages int) (*CutNode, bool) {
	if budget <= 0 {
		budget = 50_000
	}
	// Deterministic ordering: identical inputs must produce identical trees.
	order := make([]int, len(rects))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		ra, rb := rects[order[a]], rects[order[b]]
		if ra.Y != rb.Y {
			return ra.Y < rb.Y
		}
		if ra.X != rb.X {
			return ra.X < rb.X
		}
		if ra.W != rb.W {
			return ra.W < rb.W
		}
		return ids[order[a]] < ids[order[b]]
	})
	sortedRects := make([]core.Rect, len(rects))
	sortedIDs := make([]string, len(ids))
	for i, idx := range order {
		sortedRects[i] = rects[idx]
		sortedIDs[i] = ids[idx]
	}

	b := &treeBuilder{kerf: kerf, budget: budget, maxStages: maxStages}
	return b.build(region, sortedRects, sortedIDs, 0)
}

// CutTreeStages returns the number of cuts on the longest root-to-leaf path
// (0 for a single piece or an empty region). It is how callers verify a layout
// against Rules.MaxCutStages.
func CutTreeStages(node *CutNode) int {
	if node == nil || node.IsLeaf() {
		return 0
	}
	left := CutTreeStages(node.Children[0])
	right := CutTreeStages(node.Children[1])
	if right > left {
		left = right
	}
	return left + 1
}

type treeBuilder struct {
	kerf      core.Dim
	budget    int
	maxStages int
}

func (b *treeBuilder) build(region core.Rect, rects []core.Rect, ids []string, depth int) (*CutNode, bool) {
	if b.budget <= 0 {
		return nil, false
	}
	b.budget--

	// Every piece must lie inside the region; otherwise the caller's layout is
	// already invalid.
	for _, r := range rects {
		if r.X < region.X || r.Y < region.Y || r.Right() > region.Right() || r.Bottom() > region.Bottom() {
			return nil, false
		}
	}

	switch len(rects) {
	case 0:
		return &CutNode{Region: region}, true
	case 1:
		r := rects[0]
		return &CutNode{Region: region, PartID: ids[0], PartCode: ids[0], Part: &r}, true
	}

	// A stage limit means this region cannot be cut again: more than one piece
	// left here has no valid sequence within the budget.
	if b.maxStages > 0 && depth >= b.maxStages {
		return nil, false
	}

	// Try vertical cuts first, then horizontal ones.
	if node, ok := b.tryCut(region, rects, ids, "v", depth); ok {
		return node, true
	}
	if node, ok := b.tryCut(region, rects, ids, "h", depth); ok {
		return node, true
	}
	return nil, false
}

func (b *treeBuilder) tryCut(region core.Rect, rects []core.Rect, ids []string, axis string, depth int) (*CutNode, bool) {
	type candidate struct {
		pos   core.Dim
		score int
	}
	var candidates []candidate
	seen := map[core.Dim]bool{}

	add := func(pos core.Dim) {
		if seen[pos] {
			return
		}
		seen[pos] = true
		candidates = append(candidates, candidate{pos: pos})
	}

	for _, r := range rects {
		if axis == "v" {
			add(r.Right())
			add(r.X - b.kerf)
		} else {
			add(r.Bottom())
			add(r.Y - b.kerf)
		}
	}

	// Prefer cuts as close to the usable edge as possible: this reproduces the
	// natural "rip strips first, then crosscut" workshop order.
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].pos < candidates[j].pos })

	for _, c := range candidates {
		var left, right []core.Rect
		var leftIDs, rightIDs []string
		valid := true
		for i, r := range rects {
			var before, after bool
			if axis == "v" {
				before = r.Right() <= c.pos
				after = r.X >= c.pos+b.kerf
			} else {
				before = r.Bottom() <= c.pos
				after = r.Y >= c.pos+b.kerf
			}
			switch {
			case before:
				left = append(left, r)
				leftIDs = append(leftIDs, ids[i])
			case after:
				right = append(right, r)
				rightIDs = append(rightIDs, ids[i])
			default:
				valid = false
			}
			if !valid {
				break
			}
		}
		if !valid || len(left) == 0 || len(right) == 0 {
			continue
		}

		var regionA, regionB core.Rect
		if axis == "v" {
			regionA, regionB = SplitV(region, c.pos, b.kerf)
		} else {
			regionA, regionB = SplitH(region, c.pos, b.kerf)
		}
		if regionA.W <= 0 || regionA.H <= 0 || regionB.W <= 0 || regionB.H <= 0 {
			continue
		}

		leftNode, okL := b.build(regionA, left, leftIDs, depth+1)
		if !okL {
			continue
		}
		rightNode, okR := b.build(regionB, right, rightIDs, depth+1)
		if !okR {
			continue
		}
		return &CutNode{
			Region:   region,
			Axis:     axis,
			Pos:      c.pos,
			Children: []*CutNode{leftNode, rightNode},
		}, true
	}
	return nil, false
}

// Instructions converts a cut tree into an ordered, human readable cut list.
func Instructions(node *CutNode, kerf core.Dim) []string {
	var steps []string
	var walk func(n *CutNode, depth int)
	walk = func(n *CutNode, depth int) {
		if n == nil || n.IsLeaf() {
			return
		}
		label := "vertical (rip)"
		if n.Axis == "h" {
			label = "horizontal (crosscut)"
		}
		steps = append(steps, formatStep(label, n.Pos, n.Region, kerf))
		walk(n.Children[0], depth+1)
		walk(n.Children[1], depth+1)
	}
	walk(node, 0)
	return steps
}

func formatStep(label string, pos core.Dim, region core.Rect, kerf core.Dim) string {
	offset := pos - region.X
	return label + " cut at " + formatDim(offset) + " mm from the region edge, kerf " + formatDim(kerf) + " mm"
}

func formatDim(d core.Dim) string {
	// Small helper kept local to avoid pulling formatting dependencies into
	// the geometry package.
	neg := d < 0
	if neg {
		d = -d
	}
	whole := d / core.Millimeter
	frac := d % core.Millimeter
	s := itoa(whole)
	if frac != 0 {
		s += "." + pad3(itoa(frac))
	}
	if neg {
		s = "-" + s
	}
	return s
}

func itoa(v core.Dim) string {
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

func pad3(s string) string {
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}
