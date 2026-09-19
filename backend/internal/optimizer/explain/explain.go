// Package explain turns solution metrics into plain-language notes, so a plan
// is never a black box: waste is broken down and compared with a lower bound.
package explain

import (
	"fmt"
	"math"

	"github.com/size-module/backend/internal/optimizer/core"
)

// Notes returns human readable observations about a solution.
func Notes(p core.Problem, s core.Solution) []string {
	m := s.Metrics
	var notes []string

	if m.SheetCount > 0 {
		notes = append(notes, fmt.Sprintf(
			"Used %d sheet(s): parts cover %.2f m² of %.2f m² stock (%.1f%% yield).",
			m.SheetCount, m.PartAreaM2, m.StockAreaM2, m.YieldPct))
	}
	if m.RemnantSheets > 0 {
		notes = append(notes, fmt.Sprintf(
			"%d of those sheet(s) came from reusable remnants instead of new stock.",
			m.RemnantSheets))
	} else {
		offered := 0
		for _, st := range p.Stocks {
			if st.IsRemnant {
				offered++
			}
		}
		if offered > 0 {
			notes = append(notes, fmt.Sprintf(
				"%d available remnant(s) were considered; using them did not improve this plan, so they stay in stock.",
				offered))
		}
	}
	if m.OffcutAreaM2 > 0 {
		count := 0
		for _, sh := range s.Sheets {
			count += len(sh.Offcuts)
		}
		notes = append(notes, fmt.Sprintf(
			"%d reusable offcut(s), %.2f m² in total, stay in circulation instead of becoming scrap.",
			count, m.OffcutAreaM2))
	}
	if m.TrimAreaM2 > 0 || m.KerfAreaM2 > 0 {
		notes = append(notes, fmt.Sprintf(
			"Edge trim %.2f m² and saw kerf ≈ %.2f m² are machine losses that cannot be packed away.",
			m.TrimAreaM2, m.KerfAreaM2))
	}
	if bound, ok := lowerBound(p, m); ok && m.SheetCount > bound {
		notes = append(notes, fmt.Sprintf(
			"Area lower bound is %d sheet(s); this layout uses %d. The gap is shape waste, not size waste.",
			bound, m.SheetCount))
	}
	if m.PartsPlaced < m.PartsRequested {
		notes = append(notes, fmt.Sprintf(
			"%d of %d requested pieces could not be placed; add stock or relax the rules.",
			m.PartsRequested-m.PartsPlaced, m.PartsRequested))
	}
	if m.PatternCount > 1 {
		notes = append(notes, fmt.Sprintf(
			"%d different patterns are required; fewer patterns mean fewer machine setups.",
			m.PatternCount))
	}
	return notes
}

// lowerBound estimates the minimum number of sheets any solution can use
// (2D area or 1D length bound). It is a bound, not an answer: it ignores shape.
func lowerBound(p core.Problem, m core.Metrics) (int, bool) {
	if m.StockAreaM2 <= 0 || m.PartAreaM2 <= 0 {
		return 0, false
	}
	maxArea := 0.0
	for _, st := range p.Stocks {
		if st.Quantity <= 0 || st.Width <= 0 || st.Height <= 0 {
			continue
		}
		usableW := st.Width - 2*p.Rules.Trim
		usableH := st.Height - 2*p.Rules.Trim
		if usableW <= 0 || usableH <= 0 {
			continue
		}
		if a := core.AreaM2(usableW, usableH); a > maxArea {
			maxArea = a
		}
	}
	if maxArea <= 0 {
		return 0, false
	}
	return int(math.Ceil(m.PartAreaM2 / maxArea)), true
}
