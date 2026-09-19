package pack2d

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/cutter"
	"github.com/size-module/backend/internal/optimizer/lp"
)

const columnVersion = "0.1.0"

const (
	cg2MaxRounds   = 40
	cg2MaxColumns  = 400
	cg2PricingTol  = 1e-6
	cg2BudgetShare = 0.4 // share of the time budget spent generating columns
	// cg2MinFill is the fraction of the usable sheet area a pattern must reach
	// before it is worth turning into real sheets. Flooring an LP solution
	// inevitably instantiates some thin patterns (singletons exist to keep the
	// master feasible); cutting those wastes stock that the beam-based residual
	// pass uses far better. Patterns below this bar are left to that pass.
	cg2MinFill = 0.55
)

// ColumnSolver solves 2D guillotine cutting stock problems with staged column
// generation. A pattern is a stack of full-width strips; each strip is filled
// left to right with pieces of one height. That is exactly a two-stage
// guillotine layout (crosscut into strips, then rip each strip), which is how
// panel saws and glass cutters work.
//
// The restricted master LP (minimise the cost of the sheets used) comes from
// the shared simplex; pricing maximises the dual value of one sheet with a
// strip knapsack plus a height knapsack. Like the 1D solver it reports an LP
// lower bound in the notes and falls back to the beam solver when the LP
// machinery cannot be trusted.
type ColumnSolver struct{}

func NewColumn() *ColumnSolver { return &ColumnSolver{} }

func (s *ColumnSolver) Name() string    { return "cg-2d" }
func (s *ColumnSolver) Version() string { return columnVersion }

func (s *ColumnSolver) Capabilities() core.Capabilities {
	return core.Capabilities{
		Dimension:   core.Profile2D,
		CutMode:     core.CutGuillotine,
		Rotation:    true,
		Grain:       true,
		Remnants:    true,
		Rank:        5,
		Description: "Two-stage guillotine column generation (strip pricing + height DP) with an LP lower bound.",
	}
}

// ---------------------------------------------------------------- problem ---

type cg2Type struct {
	ID          string
	Code        string
	W, H        core.Dim
	AllowRotate bool
	Grain       core.GrainMode
	Variants    []int // indexes into the variant list
}

type cg2Variant struct {
	typ  int
	w, h core.Dim
	rot  bool
}

type cg2Format struct {
	ID, Code, Label string
	W, H            core.Dim
	UsableW, UsableH core.Dim
	Cost            float64
	Available       int
}

type cg2Strip struct {
	height core.Dim
	items  []int // variant indexes, left to right
}

type cg2Pattern struct {
	format int
	strips []cg2Strip
	counts []int // aggregated per part type: the master LP column
	cost   float64
	x      float64 // LP value from the last master solve
}

func collectSheetFormats(stocks []core.StockItem, rules core.Rules) []cg2Format {
	var formats []cg2Format
	for _, stock := range stocks {
		if stock.Quantity <= 0 || stock.Width <= 0 || stock.Height <= 0 {
			continue
		}
		usableW := stock.Width - 2*rules.Trim
		usableH := stock.Height - 2*rules.Trim
		if usableW <= 0 || usableH <= 0 {
			continue
		}
		cost := stock.CostPerUnit
		if cost <= 0 {
			cost = 1
		}
		formats = append(formats, cg2Format{
			ID: stock.ID, Code: stock.Code, Label: stock.Label,
			W: stock.Width, H: stock.Height,
			UsableW: usableW, UsableH: usableH,
			Cost: cost, Available: stock.Quantity,
		})
	}
	return formats
}

