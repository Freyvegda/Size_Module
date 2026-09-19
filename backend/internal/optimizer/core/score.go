package core

import "math"

// Summarize assembles the metrics of a solution from its sheets and leftovers.
// Solvers only have to fill in the raw geometry; the scorecard is derived here
// so every solver is measured the same way.
func Summarize(p Problem, sheets []SheetPlan, unplaced []UnplacedPart, elapsedMS int64) Metrics {
	m := Metrics{
		SheetCount: len(sheets),
		ElapsedMS:  elapsedMS,
	}
	for i := range p.Parts {
		m.PartsRequested += p.Parts[i].Quantity
	}
	patterns := map[string]struct{}{}
	costByStock := make(map[string]float64, len(p.Stocks))
	remnantIDs := make(map[string]struct{}, len(p.Stocks))
	for i := range p.Stocks {
		if p.Stocks[i].ID != "" {
			costByStock[p.Stocks[i].ID] = p.Stocks[i].CostPerUnit
		}
		if p.Stocks[i].IsRemnant && p.Stocks[i].ID != "" {
			remnantIDs[p.Stocks[i].ID] = struct{}{}
		}
	}
	for _, sh := range sheets {
		m.StockAreaM2 += AreaM2(sh.Width, sh.Height)
		m.Cost += costByStock[sh.StockID]
		if _, ok := remnantIDs[sh.StockID]; ok {
			m.RemnantSheets++
		}
		var placedArea Dim
		for _, pl := range sh.Placements {
			placedArea += pl.W * pl.H
		}
		// placedArea is in µm²; square meters need one division by 1e12.
		m.PartAreaM2 += float64(placedArea) / 1e12
		m.PartsPlaced += len(sh.Placements)
		for _, off := range sh.Offcuts {
			m.OffcutAreaM2 += AreaM2(off.W, off.H)
		}
		patterns[patternKey(sh)] = struct{}{}
	}
	m.PatternCount = len(patterns)
	m.ScrapAreaM2 = m.StockAreaM2 - m.PartAreaM2 - m.OffcutAreaM2
	if m.ScrapAreaM2 < 0 {
		m.ScrapAreaM2 = 0
	}
	if m.StockAreaM2 > 0 {
		m.YieldPct = 100 * m.PartAreaM2 / m.StockAreaM2
		m.WastePct = 100 * (1 - m.PartAreaM2/m.StockAreaM2)
	}
	return m
}

// patternKey groups sheets that have the same multiset of part codes.
func patternKey(sh SheetPlan) string {
	key := ""
	for _, pl := range sh.Placements {
		key += pl.PartCode + "|"
	}
	if len(key) == 0 {
		key = "empty"
	}
	return key
}

// Score converts metrics into a single "higher is better" number using the
// objective weights of the problem. Solvers use it to compare candidate
// solutions; the API exposes it for transparency.
func Score(p Problem, m Metrics) float64 {
	w := p.Objective.Weights
	if w == (Weights{}) {
		w = DefaultWeights()
	}
	fill := 0.0
	if m.PartsRequested > 0 {
		fill = float64(m.PartsPlaced) / float64(m.PartsRequested)
	}
	s := 0.0
	s += w.FillPriority * fill
	// Only fresh stock sheets are charged: a sheet cut from a physical remnant
	// is already paid-for material, so using it is not "using another sheet".
	freshSheets := m.SheetCount - m.RemnantSheets
	if freshSheets < 0 {
		freshSheets = 0
	}
	s -= w.MinSheets * float64(freshSheets)
	s -= w.MinScrap * m.ScrapAreaM2
	s -= w.MinPatterns * float64(m.PatternCount)
	s -= w.MinOffcutArea * m.OffcutAreaM2
	s -= w.Cost * m.Cost
	return math.Round(s*1e6) / 1e6
}
