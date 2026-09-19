package pack1d

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/cutter"
	"github.com/size-module/backend/internal/optimizer/lp"
)

const cgVersion = "0.1.0"

const (
	cgMaxRounds  = 200
	cgMaxColumns = 600
	// cgPricingTol is the minimum positive reduced cost that justifies adding a
	// column. Below it the LP is considered converged.
	cgPricingTol = 1e-6
)

// ColumnSolver solves 1D cutting stock problems with Gilmore–Gomory column
// generation: the restricted master LP (minimise the cost of the patterns used)
// is solved with a small dense simplex, and new patterns are priced with an
// exact DP knapsack over the remaining dual values. Patterns are then rounded
// to integers, the residual demand is handed to the FFD baseline and the result
// is compacted.
//
// It is strongest on homogeneous orders — many pieces of few lengths — where
// constructive heuristics leave material on the table. When anything about the
// LP goes wrong it falls back to the baseline solver rather than returning a
// bad plan.
type ColumnSolver struct{}

func NewColumn() *ColumnSolver { return &ColumnSolver{} }

func (s *ColumnSolver) Name() string    { return "cg-1d" }
func (s *ColumnSolver) Version() string { return cgVersion }

func (s *ColumnSolver) Capabilities() core.Capabilities {
	return core.Capabilities{
		Dimension:   core.Profile1D,
		CutMode:     core.CutGuillotine,
		Rank:        5,
		Description: "Gilmore-Gomory column generation with DP pricing. Strongest on homogeneous orders.",
	}
}

// ---------------------------------------------------------------- problem ---

type cgType struct {
	ID     string
	Code   string
	Length core.Dim
}

type cgFormat struct {
	ID        string
	Code      string
	Label     string
	Length    core.Dim
	Width     core.Dim
	Usable    core.Dim
	Cost      float64
	Available int
}

type cgPattern struct {
	format int
	counts []int
	length core.Dim
	cost   float64
	// x is the LP value of this pattern in the last master solution.
	x float64
}

// barState is one physical bar: a format plus the pieces assigned to it.
type barState struct {
	format cgFormat
	pieces []core.Part
}

func collectTypes(parts []core.Part) ([]cgType, []int) {
	index := map[string]int{}
	var types []cgType
	var demands []int
	for _, part := range parts {
		if part.Length <= 0 || part.Quantity <= 0 {
			continue
		}
		if i, ok := index[part.ID]; ok {
			demands[i] += part.Quantity
			continue
		}
		index[part.ID] = len(types)
		types = append(types, cgType{ID: part.ID, Code: part.Code, Length: part.Length})
		demands = append(demands, part.Quantity)
	}
	return types, demands
}

func collectFormats(stocks []core.StockItem, rules core.Rules) []cgFormat {
	var formats []cgFormat
	for _, stock := range stocks {
		if stock.Length <= 0 || stock.Quantity <= 0 {
			continue
		}
		usable := stock.Length - 2*rules.Trim
		if usable <= 0 {
			continue
		}
		cost := stock.CostPerUnit
		if cost <= 0 {
			// A missing price means "one bar"; without it the LP would treat
			// the format as free and use it without limit.
			cost = 1
		}
		width := stock.Width
		if width <= 0 {
			width = core.Millimeter
		}
		formats = append(formats, cgFormat{
			ID:        stock.ID,
			Code:      stock.Code,
			Label:     stock.Label,
			Length:    stock.Length,
			Width:     width,
			Usable:    usable,
			Cost:      cost,
			Available: stock.Quantity,
		})
	}
	return formats
}

// ------------------------------------------------------------------ solve ---