func collectVariants(parts []core.Part, rules core.Rules) ([]cg2Type, []cg2Variant, []int) {
	index := map[string]int{}
	var types []cg2Type
	var demands []int
	for _, part := range parts {
		if part.Width <= 0 || part.Height <= 0 || part.Quantity <= 0 {
			continue
		}
		if ti, ok := index[part.ID]; ok {
			demands[ti] += part.Quantity
			continue
		}
		index[part.ID] = len(types)
		types = append(types, cg2Type{
			ID: part.ID, Code: part.Code, W: part.Width, H: part.Height,
			AllowRotate: part.AllowRotate, Grain: part.Grain,
		})
		demands = append(demands, part.Quantity)
	}

	var variants []cg2Variant
	for ti := range types {
		types[ti].Variants = append(types[ti].Variants, len(variants))
		variants = append(variants, cg2Variant{typ: ti, w: types[ti].W, h: types[ti].H})
		if types[ti].W != types[ti].H && mayRotate(core.Part{
			Width: types[ti].W, Height: types[ti].H,
			AllowRotate: types[ti].AllowRotate, Grain: types[ti].Grain,
		}, rules) {
			types[ti].Variants = append(types[ti].Variants, len(variants))
			variants = append(variants, cg2Variant{typ: ti, w: types[ti].H, h: types[ti].W, rot: true})
		}
	}
	return types, variants, demands
}

// ------------------------------------------------------------------ solve ---

func (s *ColumnSolver) Solve(ctx context.Context, p core.Problem, progress core.ProgressFunc) (core.Solution, error) {
	start := time.Now()
	rules := normalizeRules(p.Rules)

	types, variants, demands := collectVariants(p.Parts, rules)
	formats := collectSheetFormats(p.Stocks, rules)
	if len(types) == 0 {
		return assembleSolution("cg-2d", columnVersion, p, nil, start, []string{"No 2d parts to cut."}), nil
	}
	if len(formats) == 0 {
		sol := assembleSolution("cg-2d", columnVersion, p, nil, start,
			[]string{"No usable sheet size after trimming."})
		sol.Unplaced = aggregateUnplaced2D(types, demands, "no usable stock size")
		return sol, nil
	}

	// Column generation gets a slice of the budget; the rest goes to local search.
	cgBudget := time.Duration(float64(p.BudgetMS)*cg2BudgetShare) * time.Millisecond
	if cgBudget < 200*time.Millisecond {
		cgBudget = 200 * time.Millisecond
	}
	cgCtx, cancel := context.WithTimeout(ctx, cgBudget)
	pool, lpBound, lpUnmet, rounds, err := s.generate(cgCtx, types, variants, demands, formats, rules)
	cancel()
	if err != nil {
		return s.fallback(ctx, p, progress, start, err)
	}

	sheets, unplaced, notes := s.assemble(ctx, p, pool, lpBound, lpUnmet, rounds, types, variants, demands, formats, rules)
	sol := assembleSolution("cg-2d", columnVersion, p, sheets, start, notes)
	sol.Unplaced = unplaced
	if progress != nil {
		progress(sol)
	}

	improved := Improve(ctx, p, sol, p.Seed, progress)
	if len(improved.Sheets) < len(sol.Sheets) {
		improved.Notes = append(improved.Notes, fmt.Sprintf(
			"Local search removed %d sheet(s) from the column generation plan.", len(sol.Sheets)-len(improved.Sheets)))
	}
	return improved, nil
}

// fallback runs the beam solver and explains why in the notes.
func (s *ColumnSolver) fallback(ctx context.Context, p core.Problem, progress core.ProgressFunc, start time.Time, cause error) (core.Solution, error) {
	sol, err := NewBeam().Solve(ctx, p, progress)
	if err != nil {
		return core.Solution{}, fmt.Errorf("cg-2d failed (%v) and the beam fallback failed too: %w", cause, err)
	}
	sol.Solver = "cg-2d"
	sol.SolverVersion = columnVersion
	sol.Metrics.ElapsedMS = time.Since(start).Milliseconds()
	sol.Notes = append(sol.Notes, fmt.Sprintf("Column generation did not apply (%v); the beam plan is shown.", cause))
	return sol, nil
}

// ------------------------------------------------------------- generation ---

