// Package costing values a cutting plan: what new material it consumes, what
// remnants it takes from the pool, what leftovers it keeps in circulation and
// what the unusable waste is worth.
//
// Costs are unitless numbers (the same currency the stock catalog uses). The
// model is deliberately simple and transparent:
//
//   - a sheet cut from a catalog format costs its full CostPerUnit;
//   - a sheet cut from a physical remnant costs the remnant's (prorated) value;
//   - part, trim, kerf, scrap and offcut areas each carry their share of the
//     sheet cost;
//   - NetCost = material taken − offcut credit, i.e. the material actually
//     consumed by the order once the leftovers that stay in stock are valued.
package costing

import (
	"math"

	"github.com/size-module/backend/internal/optimizer/core"
)

// Report is the cost breakdown of one solution.
type Report struct {
	// NewMaterialCost is the value of sheets cut from catalog formats.
	NewMaterialCost float64 `json:"newMaterialCost"`
	// RemnantCost is the (prorated) value of physical remnants consumed.
	RemnantCost float64 `json:"remnantCost"`
	// TotalStockCost is NewMaterialCost + RemnantCost.
	TotalStockCost float64 `json:"totalStockCost"`
	// OffcutCredit is the value of reusable leftovers that stay in stock.
	OffcutCredit float64 `json:"offcutCredit"`
	// NetCost is TotalStockCost - OffcutCredit: what the order really consumed.
	NetCost float64 `json:"netCost"`
	// PartCost, TrimCost, KerfCost and ScrapCost split the stock value by where
	// the material went.
	PartCost  float64 `json:"partCost"`
	TrimCost  float64 `json:"trimCost"`
	KerfCost  float64 `json:"kerfCost"`
	ScrapCost float64 `json:"scrapCost"`
	// CostPerPart and CostPerM2 value the net cost against the parts produced.
	CostPerPart float64 `json:"costPerPart"`
	CostPerM2   float64 `json:"costPerM2"`
}

// Breakdown values a solution from its problem snapshot. It is a pure function
// of the geometry and the stock costs, so the same plan always costs the same.
func Breakdown(p core.Problem, sol core.Solution) Report {
	costByStock := make(map[string]float64, len(p.Stocks))
	remnantByStock := make(map[string]bool, len(p.Stocks))
	for _, st := range p.Stocks {
		if st.ID == "" {
			continue
		}
		costByStock[st.ID] = st.CostPerUnit
		remnantByStock[st.ID] = st.IsRemnant
	}

	var report Report
	for _, sheet := range sol.Sheets {
		sheetCost := costByStock[sheet.StockID]
		if sheetCost <= 0 {
			continue
		}
		report.TotalStockCost += sheetCost
		if remnantByStock[sheet.StockID] {
			report.RemnantCost += sheetCost
		} else {
			report.NewMaterialCost += sheetCost
		}
	}

	m := sol.Metrics
	if m.StockAreaM2 <= 0 {
		// The caller may hand us a solution whose metrics were not summarised.
		m = core.Summarize(p, sol.Sheets, sol.Unplaced, m.ElapsedMS)
	}
	if m.StockAreaM2 <= 0 {
		return Report{}
	}

	share := func(area float64) float64 {
		if area < 0 {
			area = 0
		}
		return report.TotalStockCost * area / m.StockAreaM2
	}
	report.PartCost = share(m.PartAreaM2)
	report.TrimCost = share(m.TrimAreaM2)
	report.KerfCost = share(m.KerfAreaM2)
	report.ScrapCost = share(m.ScrapAreaM2)
	report.OffcutCredit = share(m.OffcutAreaM2)
	report.NetCost = report.TotalStockCost - report.OffcutCredit

	if m.PartsPlaced > 0 {
		report.CostPerPart = report.NetCost / float64(m.PartsPlaced)
	}
	if m.PartAreaM2 > 0 {
		report.CostPerM2 = report.NetCost / m.PartAreaM2
	}
	return round(report)
}

func round(r Report) Report {
	r.NewMaterialCost = round4(r.NewMaterialCost)
	r.RemnantCost = round4(r.RemnantCost)
	r.TotalStockCost = round4(r.TotalStockCost)
	r.OffcutCredit = round4(r.OffcutCredit)
	r.NetCost = round4(r.NetCost)
	r.PartCost = round4(r.PartCost)
	r.TrimCost = round4(r.TrimCost)
	r.KerfCost = round4(r.KerfCost)
	r.ScrapCost = round4(r.ScrapCost)
	r.CostPerPart = round4(r.CostPerPart)
	r.CostPerM2 = round4(r.CostPerM2)
	return r
}

func round4(v float64) float64 {
	return math.Round(v*1e4) / 1e4
}