func (s *ColumnSolver) Solve(ctx context.Context, p core.Problem, progress core.ProgressFunc) (core.Solution, error) {
	start := time.Now()
	rules := p.Rules
	if rules.Kerf < 0 {
		rules.Kerf = 0
	}
	if rules.Trim < 0 {
		rules.Trim = 0
	}

	types, demands := collectTypes(p.Parts)
	formats := collectFormats(p.Stocks, rules)
	if len(types) == 0 {
		return assembleSolution(p, nil, nil, start, []string{"No 1d parts with positive length; nothing to cut."}), nil
	}
	if len(formats) == 0 {
		unplaced := aggregateUnplaced(types, demands, "no usable stock length")
		return assembleSolution(p, nil, unplaced, start, []string{"No usable stock length."}), nil
	}

	pool, lpBound, rounds, err := s.generate(ctx, types, demands, formats, rules)
	if err != nil {
		return s.fallback(ctx, p, progress, start, err)
	}

	bars, unplaced, notes := s.assemble(ctx, p, pool, lpBound, rounds, types, demands, formats, rules)
	sheets := make([]core.SheetPlan, 0, len(bars))
	for i, bar := range bars {
		sheets = append(sheets, buildBarSheet(bar, i, rules))
	}

	sol := assembleSolution(p, sheets, unplaced, start, notes)
	sol.Metrics.StockLengthM, sol.Metrics.UsedLengthM = lengthTotals(sheets)
	if progress != nil {
		progress(sol)
	}
	return sol, nil
}

// fallback runs the FFD baseline and explains why in the notes.
func (s *ColumnSolver) fallback(ctx context.Context, p core.Problem, progress core.ProgressFunc, start time.Time, cause error) (core.Solution, error) {
	sol, err := New().Solve(ctx, p, progress)
	if err != nil {
		return core.Solution{}, fmt.Errorf("cg-1d failed (%v) and the ffd fallback failed too: %w", cause, err)
	}
	sol.Solver = "cg-1d"
	sol.SolverVersion = cgVersion
	sol.Metrics.ElapsedMS = time.Since(start).Milliseconds()
	sol.Notes = append(sol.Notes, fmt.Sprintf("Column generation did not apply (%v); the FFD baseline plan is shown.", cause))
	return sol, nil
}

// ------------------------------------------------------------- generation ---

func (s *ColumnSolver) generate(ctx context.Context, types []cgType, demands []int, formats []cgFormat, rules core.Rules) ([]cgPattern, float64, int, error) {
	pool := seedPatterns(types, demands, formats, rules.Kerf)
	seen := make(map[string]bool, len(pool))
	for _, pat := range pool {
		seen[patternKey(pat, len(types))] = true
	}

	lpBound := 0.0
	rounds := 0
	for round := 0; round < cgMaxRounds && len(pool) < cgMaxColumns; round++ {
		if err := ctx.Err(); err != nil {
			break
		}
		rounds++

		A, b, c := masterMatrix(pool, types, demands, formats)
		res, err := lp.Solve(A, b, c, lp.Options{})
		if err != nil {
			return nil, 0, rounds, err
		}
		if res.Status != lp.StatusOptimal {
			return nil, 0, rounds, fmt.Errorf("master LP status %s", res.Status)
		}
		if violation := lp.DualViolation(A, c, res.Duals); violation > 1e-6 {
			return nil, 0, rounds, fmt.Errorf("untrustworthy duals (violation %.2e)", violation)
		}
		if gap := math.Abs(res.Objective - res.DualObjective); gap > 1e-6*(1+math.Abs(res.Objective)) {
			return nil, 0, rounds, fmt.Errorf("duality gap %.2e", gap)
		}
		lpBound = res.Objective
		for i := range pool {
			pool[i].x = res.X[i]
		}

		added := 0
		for fi := range formats {
			counts, value, ok := pricePattern(res.Duals, types, demands, formats[fi], rules.Kerf)
			if !ok {
				continue
			}
			if value-formats[fi].Cost <= cgPricingTol {
				continue
			}
			pat := cgPattern{
				format: fi,
				counts: counts,
				length: patternLength(counts, types, rules.Kerf),
				cost:   formats[fi].Cost,
			}
			key := patternKey(pat, len(types))
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
	return pool, lpBound, rounds, nil
}

// masterMatrix builds the restricted master in equality form: pattern columns
// followed by one surplus column per part type.
func masterMatrix(pool []cgPattern, types []cgType, demands []int, formats []cgFormat) ([][]float64, []float64, []float64) {
	m := len(types)
	cols := len(pool) + m
	A := make([][]float64, m)
	for i := range A {
		A[i] = make([]float64, cols)
	}
	for j, pat := range pool {
		for i, count := range pat.counts {
			A[i][j] = float64(count)
		}
	}
	for i := 0; i < m; i++ {
		A[i][len(pool)+i] = -1 // surplus: demand may be exceeded, never unmet
	}
	c := make([]float64, cols)
	for j, pat := range pool {
		c[j] = pat.cost
	}
	b := make([]float64, m)
	for i := range demands {
		b[i] = float64(demands[i])
	}
	return A, b, c
}

// seedPatterns gives the master a feasible start: one singleton pattern per
// type and format, plus a greedy fill of every format.
func seedPatterns(types []cgType, demands []int, formats []cgFormat, kerf core.Dim) []cgPattern {
	var out []cgPattern
	seen := map[string]bool{}
	add := func(format int, counts []int) {
		pat := cgPattern{
			format: format,
			counts: counts,
			length: patternLength(counts, types, kerf),
			cost:   formats[format].Cost,
		}
		key := patternKey(pat, len(types))
		if seen[key] || pat.length <= 0 || pat.length > formats[format].Usable {
			return
		}
		seen[key] = true
		out = append(out, pat)
	}

	for i := range types {
		single := make([]int, len(types))
		single[i] = 1
		for fi := range formats {
			add(fi, single)
		}
	}

	// Greedy best-fill per format, largest pieces first.
	order := make([]int, len(types))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		if types[order[a]].Length != types[order[b]].Length {
			return types[order[a]].Length > types[order[b]].Length
		}
		return types[order[a]].Code < types[order[b]].Code
	})
	for fi := range formats {
		counts := make([]int, len(types))
		used := core.Dim(0)
		pieces := 0
		for _, i := range order {
			for counts[i] < demands[i] {
				need := types[i].Length
				if pieces > 0 {
					need += kerf
				}
				if used+need > formats[fi].Usable {
					break
				}
				used += need
				pieces++
				counts[i]++
			}
		}
		add(fi, counts)
	}
	return out
}