func (s *ColumnSolver) generate(ctx context.Context, types []cg2Type, variants []cg2Variant, demands []int, formats []cg2Format, rules core.Rules) ([]cg2Pattern, float64, float64, int, error) {
	pool := seedPatterns2D(types, variants, demands, formats, rules)
	seen := make(map[string]bool, len(pool))
	for _, pat := range pool {
		seen[patternKey2D(pat, len(types))] = true
	}

		lpBound := 0.0
	lpUnmet := 0.0
	rounds := 0
	for round := 0; round < cg2MaxRounds && len(pool) < cg2MaxColumns; round++ {
		if ctx.Err() != nil {
			break
		}
		rounds++

		A, b, c := masterMatrix2D(pool, types, demands, formats)
		res, err := lp.Solve(A, b, c, lp.Options{})
		if err != nil {
			return nil, 0, 0, rounds, err
		}
		if res.Status != lp.StatusOptimal {
			return nil, 0, 0, rounds, fmt.Errorf("master LP status %s", res.Status)
		}
		if violation := lp.DualViolation(A, c, res.Duals); violation > 1e-6 {
			return nil, 0, 0, rounds, fmt.Errorf("untrustworthy duals (violation %.2e)", violation)
		}
		if gap := math.Abs(res.Objective - res.DualObjective); gap > 1e-6*(1+math.Abs(res.Objective)) {
			return nil, 0, 0, rounds, fmt.Errorf("duality gap %.2e", gap)
		}
		// Split the objective into sheet cost and unmet demand (columns after
		// the patterns), and read the sheet-availability duals for pricing.
		sheetCost := 0.0
		for j := range pool {
			sheetCost += pool[j].cost * res.X[j]
		}
		unmet := 0.0
		for i := 0; i < len(types); i++ {
			unmet += res.X[len(pool)+i]
		}
		lpBound = sheetCost
		lpUnmet = unmet
		for i := range pool {
			pool[i].x = res.X[i]
		}
		availabilityDuals := make([]float64, len(formats))
		for f := range formats {
			availabilityDuals[f] = res.Duals[len(types)+f]
		}

		added := 0
		for fi := range formats {
			pat, value, ok := pricePattern2D(res.Duals, types, variants, demands, formats[fi], fi, rules)
			if !ok || value-availabilityDuals[fi]-formats[fi].Cost <= cg2PricingTol {
				continue
			}
			key := patternKey2D(pat, len(types))
			if seen[key] {
				continue
			}
			seen[key] = true
			pool = append(pool, pat)
			added++
		}
		if added == 0 {
			break
		}
	}
	return pool, lpBound, lpUnmet, rounds, nil
}

// cg2UnmetPenalty prices one unserved piece in the master LP, so the LP serves
// as much demand as the stock allows before it starts minimising sheet cost.
const cg2UnmetPenalty = 1000.0

// masterMatrix2D builds the restricted master in equality form with three
// column groups: patterns, unserved demand (penalised) and unused sheet
// availability. The last two keep the LP feasible and meaningful for
// stock-limited orders — a plain covering LP would be infeasible and would let
// the rounding plan more sheets than exist.
func masterMatrix2D(pool []cg2Pattern, types []cg2Type, demands []int, formats []cg2Format) ([][]float64, []float64, []float64) {
	rows := len(types) + len(formats)
	cols := len(pool) + len(types) + len(formats)
	A := make([][]float64, rows)
	for i := range A {
		A[i] = make([]float64, cols)
	}
	for j, pat := range pool {
		for i, count := range pat.counts {
			A[i][j] = float64(count)
		}
		if pat.format >= 0 && pat.format < len(formats) {
			A[len(types)+pat.format][j] = 1
		}
	}
	for i := 0; i < len(types); i++ {
		A[i][len(pool)+i] = 1 // unserved pieces
	}

	b := make([]float64, rows)
	for i := range demands {
		b[i] = float64(demands[i])
	}
	for f := range formats {
		b[len(types)+f] = float64(formats[f].Available)
	}

	c := make([]float64, cols)
	for j, pat := range pool {
		c[j] = pat.cost
	}
	for i := 0; i < len(types); i++ {
		c[len(pool)+i] = cg2UnmetPenalty
	}
	for f := 0; f < len(formats); f++ {
		A[len(types)+f][len(pool)+len(types)+f] = 1 // unused availability, free
	}
	return A, b, c
}

