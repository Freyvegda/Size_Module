package campaigns

import (
	"fmt"
	"strings"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/id"
)

// RemnantLabel builds the label of an offcut that re-enters a campaign budget:
// unique per campaign, item, sheet and offcut, and short enough to write on the
// piece.
func RemnantLabel(campaignID string, itemSeq, sheetIndex, offcutIndex int) string {
	short := campaignID
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("CMP-%s-%02d-%02d-%d",
		strings.ToUpper(short), itemSeq, sheetIndex+1, offcutIndex+1)
}

// ConsumeBudget returns the campaign's stock budget after an item was solved:
//
//   - every sheet consumes one unit of the stock entry it came from; a physical
//     remnant (quantity one) leaves the budget entirely;
//   - every offcut that meets the offcut policy re-enters the budget as a
//     labelled remnant, so later items can use it;
//   - costs are prorated onto the new remnants exactly like plan acceptance
//     does, so a valuable leftover is not treated as free.
//
// The function is pure: the caller decides how to persist the result, which
// keeps budget arithmetic testable without a database.
func ConsumeBudget(stock []core.StockItem, sol core.Solution, rules core.Rules, label func(sheetIndex, offcutIndex int) string) []core.StockItem {
	return ConsumeBudgetWithIDs(stock, sol, rules, label, func(_, _ int) string { return id.New() })
}

// ConsumeBudgetWithIDs is ConsumeBudget with caller-supplied ids for the
// remnants that re-enter the budget. The campaign store uses it to point the
// budget at real stock_items rows, so accepting a later plan consumes the piece
// instead of silently missing it. newID is only called for offcuts that pass
// the offcut policy.
func ConsumeBudgetWithIDs(stock []core.StockItem, sol core.Solution, rules core.Rules, label func(sheetIndex, offcutIndex int) string, newID func(sheetIndex, offcutIndex int) string) []core.StockItem {
	// Count how many sheets came from each stock entry. A physical remnant has
	// quantity one, so a single use removes it from the budget.
	used := map[string]int{}
	for _, sheet := range sol.Sheets {
		if sheet.StockID != "" {
			used[sheet.StockID]++
		}
	}

	// Remaining entries keep their order; fully consumed ones drop out.
	out := make([]core.StockItem, 0, len(stock)+len(sol.Sheets))
	for _, entry := range stock {
		remaining := entry.Quantity - used[entry.ID]
		if remaining <= 0 {
			continue
		}
		entry.Quantity = remaining
		out = append(out, entry)
	}

	// Offcuts come back as new remnants, in a deterministic order. Costs and
	// shapes come from the original budget entry, so a consumed entry still
	// tells us what its leftovers are worth.
	for _, sheet := range sol.Sheets {
		original, known := findStock(stock, sheet.StockID)
		if !known {
			continue
		}
		is1D := original.Length > 0
		for i, off := range sheet.Offcuts {
			if !offcutReusable(off, is1D, rules) {
				continue
			}
			remnant := core.StockItem{
				ID:          newID(sheet.Index, i),
				FormatID:    original.FormatID,
				Code:        sheet.StockCode,
				Label:       label(sheet.Index, i),
				Quantity:    1,
				CostPerUnit: remnantCost(off, is1D, original),
				IsRemnant:   true,
				// Carry the material so the next item can be scoped to it.
				MaterialSpecID: original.MaterialSpecID,
			}
			if is1D {
				remnant.Length = off.W
				remnant.Width = original.Width
			} else {
				remnant.Width = off.W
				remnant.Height = off.H
			}
			out = append(out, remnant)
		}
	}
	return out
}

func findStock(stock []core.StockItem, stockID string) (core.StockItem, bool) {
	for _, entry := range stock {
		if entry.ID == stockID {
			return entry, true
		}
	}
	return core.StockItem{}, false
}

// offcutReusable re-applies the offcut policy, so a campaign cannot turn scrap
// into fake stock.
func offcutReusable(off core.Rect, is1D bool, rules core.Rules) bool {
	if off.W <= 0 || off.H <= 0 {
		return false
	}
	if is1D {
		return off.W >= rules.OffcutMinLength
	}
	return off.W >= rules.OffcutMinW && off.H >= rules.OffcutMinH
}

// remnantCost prorates the source cost onto the leftover, so a valuable
// remnant is not treated as free by the items that follow.
func remnantCost(off core.Rect, is1D bool, source core.StockItem) float64 {
	if source.CostPerUnit <= 0 {
		return 0
	}
	var share float64
	if is1D {
		if source.Length <= 0 {
			return 0
		}
		share = float64(off.W) / float64(source.Length)
	} else {
		area := float64(source.Width) * float64(source.Height)
		if area <= 0 {
			return 0
		}
		share = float64(off.W) * float64(off.H) / area
	}
	if share > 1 {
		share = 1
	}
	return source.CostPerUnit * share
}