// pricePattern solves the pricing knapsack for one format: maximise the dual
// value of a pattern that fits. Lengths are rounded conservatively (items up,
// capacity down) so every pattern the DP proposes is feasible in exact
// micrometers; a final exact check trims anything that still does not fit.
func pricePattern(duals []float64, types []cgType, demands []int, format cgFormat, kerf core.Dim) ([]int, float64, bool) {
	capacity := int((format.Usable + kerf) / core.Millimeter) // floor
	if capacity <= 0 {
		return nil, 0, false
	}

	type dpItem struct {
		typ   int
		lenMM int
		value float64
	}
	var items []dpItem
	for i := range types {
		if demands[i] <= 0 || types[i].Length <= 0 || duals[i] <= 0 {
			continue
		}
		lengthMM := int((types[i].Length + kerf + core.Millimeter - 1) / core.Millimeter) // ceil
		if lengthMM > capacity {
			continue
		}
		items = append(items, dpItem{typ: i, lenMM: lengthMM, value: duals[i]})
	}
	if len(items) == 0 {
		return nil, 0, false
	}

	best := make([]float64, capacity+1)
	choice := make([]int, capacity+1)
	for c := range choice {
		choice[c] = -1
	}
	for c := 1; c <= capacity; c++ {
		best[c] = best[c-1] // leaving capacity unused is allowed
		for k := range items {
			if items[k].lenMM <= c {
				if v := best[c-items[k].lenMM] + items[k].value; v > best[c]+1e-12 {
					best[c] = v
					choice[c] = k
				}
			}
		}
	}

	counts := make([]int, len(types))
	for c := capacity; c > 0; {
		k := choice[c]
		if k < 0 {
			c--
			continue
		}
		counts[items[k].typ]++
		c -= items[k].lenMM
	}

	total := 0
	for i, count := range counts {
		if count > demands[i] {
			counts[i] = demands[i]
		}
		total += counts[i]
	}
	if total == 0 {
		return nil, 0, false
	}
	counts = trimToFit(counts, types, format.Usable, kerf)

	value := 0.0
	for i, count := range counts {
		value += float64(count) * duals[i]
	}
	return counts, value, true
}

// trimToFit drops the shortest pieces until the pattern fits exactly.
func trimToFit(counts []int, types []cgType, usable, kerf core.Dim) []int {
	for patternLength(counts, types, kerf) > usable {
		shortest := -1
		for i, count := range counts {
			if count <= 0 {
				continue
			}
			if shortest < 0 || types[i].Length < types[shortest].Length {
				shortest = i
			}
		}
		if shortest < 0 {
			break
		}
		counts[shortest]--
	}
	return counts
}