// seedPatterns2D gives the master a feasible start: one singleton per type and
// format plus a greedy strip pattern per format.
func seedPatterns2D(types []cg2Type, variants []cg2Variant, demands []int, formats []cg2Format, rules core.Rules) []cg2Pattern {
	var out []cg2Pattern
	seen := map[string]bool{}
	add := func(fi int, strips []cg2Strip) {
		pat, ok := finalisePattern(strips, types, variants, demands, formats[fi], fi, rules)
		if !ok {
			return
		}
		key := patternKey2D(pat, len(types))
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, pat)
	}

	for ti := range types {
		// Singleton patterns make the master feasible even when a type only
		// fits a specific format.
		for vi := range types[ti].Variants {
			variant := types[ti].Variants[vi]
			for fi := range formats {
				add(fi, []cg2Strip{{height: variants[variant].h, items: []int{variant}}})
			}
		}
	}

	// Greedy strip pattern per format: pick, for every piece height, the strip
	// that packs the most area, then stack strips while height remains.
	for fi := range formats {
		add(fi, greedyStrips(types, variants, demands, formats[fi], rules))
	}
	return out
}

func greedyStrips(types []cg2Type, variants []cg2Variant, demands []int, format cg2Format, rules core.Rules) []cg2Strip {
	byHeight := map[core.Dim][]int{}
	var heights []core.Dim
	for vi := range variants {
		h := variants[vi].h
		if _, ok := byHeight[h]; !ok {
			heights = append(heights, h)
		}
		byHeight[h] = append(byHeight[h], vi)
	}
	sort.Slice(heights, func(i, j int) bool { return heights[i] < heights[j] })

	totalH := rules.Trim
	var strips []cg2Strip
	for _, h := range heights {
		if h > format.UsableH {
			continue
		}
		items := fillStripByArea(byHeight[h], demands, types, variants, format.UsableW, rules.Kerf)
		if len(items) == 0 {
			continue
		}
		if totalH+h > rules.Trim+format.UsableH {
			break
		}
		strips = append(strips, cg2Strip{height: h, items: items})
		totalH += h + rules.Kerf
	}
	return strips
}

// fillStripByArea is a small 1D knapsack that maximises packed area for one
// strip height.
func fillStripByArea(candidates []int, demands []int, types []cg2Type, variants []cg2Variant, usableW core.Dim, kerf core.Dim) []int {
	capacityMM := int((usableW + kerf) / core.Millimeter)
	if capacityMM <= 0 {
		return nil
	}
	type item struct {
		vi    int
		lenMM int
		value float64
	}
	var items []item
	for _, vi := range candidates {
		v := variants[vi]
		l := int((v.w + kerf + core.Millimeter - 1) / core.Millimeter)
		if l > capacityMM {
			continue
		}
		items = append(items, item{vi: vi, lenMM: l, value: float64(v.w) * float64(v.h)})
	}
	if len(items) == 0 {
		return nil
	}
	best := make([]float64, capacityMM+1)
	choice := make([]int, capacityMM+1)
	for i := range choice {
		choice[i] = -1
	}
	for c := 1; c <= capacityMM; c++ {
		best[c] = best[c-1]
		for k := range items {
			if items[k].lenMM <= c {
				if v := best[c-items[k].lenMM] + items[k].value; v > best[c]+1e-9 {
					best[c] = v
					choice[c] = k
				}
			}
		}
	}
	var picked []int
	for c := capacityMM; c > 0; {
		k := choice[c]
		if k < 0 {
			c--
			continue
		}
		picked = append(picked, items[k].vi)
		c -= items[k].lenMM
	}
	sort.Ints(picked)
	return clampStripToDemand(picked, types, variants, demands, usableW, kerf)
}

