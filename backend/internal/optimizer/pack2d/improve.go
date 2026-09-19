package pack2d

import (
	"context"
	"math/rand"
	"sort"
	"time"

	"github.com/size-module/backend/internal/optimizer/core"
)

// Local search over an existing 2D plan. Beam search builds a plan from
// scratch with a fixed heuristic; local search takes that plan and attacks the
// expensive mistakes, in this order of value:
//
//  1. dissolve a sheet: move every piece of the least-used sheet into the other
//     sheets one by one; if the last piece lands somewhere, a whole sheet is
//     gone. This is the only move that reduces sheet count, which the objective
//     rewards directly.
//  2. merge two sheets: pack both part sets into one sheet of the larger
//     format. Cheap to test, occasionally removes a sheet.
//  3. repack a sheet with a wider beam: keeps the sheet count but can shrink
//     scrap and grow reusable offcuts.
//
// Every candidate is rebuilt with the guillotine-safe beam packer and scored
// with the real objective, so nothing is accepted that makes the plan worse —
// and nothing is accepted that is not cuttable.
//
// The search is any-time: it keeps going until the context deadline, which is
// how the polish solver spends the whole time budget.

// Improve returns the best plan it can find within the budget. It never returns
// a worse plan than the input.
//
// The effective deadline is the tighter of the caller's context and the
// problem's own BudgetMS, so a caller that forgets a deadline still gets a
// bounded search.
func Improve(ctx context.Context, p core.Problem, in core.Solution, seed uint64, progress core.ProgressFunc) core.Solution {
	if len(in.Sheets) == 0 || !is2D(p) {
		return in
	}
	if p.BudgetMS > 0 {
		budgetDeadline := time.Now().Add(time.Duration(p.BudgetMS) * time.Millisecond)
		if existing, ok := ctx.Deadline(); !ok || budgetDeadline.Before(existing) {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, budgetDeadline)
			defer cancel()
		}
	}
	if seed == 0 {
		seed = 1
	}
	im := &improver{
		p:     p,
		rules: normalizeRules(p.Rules),
		beam:  NewBeam(),
		rng:   rand.New(rand.NewSource(int64(seed) ^ 0x5deece66d)),
		byID:  partIndex(p.Parts),
		best:  in,
	}
	im.best.Metrics = core.Summarize(p, in.Sheets, in.Unplaced, in.Metrics.ElapsedMS)
	im.bestScore = core.Score(p, im.best.Metrics)

	im.run(ctx, progress)
	im.best.Metrics = core.Summarize(p, im.best.Sheets, im.best.Unplaced, im.best.Metrics.ElapsedMS)
	return im.best
}

type improver struct {
	p         core.Problem
	rules     core.Rules
	beam      *BeamSolver
	rng       *rand.Rand
	byID      map[string]core.Part
	best      core.Solution
	bestScore float64
}

// partIndex maps part ids to their original definitions.
func partIndex(parts []core.Part) map[string]core.Part {
	out := make(map[string]core.Part, len(parts))
	for _, part := range parts {
		out[part.ID] = part
	}
	return out
}

const (
	// maxStagnant bounds the search: after this many consecutive fruitless
	// rounds the plan is as good as this neighbourhood allows.
	maxStagnant = 14
	// maxRounds is a hard safety cap; the budget and stagnation usually bind
	// first. 25 rounds are enough to reach the sheet removals and the useful
	// repacks, and small enough that the search finishes before the time budget
	// on normal instances — which keeps results deterministic and cheap.
	maxRounds = 25
	// minScoreGain filters out noise moves. Without it, a repack that gains
	// 0.0001 points resets the stagnation counter and the search never settles,
	// which made the solver's result depend on machine speed.
	minScoreGain = 0.01
	// maxSheetsPerStep / maxPairsPerStep bound the work of a single round.
	maxSheetsPerStep = 3
	maxPairsPerStep  = 3
)

func (im *improver) run(ctx context.Context, progress core.ProgressFunc) {
	stagnant := 0
	for round := 0; round < maxRounds; round++ {
		if ctx.Err() != nil || stagnant >= maxStagnant {
			return
		}
		if im.step(ctx) {
			stagnant = 0
			if progress != nil {
				progress(im.best)
			}
		} else {
			stagnant++
		}
	}
}

func (im *improver) step(ctx context.Context) bool {
	// Work is bounded per step: a handful of the emptiest sheets, a few pairs,
	// and one repack. Unbounded loops made every step expensive and burned the
	// whole budget even on small problems.
	targets := im.sheetOrder()
	if len(targets) > maxSheetsPerStep {
		targets = targets[:maxSheetsPerStep]
	}

	for _, j := range targets {
		if im.tryDissolve(ctx, j) {
			return true
		}
	}
	pairs := im.pairs()
	if len(pairs) > maxPairsPerStep {
		pairs = pairs[:maxPairsPerStep]
	}
	for _, pair := range pairs {
		if im.tryMerge(ctx, pair[0], pair[1]) {
			return true
		}
	}
	for _, j := range targets {
		if im.tryRepack(ctx, j) {
			return true
		}
	}
	return false
}

