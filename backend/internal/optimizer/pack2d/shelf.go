// Package pack2d contains 2D sheet solvers. ShelfSolver is the fast baseline:
// it groups parts into horizontal strips, which is exactly how a panel saw or a
// glass cutter works, so every layout it produces is guillotine-feasible.
//
// It is deliberately simple. The production roadmap replaces the strip
// heuristic with beam search over cut trees plus column generation, but this
// solver stays as the baseline every candidate is compared against.
package pack2d

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/cutter"
)

const shelfVersion = "0.1.0"

type ShelfSolver struct{}

func New() *ShelfSolver { return &ShelfSolver{} }

func (s *ShelfSolver) Name() string    { return "shelf-2d" }
func (s *ShelfSolver) Version() string { return shelfVersion }

func (s *ShelfSolver) Capabilities() core.Capabilities {
	return core.Capabilities{
		Dimension:   core.Profile2D,
		CutMode:     core.CutGuillotine,
		Rotation:    true,
		Grain:       true,
		Remnants:    true,
		Rank:        20,
		Description: "Shelf/strip packer producing guillotine-cuttable layouts. Baseline for panels and sheets.",
	}
}

type instance struct {
	part core.Part
	seq  int
}

// strip is one horizontal band of a sheet; a strip is created by a full-width
// rip cut, so all pieces inside it share the same height.
type strip struct {
	y     core.Dim
	h     core.Dim
	right core.Dim // right edge of the last placed piece
	used  bool
}

type orientation struct {
	w, h core.Dim
	rot  bool
}

func orientations(part core.Part, canRotate bool) []orientation {
	out := []orientation{{w: part.Width, h: part.Height}}
	if canRotate && part.Width != part.Height {
		out = append(out, orientation{w: part.Height, h: part.Width, rot: true})
	}
	return out
}

func (s *ShelfSolver) Solve(ctx context.Context, p core.Problem, progress core.ProgressFunc) (core.Solution, error) {
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
				steps, ok := cutter.ForSheet(plan, rules.Kerf)
				if ok {
					plan.CutSteps = steps
				} else {
					notes = append(notes, "sheet "+itoa(plan.Index+1)+
						": no valid guillotine cut sequence found; use free cut mode or adjust rules")
				}
			}
			sheets = append(sheets, plan)

			if progress != nil {
				progress(assembleSolution("shelf-2d", shelfVersion, p, sheets, start, notes))
			}
		}
	}

	sol := assembleSolution("shelf-2d", shelfVersion, p, sheets, start, notes)
	sol.Unplaced = collectUnplaced(p, taken, instances)
	sol.Metrics.ScrapAreaM2 = math.Max(0,
		sol.Metrics.StockAreaM2-sol.Metrics.PartAreaM2-sol.Metrics.OffcutAreaM2)
	sol.Metrics.TrimAreaM2, sol.Metrics.KerfAreaM2 = overheads(sheets, rules)
	return sol, nil
}

// packSheet fills one usable area, updating taken in place.
func packSheet(usable core.Rect, defects []core.Rect, rules core.Rules, instances []instance, taken []bool) (core.SheetPlan, bool) {
	var strips []strip
	var placements []core.Placement
	placedAny := false

	for i := range instances {
		if taken[i] {
			continue
		}
		if rules.MaxPartsPerSheet > 0 && len(placements) >= rules.MaxPartsPerSheet {
			break
		}
		part := instances[i].part
		if part.Width <= 0 || part.Height <= 0 {
			continue // 1d parts are handled by the 1d solver
		}

		if pl, ok := placeInStrips(&strips, usable, part, rules, defects); ok {
			placements = append(placements, pl)
			taken[i] = true
			placedAny = true
			continue
		}
		if pl, ok := placeInNewStrip(&strips, usable, part, rules, defects); ok {
			placements = append(placements, pl)
			taken[i] = true
			placedAny = true
			continue
		}
		// This piece does not fit the current sheet; a smaller piece later in
		// the list still might, so keep scanning instead of breaking.
	}

	return core.SheetPlan{Placements: placements}, placedAny
}