// pricePattern2D maximises the dual value of one sheet: for every piece height
// a strip knapsack, then a height knapsack over those strips.
func pricePattern2D(duals []float64, types []cg2Type, variants []cg2Variant, demands []int, format cg2Format, formatIndex int, rules core.Rules) (cg2Pattern, float64, bool) {
	byHeight := map[core.Dim][]int{}
	var heights []core.Dim
	for vi := range variants {
		h := variants[vi].h
		if _, ok := byHeight[h]; !ok {
			heights = append(heights, h)
		}
		byHeight[h] = append(byHeight[h], vi)
	}
	sort.Slice(heights, func(i, j int) bool { return heights[i] < heights[j] })

	type stripOption struct {
		height core.Dim
		items  []int
		value  float64
	}
	var options []stripOption
	for _, h := range heights {
		if h > format.UsableH {
			continue
		}
		items, value := bestStrip(byHeight[h], duals, types, variants, demands, format.UsableW, rules.Kerf)
		if len(items) == 0 || value <= 0 {
			continue
		}
		options = append(options, stripOption{height: h, items: items, value: value})
	}
	if len(options) == 0 {
		return cg2Pattern{}, 0, false
	}

	// Height knapsack: each strip consumes its height plus a kerf.
	capacityMM := int((format.UsableH + rules.Kerf) / core.Millimeter)
	if capacityMM <= 0 {
		return cg2Pattern{}, 0, false
	}
	type item struct {
		opt   int
		lenMM int
		value float64
	}
	var items []item
	for oi := range options {
		l := int((options[oi].height + rules.Kerf + core.Millimeter - 1) / core.Millimeter)
		if l > capacityMM {
			continue
		}
		items = append(items, item{opt: oi, lenMM: l, value: options[oi].value})
	}
	if len(items) == 0 {
		return cg2Pattern{}, 0, false
	}
	best := make([]float64, capacityMM+1)
	choice := make([]int, capacityMM+1)
	for i := range choice {
		choice[i] = -1
	}
	for c := 1; c <= capacityMM; c++ {
		best[c] = best[c-1]
		for k := range items {
			if items[k].lenMM <= c {
				if v := best[c-items[k].lenMM] + items[k].value; v > best[c]+1e-12 {
					best[c] = v
					choice[c] = k
				}
			}
		}
	}

	var strips []cg2Strip
	for c := capacityMM; c > 0; {
		k := choice[c]
		if k < 0 {
			c--
			continue
		}
		opt := options[items[k].opt]
		strips = append(strips, cg2Strip{height: opt.height, items: opt.items})
		c -= items[k].lenMM
	}
	if len(strips) == 0 {
		return cg2Pattern{}, 0, false
	}
	pat, ok := finalisePattern(strips, types, variants, demands, format, formatIndex, rules)
	if !ok {
		return cg2Pattern{}, 0, false
	}
	value := 0.0
	for ti, count := range pat.counts {
		value += float64(count) * duals[ti]
	}
	return pat, value, true
}

// bestStrip solves the strip knapsack for one piece height.
func bestStrip(candidates []int, duals []float64, types []cg2Type, variants []cg2Variant, demands []int, usableW core.Dim, kerf core.Dim) ([]int, float64) {
	capacityMM := int((usableW + kerf) / core.Millimeter)
	if capacityMM <= 0 {
		return nil, 0
	}
	type item struct {
		vi    int
		lenMM int
		value float64
	}
	var items []item
	for _, vi := range candidates {
		v := variants[vi]
		if duals[v.typ] <= 0 {
			continue
		}
		l := int((v.w + kerf + core.Millimeter - 1) / core.Millimeter)
		if l > capacityMM {
			continue
		}
		items = append(items, item{vi: vi, lenMM: l, value: duals[v.typ]})
	}
	if len(items) == 0 {
		return nil, 0
	}
	best := make([]float64, capacityMM+1)
	choice := make([]int, capacityMM+1)
	for i := range choice {
		choice[i] = -1
	}
	for c := 1; c <= capacityMM; c++ {
		best[c] = best[c-1]
		for k := range items {
			if items[k].lenMM <= c {
				if v := best[c-items[k].lenMM] + items[k].value; v > best[c]+1e-12 {
					best[c] = v
					choice[c] = k
				}
			}
		}
	}
	var picked []int
	for c := capacityMM; c > 0; {
		k := choice[c]
		if k < 0 {
			c--
			continue
		}
		picked = append(picked, items[k].vi)
		c -= items[k].lenMM
	}
	sort.Ints(picked)
	picked = clampStripToDemand(picked, types, variants, demands, usableW, kerf)
	value := 0.0
	for _, vi := range picked {
		value += duals[variants[vi].typ]
	}
	return picked, value
}