// sheetOrder lists sheet indexes by utilisation ascending (the emptiest sheets
// first), rotated by the RNG so repeated rounds try different targets.
func (im *improver) sheetOrder() []int {
	order := make([]int, len(im.best.Sheets))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		ua, ub := utilisation(im.best.Sheets[order[a]]), utilisation(im.best.Sheets[order[b]])
		if ua != ub {
			return ua < ub
		}
		return order[a] < order[b]
	})
	if len(order) > 1 {
		shift := im.rng.Intn(len(order))
		order = append(order[shift:], order[:shift]...)
	}
	return order
}

func (im *improver) pairs() [][2]int {
	n := len(im.best.Sheets)
	var out [][2]int
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			out = append(out, [2]int{i, j})
		}
	}
	if len(out) > 1 {
		im.rng.Shuffle(len(out), func(a, b int) { out[a], out[b] = out[b], out[a] })
	}
	return out
}

// tryDissolve moves every piece of sheet j into the other sheets. The move only
// commits when all pieces find a home, which deletes the sheet entirely.
func (im *improver) tryDissolve(ctx context.Context, j int) bool {
	if j < 0 || j >= len(im.best.Sheets) {
		return false
	}
	source := im.best.Sheets[j]
	pieces := sheetInstances(source, im.byID)
	if len(pieces) == 0 {
		return false
	}
	sort.SliceStable(pieces, func(a, b int) bool {
		areaA := pieces[a].Width * pieces[a].Height
		areaB := pieces[b].Width * pieces[b].Height
		if areaA != areaB {
			return areaA > areaB
		}
		return pieces[a].Code < pieces[b].Code
	})

	sheets := make([]core.SheetPlan, 0, len(im.best.Sheets)-1)
	for i, s := range im.best.Sheets {
		if i != j {
			sheets = append(sheets, s)
		}
	}
	if len(sheets) == 0 {
		return false
	}

	// Best-fit destinations: try the fullest sheet that can still take the
	// piece first, so empty sheets stay empty for later merges. The first
	// destination that fits wins, which keeps a dissolve attempt cheap.
	destinations := make([]int, len(sheets))
	for i := range destinations {
		destinations[i] = i
	}
	sort.SliceStable(destinations, func(a, b int) bool {
		ua, ub := utilisation(sheets[destinations[a]]), utilisation(sheets[destinations[b]])
		if ua != ub {
			return ua > ub
		}
		return destinations[a] < destinations[b]
	})

	for _, piece := range pieces {
		placed := false
		for _, i := range destinations {
			stock := stockOf(sheets[i], im.p)
			if stock.Width <= 0 || stock.Height <= 0 {
				continue
			}
			candidateParts := append(sheetInstances(sheets[i], im.byID), piece)
			node, ok := im.beam.packParts(ctx, usableOf(stock, im.rules), stock.Defects, im.rules, candidateParts)
			if !ok {
				continue
			}
			sheets[i] = sheetFromNode(stock, im.rules, node, sheets[i].Index)
			placed = true
			break
		}
		if !placed || ctx.Err() != nil {
			return false
		}
	}

	candidate := reindex(sheets)
	score, ok := im.accept(candidate)
	if !ok {
		return false
	}
	im.commit(candidate, score)
	return true
}

// tryMerge packs all pieces of sheets a and b into a single sheet of the larger
// format.
func (im *improver) tryMerge(ctx context.Context, a, b int) bool {
	if a < 0 || b < 0 || a >= len(im.best.Sheets) || b >= len(im.best.Sheets) || a == b {
		return false
	}
	first, second := im.best.Sheets[a], im.best.Sheets[b]
	stockA, stockB := stockOf(first, im.p), stockOf(second, im.p)
	stock := stockA
	if stockB.Width*stockB.Height > stockA.Width*stockA.Height {
		stock = stockB
	}
	pieces := append(sheetInstances(first, im.byID), sheetInstances(second, im.byID)...)
	node, ok := im.beam.packParts(ctx, usableOf(stock, im.rules), stock.Defects, im.rules, pieces)
	if !ok {
		return false
	}
	merged := sheetFromNode(stock, im.rules, node, first.Index)

	sheets := make([]core.SheetPlan, 0, len(im.best.Sheets)-1)
	for i, s := range im.best.Sheets {
		switch i {
		case a:
			sheets = append(sheets, merged)
		case b:
			// dropped
		default:
			sheets = append(sheets, s)
		}
	}
	candidate := reindex(sheets)
	score, ok := im.accept(candidate)
	if !ok {
		return false
	}
	im.commit(candidate, score)
	return true
}