func placeInStrips(strips *[]strip, usable core.Rect, part core.Part, rules core.Rules, defects []core.Rect) (core.Placement, bool) {
	canRotate := rules.AllowRotate && part.AllowRotate && part.Grain == core.GrainNone
	for si := range *strips {
		st := &(*strips)[si]
		x := usable.X
		if st.used {
			x = st.right + rules.Kerf
		}
		for _, o := range orientations(part, canRotate) {
			if o.h > st.h || x+o.w > usable.Right() {
				continue
			}
			rect := core.Rect{X: x, Y: st.y, W: o.w, H: o.h}
			if hitsDefect(rect, defects) {
				continue
			}
			st.right = x + o.w
			st.used = true
			return newPlacement(part, x, st.y, o), true
		}
	}
	return core.Placement{}, false
}

func placeInNewStrip(strips *[]strip, usable core.Rect, part core.Part, rules core.Rules, defects []core.Rect) (core.Placement, bool) {
	y := usable.Y
	if len(*strips) > 0 {
		last := (*strips)[len(*strips)-1]
		y = last.y + last.h + rules.Kerf
	}
	canRotate := rules.AllowRotate && part.AllowRotate && part.Grain == core.GrainNone
	for _, o := range orientations(part, canRotate) {
		if o.w > usable.W || y+o.h > usable.Bottom() {
			continue
		}
		rect := core.Rect{X: usable.X, Y: y, W: o.w, H: o.h}
		if hitsDefect(rect, defects) {
			continue
		}
		*strips = append(*strips, strip{y: y, h: o.h, right: usable.X + o.w, used: true})
		return newPlacement(part, usable.X, y, o), true
	}
	return core.Placement{}, false
}

func newPlacement(part core.Part, x, y core.Dim, o orientation) core.Placement {
	return core.Placement{
		PartID:   part.ID,
		PartCode: part.Code,
		X:        x,
		Y:        y,
		W:        o.w,
		H:        o.h,
		Rotated:  o.rot,
		Priority: part.Priority,
	}
}

// findOffcuts reconstructs leftovers from the placements: the unused tail of
// every strip and the unused band below the last strip.
func findOffcuts(usable core.Rect, placements []core.Placement, rules core.Rules) []core.Rect {
	if rules.OffcutMinW <= 0 || rules.OffcutMinH <= 0 {
		return nil
	}
	type band struct {
		y, h  core.Dim
		right core.Dim
	}
	bands := map[core.Dim]*band{}
	var ys []core.Dim
	for _, pl := range placements {
		b, ok := bands[pl.Y]
		if !ok {
			b = &band{y: pl.Y, h: pl.H, right: usable.X}
			bands[pl.Y] = b
			ys = append(ys, pl.Y)
		}
		if pl.H > b.h {
			b.h = pl.H
		}
		if pl.X+pl.W > b.right {
			b.right = pl.X + pl.W
		}
	}
	sort.Slice(ys, func(i, j int) bool { return ys[i] < ys[j] })

	var out []core.Rect
	bottom := usable.Y
	for _, y := range ys {
		b := bands[y]
		bottom = b.y + b.h
		rem := core.Rect{X: b.right, Y: b.y, W: usable.Right() - b.right, H: b.h}
		if rem.W >= rules.OffcutMinW && rem.H >= rules.OffcutMinH && rem.W > 0 && rem.H > 0 {
			out = append(out, rem)
		}
	}
	below := core.Rect{
		X: usable.X,
		Y: bottom + rules.Kerf,
		W: usable.W,
		H: usable.Bottom() - (bottom + rules.Kerf),
	}
	if below.W >= rules.OffcutMinW && below.H >= rules.OffcutMinH && below.W > 0 && below.H > 0 {
		out = append(out, below)
	}
	return out
}