// clampStripToDemand enforces the demand per part type and the exact strip
// width, dropping pieces until both hold.
func clampStripToDemand(picked []int, types []cg2Type, variants []cg2Variant, demands []int, usableW, kerf core.Dim) []int {
	used := make([]int, len(types))
	out := make([]int, 0, len(picked))
	for _, vi := range picked {
		ti := variants[vi].typ
		if used[ti] >= demands[ti] {
			continue
		}
		used[ti]++
		out = append(out, vi)
	}
	for stripWidth(out, variants, kerf) > usableW && len(out) > 0 {
		// Drop the widest piece (deterministic tie-break by index).
		worst := 0
		for i := range out {
			if variants[out[i]].w > variants[out[worst]].w {
				worst = i
			}
		}
		out = append(out[:worst], out[worst+1:]...)
	}
	return out
}

func stripWidth(items []int, variants []cg2Variant, kerf core.Dim) core.Dim {
	total := core.Dim(0)
	for i, vi := range items {
		if i > 0 {
			total += kerf
		}
		total += variants[vi].w
	}
	return total
}

// finalisePattern aggregates a strip list into a master column, clamps it to
// demand and verifies that it really lays out inside the sheet, dropping the
// last strip until it does.
func finalisePattern(strips []cg2Strip, types []cg2Type, variants []cg2Variant, demands []int, format cg2Format, formatIndex int, rules core.Rules) (cg2Pattern, bool) {
	for candidate := len(strips); candidate > 0; candidate-- {
		pat := cg2Pattern{format: formatIndex, cost: format.Cost}
		counts := make([]int, len(types))
		for _, strip := range strips[:candidate] {
			items := clampStripToDemandLocal(strip.items, counts, demands, variants, format.UsableW, rules.Kerf)
			if len(items) > 0 {
				pat.strips = append(pat.strips, cg2Strip{height: strip.height, items: items})
			}
		}
		if len(pat.strips) == 0 {
			continue
		}
		if _, _, ok := layoutPattern(pat, format, rules, types, variants); !ok {
			continue
		}
		pat.counts = counts
		return pat, true
	}
	return cg2Pattern{}, false
}

// clampStripToDemandLocal is clampStripToDemand against a running type count.
func clampStripToDemandLocal(picked []int, counts []int, demands []int, variants []cg2Variant, usableW, kerf core.Dim) []int {
	out := make([]int, 0, len(picked))
	used := make([]int, len(counts))
	for _, vi := range picked {
		ti := variants[vi].typ
		if counts[ti]+used[ti] >= demands[ti] {
			continue
		}
		used[ti]++
		out = append(out, vi)
	}
	for stripWidth(out, variants, kerf) > usableW && len(out) > 0 {
		worst := 0
		for i := range out {
			if variants[out[i]].w > variants[out[worst]].w {
				worst = i
			}
		}
		used[variants[out[worst]].typ]--
		out = append(out[:worst], out[worst+1:]...)
	}
	for ti, n := range used {
		counts[ti] += n
	}
	return out
}

func patternKey2D(pat cg2Pattern, typeCount int) string {
	var sb strings.Builder
	sb.WriteString(itoa(pat.format))
	sb.WriteString("|")
	for i := 0; i < typeCount; i++ {
		count := 0
		if i < len(pat.counts) {
			count = pat.counts[i]
		}
		sb.WriteString(itoa(count))
		sb.WriteString(",")
	}
	return sb.String()
}

// ---------------------------------------------------------------- assembly ---