// tryRepack rebuilds one sheet with a wider beam. It cannot reduce the sheet
// count, but it often turns unusable scrap into reusable offcuts.
func (im *improver) tryRepack(ctx context.Context, j int) bool {
	if j < 0 || j >= len(im.best.Sheets) {
		return false
	}
	sheet := im.best.Sheets[j]
	stock := stockOf(sheet, im.p)
	if stock.Width <= 0 || stock.Height <= 0 {
		return false
	}
	width := defaultBeamWidth + 8*im.rng.Intn(4)
	probe := &BeamSolver{Width: width, CandidatesPerStep: defaultBeamCands + 2}
	node, ok := probe.packParts(ctx, usableOf(stock, im.rules), stock.Defects, im.rules, sheetInstances(sheet, im.byID))
	if !ok {
		return false
	}
	candidate := make([]core.SheetPlan, len(im.best.Sheets))
	copy(candidate, im.best.Sheets)
	candidate[j] = sheetFromNode(stock, im.rules, node, sheet.Index)

	score, ok := im.accept(candidate)
	if !ok {
		return false
	}
	im.commit(candidate, score)
	return true
}

// accept scores a candidate plan against the current best. Only meaningful
// improvements are accepted, so the search settles instead of chasing noise.
func (im *improver) accept(candidate []core.SheetPlan) (float64, bool) {
	metrics := core.Summarize(im.p, candidate, im.best.Unplaced, 0)
	score := core.Score(im.p, metrics)
	if score > im.bestScore+minScoreGain {
		return score, true
	}
	return score, false
}

func (im *improver) commit(sheets []core.SheetPlan, score float64) {
	im.best.Sheets = sheets
	im.bestScore = score
	im.best.Metrics = core.Summarize(im.p, sheets, im.best.Unplaced, 0)
}

// ---------------------------------------------------------------- helpers ---

func is2D(p core.Problem) bool {
	for _, part := range p.Parts {
		if part.Width > 0 && part.Height > 0 {
			return true
		}
	}
	return false
}

func normalizeRules(rules core.Rules) core.Rules {
	out := rules
	if out.Kerf < 0 {
		out.Kerf = 0
	}
	if out.Trim < 0 {
		out.Trim = 0
	}
	return out
}

// sheetInstances explodes a sheet's placements into individual parts. Original
// part constraints (grain, rotation permission) are preserved so that local
// search can never re-pack a piece in a way its rules forbid.
func sheetInstances(sheet core.SheetPlan, byID map[string]core.Part) []core.Part {
	out := make([]core.Part, 0, len(sheet.Placements))
	for _, pl := range sheet.Placements {
		if original, ok := byID[pl.PartID]; ok {
			out = append(out, core.Part{
				ID:             original.ID,
				Code:           pl.PartCode,
				MaterialSpecID: original.MaterialSpecID,
				Width:          pl.W,
				Height:         pl.H,
				Quantity:       1,
				Grain:          original.Grain,
				AllowRotate:    original.AllowRotate,
				Priority:       original.Priority,
			})
			continue
		}
		out = append(out, core.Part{
			ID:          pl.PartID,
			Code:        pl.PartCode,
			Width:       pl.W,
			Height:      pl.H,
			Quantity:    1,
			AllowRotate: true,
			Priority:    pl.Priority,
		})
	}
	return out
}

// stockOf reconstructs the stock item a sheet was cut from. The sheet's own
// dimensions are authoritative; defects are looked up by stock id when known.
func stockOf(sheet core.SheetPlan, p core.Problem) core.StockItem {
	stock := core.StockItem{
		ID:     sheet.StockID,
		Code:   sheet.StockCode,
		Label:  sheet.Label,
		Width:  sheet.Width,
		Height: sheet.Height,
	}
	for i := range p.Stocks {
		if sheet.StockID != "" && p.Stocks[i].ID == sheet.StockID {
			stock.Defects = p.Stocks[i].Defects
			break
		}
	}
	return stock
}

func usableOf(stock core.StockItem, rules core.Rules) core.Rect {
	return core.Rect{
		X: rules.Trim,
		Y: rules.Trim,
		W: stock.Width - 2*rules.Trim,
		H: stock.Height - 2*rules.Trim,
	}
}

func utilisation(sheet core.SheetPlan) float64 {
	area := float64(sheet.Width) * float64(sheet.Height)
	if area <= 0 {
		return 0
	}
	var used float64
	for _, pl := range sheet.Placements {
		used += float64(pl.W) * float64(pl.H)
	}
	return used / area
}

func reindex(sheets []core.SheetPlan) []core.SheetPlan {
	out := make([]core.SheetPlan, len(sheets))
	copy(out, sheets)
	for i := range out {
		out[i].Index = i
	}
	return out
}
