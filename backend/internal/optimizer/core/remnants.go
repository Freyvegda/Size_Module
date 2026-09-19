package core

import "sort"

// PrioritizeRemnants reorders a problem's stock list so that physical remnants
// are offered to the solver before fresh catalog stock. This is the
// "remnant-first allocation" policy: leftover pieces are already paid for, so
// using them first keeps them in circulation instead of growing the pile.
//
// The reorder is a stable partition, so the caller's order is preserved within
// the two groups and the plan stays deterministic. It is applied by Normalize
// when Rules.PreferRemnants is set.
func PrioritizeRemnants(p *Problem) {
	if p == nil || !p.Rules.PreferRemnants {
		return
	}
	sort.SliceStable(p.Stocks, func(i, j int) bool {
		return p.Stocks[i].IsRemnant && !p.Stocks[j].IsRemnant
	})
}