func (s *ColumnSolver) assemble(
	ctx context.Context,
	p core.Problem,
	pool []cg2Pattern,
	lpBound float64,
	lpUnmet float64,
	rounds int,
	types []cg2Type,
	variants []cg2Variant,
	demands []int,
	formats []cg2Format,
	rules core.Rules,
) ([]core.SheetPlan, []core.UnplacedPart, []string) {
	remaining := append([]int(nil), demands...)
	avail := make([]int, len(formats))
	for i := range formats {
		avail[i] = formats[i].Available
	}

	ordered := append([]cg2Pattern(nil), pool...)
	sort.SliceStable(ordered, func(i, j int) bool {
		fillI, fillJ := patternFill(ordered[i], formats, variants), patternFill(ordered[j], formats, variants)
		if fillI != fillJ {
			return fillI > fillJ
		}
		if ordered[i].x != ordered[j].x {
			return ordered[i].x > ordered[j].x
		}
		return patternKey2D(ordered[i], len(types)) < patternKey2D(ordered[j], len(types))
	})

	var sheets []core.SheetPlan
	usedPatterns := 0
	skippedThin := 0
	for _, pat := range ordered {
		if pat.x <= 0 {
			continue
		}
		if patternFill(pat, formats, variants) < cg2MinFill {
			skippedThin++
			continue
		}
		n := int(math.Floor(pat.x + 1e-9))
		if n <= 0 {
			continue
		}
		if n > avail[pat.format] {
			n = avail[pat.format]
		}
		for ti, count := range pat.counts {
			if count > 0 {
				if q := remaining[ti] / count; q < n {
					n = q
				}
			}
		}
		for k := 0; k < n; k++ {
			placements, offcuts, ok := layoutPattern(pat, formats[pat.format], rules, types, variants)
			if !ok {
				break
			}
			sheet := core.SheetPlan{
				Index:      len(sheets),
				StockID:    formats[pat.format].ID,
				StockCode:  formats[pat.format].Code,
				Label:      formats[pat.format].Label,
				Width:      formats[pat.format].W,
				Height:     formats[pat.format].H,
				Placements: placements,
				Offcuts:    offcuts,
			}
			if sheet.Label == "" {
				sheet.Label = sheet.StockCode
			}
			sheets = append(sheets, sheet)
			avail[pat.format]--
			usedPatterns++
			for ti, count := range pat.counts {
				remaining[ti] -= count
			}
		}
	}

	totalDemand := 0
	for _, d := range demands {
		totalDemand += d
	}
	notes := []string{
		fmt.Sprintf("Column generation ran %d master LP round(s) and generated %d two-stage pattern(s); %d were used for %d sheet(s).",
			rounds, len(pool), usedPatterns, len(sheets)),
		fmt.Sprintf("LP view: %.1f of %d pieces can be served with the available stock (%.1f unmet) at a sheet cost of %.2f.",
			float64(totalDemand)-lpUnmet, totalDemand, lpUnmet, lpBound),
	}
	if skippedThin > 0 {
		notes = append(notes, fmt.Sprintf(
			"%d thin pattern(s) below %.0f%% sheet utilisation were left to the residual pass.", skippedThin, cg2MinFill*100))
	}

	var unplaced []core.UnplacedPart
	residual := make([]core.Part, 0, len(types))
	for ti := range types {
		if remaining[ti] > 0 {
			residual = append(residual, core.Part{
				ID: types[ti].ID, Code: types[ti].Code,
				Width: types[ti].W, Height: types[ti].H,
				Quantity: remaining[ti],
				Grain:    types[ti].Grain, AllowRotate: types[ti].AllowRotate,
			})
		}
	}
	if len(residual) > 0 {
		var stocks []core.StockItem
		for fi := range formats {
			if avail[fi] <= 0 {
				continue
			}
			stocks = append(stocks, core.StockItem{
				ID: formats[fi].ID, Code: formats[fi].Code, Label: formats[fi].Label,
				Width: formats[fi].W, Height: formats[fi].H, Quantity: avail[fi],
			})
		}
		if len(stocks) == 0 {
			unplaced = append(unplaced, aggregateUnplaced2D(types, remaining, "no stock left")...)
			notes = append(notes, "Stock ran out before the residual demand could be placed.")
		} else {
			sub := core.Normalize(core.Problem{Parts: residual, Stocks: stocks, Rules: p.Rules})
			subSol, err := NewBeam().Solve(ctx, sub, nil)
			if err == nil {
				for _, sheet := range subSol.Sheets {
					sheet.Index = len(sheets)
					sheets = append(sheets, sheet)
				}
				unplaced = append(unplaced, subSol.Unplaced...)
				if len(subSol.Sheets) > 0 {
					notes = append(notes, fmt.Sprintf(
						"The residual demand was placed by the beam solver on %d sheet(s).", len(subSol.Sheets)))
				}
			} else {
				unplaced = append(unplaced, aggregateUnplaced2D(types, remaining, "placement failed")...)
			}
		}
	}

	// Cut instructions for the pattern sheets (the beam adds its own).
	for i := range sheets {
		if len(sheets[i].CutSteps) == 0 {
			if steps, ok := cutter.ForSheetStages(sheets[i], rules.Kerf, rules.MaxCutStages); ok {
				sheets[i].CutSteps = steps
			}
		}
	}
	return sheets, unplaced, notes
}