func patternLength(counts []int, types []cgType, kerf core.Dim) core.Dim {
	total := core.Dim(0)
	pieces := 0
	for i, count := range counts {
		if count <= 0 {
			continue
		}
		total += core.Dim(count) * types[i].Length
		pieces += count
	}
	if pieces > 1 {
		total += core.Dim(pieces-1) * kerf
	}
	return total
}

func patternKey(pat cgPattern, typeCount int) string {
	var sb strings.Builder
	sb.WriteString(strconv.Itoa(pat.format))
	sb.WriteString("|")
	for i := 0; i < typeCount; i++ {
		var count int
		if i < len(pat.counts) {
			count = pat.counts[i]
		}
		sb.WriteString(strconv.Itoa(count))
		sb.WriteString(",")
	}
	return sb.String()
}

// ---------------------------------------------------------------- assembly ---

// assemble turns the LP pool into an integer plan: floors the LP values, fills
// the residual demand with the baseline solver and compacts the result.
func (s *ColumnSolver) assemble(
	ctx context.Context,
	p core.Problem,
	pool []cgPattern,
	lpBound float64,
	rounds int,
	types []cgType,
	demands []int,
	formats []cgFormat,
	rules core.Rules,
) ([]barState, []core.UnplacedPart, []string) {
	remaining := append([]int(nil), demands...)
	avail := make([]int, len(formats))
	for i := range formats {
		avail[i] = formats[i].Available
	}

	ordered := append([]cgPattern(nil), pool...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].x != ordered[j].x {
			return ordered[i].x > ordered[j].x
		}
		if ordered[i].length != ordered[j].length {
			return ordered[i].length > ordered[j].length
		}
		return patternKey(ordered[i], len(types)) < patternKey(ordered[j], len(types))
	})

	var bars []barState
	usedPatterns := 0
	for _, pat := range ordered {
		if pat.x <= 0 {
			continue
		}
		n := int(math.Floor(pat.x + 1e-9))
		if n <= 0 {
			continue
		}
		if n > avail[pat.format] {
			n = avail[pat.format]
		}
		for i, count := range pat.counts {
			if count > 0 {
				if q := remaining[i] / count; q < n {
					n = q
				}
			}
		}
		for k := 0; k < n; k++ {
			bars = append(bars, barFromPattern(pat, types, formats))
			avail[pat.format]--
			usedPatterns++
			for i, count := range pat.counts {
				remaining[i] -= count
			}
		}
	}

	var unplaced []core.UnplacedPart
	notes := []string{
		fmt.Sprintf("Column generation ran %d master LP round(s), generated %d pattern(s) and used %d of them for %d bar(s).",
			rounds, len(pool), usedPatterns, len(bars)),
		fmt.Sprintf("LP lower bound (cost units, includes stock prices): %.2f.", lpBound),
	}

	// Residual demand goes to the FFD baseline on the remaining stock.
	residual := make([]core.Part, 0, len(types))
	for i := range types {
		if remaining[i] > 0 {
			residual = append(residual, core.Part{
				ID:       types[i].ID,
				Code:     types[i].Code,
				Length:   types[i].Length,
				Quantity: remaining[i],
			})
		}
	}
	if len(residual) > 0 {
		var stocks []core.StockItem
		for i := range formats {
			if avail[i] <= 0 {
				continue
			}
			stocks = append(stocks, core.StockItem{
				ID:       formats[i].ID,
				Code:     formats[i].Code,
				Label:    formats[i].Label,
				Length:   formats[i].Length,
				Width:    formats[i].Width,
				Quantity: avail[i],
			})
		}
		if len(stocks) == 0 {
			for i := range types {
				if remaining[i] > 0 {
					unplaced = append(unplaced, core.UnplacedPart{
						PartID: types[i].ID, PartCode: types[i].Code,
						Quantity: remaining[i], Reason: "no stock left",
					})
				}
			}
			notes = append(notes, "Stock ran out before the residual demand could be placed.")
		} else {
			sub := core.Normalize(core.Problem{Parts: residual, Stocks: stocks, Rules: p.Rules})
			subSol, err := New().Solve(ctx, sub, nil)
			if err == nil {
				for _, sheet := range subSol.Sheets {
					if bar, ok := barFromSheet(sheet, formats); ok {
						bars = append(bars, bar)
					}
				}
				unplaced = append(unplaced, subSol.Unplaced...)
				if len(subSol.Sheets) > 0 {
					notes = append(notes, fmt.Sprintf(
						"The residual demand was placed with the FFD baseline on %d bar(s).", len(subSol.Sheets)))
				}
			} else {
				unplaced = append(unplaced, aggregateUnplaced(types, remaining, "placement failed")...)
			}
		}
	}

	before := len(bars)
	bars = compactBars(bars, rules.Kerf)
	if removed := before - len(bars); removed > 0 {
		notes = append(notes, fmt.Sprintf("Compaction dissolved %d bar(s).", removed))
	}
	return bars, unplaced, notes
}