// overheads estimates the material lost to edge trim and to the saw kerf. The
// kerf figure is informational: kerf loss is already inside scrap area.
func overheads(sheets []core.SheetPlan, rules core.Rules) (trimM2, kerfM2 float64) {
	for _, sh := range sheets {
		stockArea := core.AreaM2(sh.Width, sh.Height)
		usable := core.Rect{X: rules.Trim, Y: rules.Trim, W: sh.Width - 2*rules.Trim, H: sh.Height - 2*rules.Trim}
		usableArea := 0.0
		if usable.W > 0 && usable.H > 0 {
			usableArea = core.AreaM2(usable.W, usable.H)
		}
		trimM2 += stockArea - usableArea

		// Kerf inside each strip (between pieces) plus one cut between strips.
		bandCount := map[core.Dim]int{}
		for _, pl := range sh.Placements {
			bandCount[pl.Y]++
		}
		for y, n := range bandCount {
			if n > 1 {
				kerfM2 += core.AreaM2(rules.Kerf*(core.Dim(n)-1), bandHeight(sh.Placements, y))
			}
		}
		if len(bandCount) > 1 {
			kerfM2 += core.AreaM2(usable.W, rules.Kerf*(core.Dim(len(bandCount))-1))
		}
	}
	return trimM2, kerfM2
}

func bandHeight(placements []core.Placement, y core.Dim) core.Dim {
	var h core.Dim
	for _, pl := range placements {
		if pl.Y == y && pl.H > h {
			h = pl.H
		}
	}
	return h
}

func hitsDefect(r core.Rect, defects []core.Rect) bool {
	for _, d := range defects {
		if d.W <= 0 || d.H <= 0 {
			continue
		}
		if r.X < d.Right() && d.X < r.Right() && r.Y < d.Bottom() && d.Y < r.Bottom() {
			return true
		}
	}
	return false
}

func expand(parts []core.Part) []instance {
	var out []instance
	for i := range parts {
		for q := 0; q < parts[i].Quantity; q++ {
			out = append(out, instance{part: parts[i], seq: q})
		}
	}
	return out
}

func sortInstances(instances []instance) {
	sort.SliceStable(instances, func(i, j int) bool {
		a, b := instances[i].part, instances[j].part
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		areaA, areaB := a.Width*a.Height, b.Width*b.Height
		if areaA != areaB {
			return areaA > areaB
		}
		if a.Height != b.Height {
			return a.Height > b.Height
		}
		if a.Width != b.Width {
			return a.Width > b.Width
		}
		return a.Code < b.Code
	})
}

func allTaken(taken []bool) bool {
	for _, t := range taken {
		if !t {
			return false
		}
	}
	return true
}

func collectUnplaced(p core.Problem, taken []bool, instances []instance) []core.UnplacedPart {
	type agg struct {
		code  string
		count int
		big   bool
	}
	counts := map[string]*agg{}
	for i := range instances {
		if taken[i] {
			continue
		}
		part := instances[i].part
		a, ok := counts[part.ID]
		if !ok {
			a = &agg{code: part.Code}
			counts[part.ID] = a
		}
		a.count++
		if !fitsAnyStock(p, part) {
			a.big = true
		}
	}
	out := make([]core.UnplacedPart, 0, len(counts))
	for id, a := range counts {
		reason := "no stock or remaining capacity"
		if a.big {
			reason = "part is larger than any usable stock size"
		}
		out = append(out, core.UnplacedPart{PartID: id, PartCode: a.code, Quantity: a.count, Reason: reason})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PartCode < out[j].PartCode })
	return out
}

func fitsAnyStock(p core.Problem, part core.Part) bool {
	for _, st := range p.Stocks {
		uw := st.Width - 2*p.Rules.Trim
		uh := st.Height - 2*p.Rules.Trim
		if uw <= 0 || uh <= 0 {
			continue
		}
		if part.Width <= uw && part.Height <= uh {
			return true
		}
		if p.Rules.AllowRotate && part.AllowRotate && part.Grain == core.GrainNone && part.Height <= uw && part.Width <= uh {
			return true
		}
	}
	return false
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