// layoutPattern turns a two-stage pattern into placements and offcuts.
func layoutPattern(pat cg2Pattern, format cg2Format, rules core.Rules, types []cg2Type, variants []cg2Variant) ([]core.Placement, []core.Rect, bool) {
	var placements []core.Placement
	var offcuts []core.Rect

	y := rules.Trim
	bottom := rules.Trim + format.UsableH
	right := rules.Trim + format.UsableW

	for _, strip := range pat.strips {
		if y+strip.height > bottom {
			return nil, nil, false
		}
		x := rules.Trim
		for i, vi := range strip.items {
			if i > 0 {
				x += rules.Kerf
			}
			v := variants[vi]
			if x+v.w > right {
				return nil, nil, false
			}
			partType := types[v.typ]
			placements = append(placements, core.Placement{
				PartID:   partType.ID,
				PartCode: partType.Code,
				X:        x,
				Y:        y,
				W:        v.w,
				H:        v.h,
				Rotated:  v.rot,
			})
			x += v.w
		}
		// Reusable remainder of the strip.
		if rules.OffcutMinW > 0 && rules.OffcutMinH > 0 {
			if rem := right - (x + rules.Kerf); rem >= rules.OffcutMinW && strip.height >= rules.OffcutMinH {
				offcuts = append(offcuts, core.Rect{X: x + rules.Kerf, Y: y, W: rem, H: strip.height})
			}
		}
		y += strip.height + rules.Kerf
	}
	// Reusable band below the last strip.
	if rules.OffcutMinW > 0 && rules.OffcutMinH > 0 {
		if rem := bottom - y; rem >= rules.OffcutMinH && format.UsableW >= rules.OffcutMinW {
			offcuts = append(offcuts, core.Rect{X: rules.Trim, Y: y, W: format.UsableW, H: rem})
		}
	}
	return placements, offcuts, true
}

// patternFill is the fraction of the usable sheet area a pattern covers.
func patternFill(pat cg2Pattern, formats []cg2Format, variants []cg2Variant) float64 {
	if pat.format < 0 || pat.format >= len(formats) {
		return 0
	}
	format := formats[pat.format]
	usable := float64(format.UsableW) * float64(format.UsableH)
	if usable <= 0 {
		return 0
	}
	area := 0.0
	for _, strip := range pat.strips {
		for _, vi := range strip.items {
			area += float64(variants[vi].w) * float64(variants[vi].h)
		}
	}
	return area / usable
}

func aggregateUnplaced2D(types []cg2Type, demands []int, reason string) []core.UnplacedPart {
	var out []core.UnplacedPart
	for ti := range types {
		if demands[ti] <= 0 {
			continue
		}
		out = append(out, core.UnplacedPart{
			PartID: types[ti].ID, PartCode: types[ti].Code,
			Quantity: demands[ti], Reason: reason,
		})
	}
	return out
}