func barFromPattern(pat cgPattern, types []cgType, formats []cgFormat) barState {
	bar := barState{}
	if pat.format >= 0 && pat.format < len(formats) {
		bar.format = formats[pat.format]
	}
	for i, count := range pat.counts {
		for k := 0; k < count; k++ {
			bar.pieces = append(bar.pieces, core.Part{
				ID:       types[i].ID,
				Code:     types[i].Code,
				Length:   types[i].Length,
				Quantity: 1,
			})
		}
	}
	return bar
}

// compactBars repeatedly dissolves the least filled bar into the others using
// a best-fit rule. It is exact for 1d: a move is legal when the piece plus the
// kerf fits in the destination's free length.
func compactBars(bars []barState, kerf core.Dim) []barState {
	for round := 0; round < 100; round++ {
		order := make([]int, len(bars))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(a, b int) bool {
			fa, fb := fillRatio(bars[order[a]]), fillRatio(bars[order[b]])
			if fa != fb {
				return fa < fb
			}
			return order[a] < order[b]
		})

		progress := false
		for _, j := range order {
			if next, ok := dissolveBar(bars, j, kerf); ok {
				bars = next
				progress = true
				break
			}
		}
		if !progress {
			break
		}
	}
	return bars
}

// dissolveBar moves every piece of bar j elsewhere. It plans all moves first
// and only commits when every piece found a home, so a failed attempt leaves
// the plan untouched. The returned slice has bar j removed.
func dissolveBar(bars []barState, j int, kerf core.Dim) ([]barState, bool) {
	if j < 0 || j >= len(bars) || len(bars) < 2 || len(bars[j].pieces) == 0 {
		return bars, false
	}

	pieces := append([]core.Part(nil), bars[j].pieces...)
	sort.SliceStable(pieces, func(a, b int) bool {
		if pieces[a].Length != pieces[b].Length {
			return pieces[a].Length > pieces[b].Length
		}
		return pieces[a].Code < pieces[b].Code
	})

	simFree := make([]core.Dim, len(bars))
	simCount := make([]int, len(bars))
	for i := range bars {
		simFree[i] = freeLength(bars[i], kerf)
		simCount[i] = len(bars[i].pieces)
	}

	plan := make([]int, len(pieces))
	for k, piece := range pieces {
		dest := -1
		bestFree := core.Dim(math.MaxInt64)
		for i := range bars {
			if i == j {
				continue
			}
			need := piece.Length
			if simCount[i] > 0 {
				need += kerf
			}
			if need > simFree[i] {
				continue
			}
			if leftover := simFree[i] - need; leftover < bestFree {
				bestFree = leftover
				dest = i
			}
		}
		if dest < 0 {
			return bars, false
		}
		simFree[dest] -= piece.Length
		if simCount[dest] > 0 {
			simFree[dest] -= kerf
		}
		simCount[dest]++
		plan[k] = dest
	}

	next := make([]barState, 0, len(bars)-1)
	for i := range bars {
		if i == j {
			continue
		}
		next = append(next, bars[i])
	}
	for k, piece := range pieces {
		dest := plan[k]
		if dest > j {
			dest-- // the source bar was removed
		}
		next[dest].pieces = append(next[dest].pieces, piece)
	}
	return next, true
}

func freeLength(bar barState, kerf core.Dim) core.Dim {
	used := core.Dim(0)
	for _, piece := range bar.pieces {
		used += piece.Length
	}
	if n := len(bar.pieces); n > 1 {
		used += core.Dim(n-1) * kerf
	}
	free := bar.format.Usable - used
	if free < 0 {
		return 0
	}
	return free
}

func fillRatio(bar barState) float64 {
	if bar.format.Usable <= 0 {
		return 0
	}
	used := core.Dim(0)
	for _, piece := range bar.pieces {
		used += piece.Length
	}
	return float64(used) / float64(bar.format.Usable)
}

func barFromSheet(sheet core.SheetPlan, formats []cgFormat) (barState, bool) {
	format, ok := formatByID(formats, sheet.StockID)
	if !ok {
		// Synthesize a format from the sheet so nothing is lost.
		width := sheet.Height
		if width <= 0 {
			width = core.Millimeter
		}
		format = cgFormat{ID: sheet.StockID, Code: sheet.StockCode, Label: sheet.Label, Length: sheet.Width, Width: width, Usable: sheet.Width, Cost: 1}
	}
	bar := barState{format: format}
	for _, pl := range sheet.Placements {
		bar.pieces = append(bar.pieces, core.Part{ID: pl.PartID, Code: pl.PartCode, Length: pl.W, Quantity: 1})
	}
	return bar, len(bar.pieces) > 0
}

func formatByID(formats []cgFormat, id string) (cgFormat, bool) {
	for _, format := range formats {
		if format.ID == id {
			return format, true
		}
	}
	return cgFormat{}, false
}

// buildBarSheet lays out a bar and produces placements, offcuts and cut steps.
func buildBarSheet(bar barState, index int, rules core.Rules) core.SheetPlan {
	sheet := core.SheetPlan{
		Index:     index,
		StockID:   bar.format.ID,
		StockCode: bar.format.Code,
		Label:     bar.format.Label,
		Width:     bar.format.Length,
		Height:    bar.format.Width,
	}
	if sheet.Label == "" {
		sheet.Label = sheet.StockCode
	}

	x := rules.Trim
	used := core.Dim(0)
	for k, piece := range bar.pieces {
		if k > 0 {
			x += rules.Kerf
		}
		sheet.Placements = append(sheet.Placements, core.Placement{
			PartID:   piece.ID,
			PartCode: piece.Code,
			X:        x,
			Y:        0,
			W:        piece.Length,
			H:        sheet.Height,
		})
		x += piece.Length
		used += piece.Length
	}
	if rules.OffcutMinLength > 0 && len(bar.pieces) > 0 {
		// Free space minus the cut that separates the offcut from the last piece.
		rem := bar.format.Usable - used - core.Dim(len(bar.pieces))*rules.Kerf
		if rem >= rules.OffcutMinLength {
			sheet.Offcuts = []core.Rect{{X: x + rules.Kerf, Y: 0, W: rem, H: sheet.Height}}
		}
	}
	if steps, ok := cutter.ForSheetStages(sheet, rules.Kerf, rules.MaxCutStages); ok {
		sheet.CutSteps = steps
	}
	return sheet
}

// ----------------------------------------------------------------- helpers ---

func assembleSolution(p core.Problem, sheets []core.SheetPlan, unplaced []core.UnplacedPart, start time.Time, notes []string) core.Solution {
	sol := core.Solution{
		Solver:        "cg-1d",
		SolverVersion: cgVersion,
		Seed:          p.Seed,
		Sheets:        sheets,
		Unplaced:      unplaced,
		Notes:         notes,
	}
	sol.Metrics = core.Summarize(p, sheets, unplaced, time.Since(start).Milliseconds())
	return sol
}

func lengthTotals(sheets []core.SheetPlan) (stockLenM, usedLenM float64) {
	for _, sheet := range sheets {
		stockLenM += core.LengthM(sheet.Width)
		for _, pl := range sheet.Placements {
			usedLenM += core.LengthM(pl.W)
		}
	}
	return stockLenM, usedLenM
}

func aggregateUnplaced(types []cgType, remaining []int, reason string) []core.UnplacedPart {
	var out []core.UnplacedPart
	for i := range types {
		if remaining[i] <= 0 {
			continue
		}
		out = append(out, core.UnplacedPart{
			PartID:   types[i].ID,
			PartCode: types[i].Code,
			Quantity: remaining[i],
			Reason:   reason,
		})
	}
	return out
}
