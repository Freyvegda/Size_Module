// Package validator enforces the invariants every solution must satisfy
// before it is ever shown to a planner: pieces inside the sheet, kerf honoured,
// grain respected, demand not exceeded and — for guillotine machines — a valid
// cut sequence. Nothing bypasses this package.
package validator

import (
	"fmt"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/cutter"
	"github.com/size-module/backend/internal/optimizer/geom"
)

type Violation struct {
	Severity   string `json:"severity"` // "error" or "warning"
	Code       string `json:"code"`
	SheetIndex int    `json:"sheetIndex,omitempty"`
	PartCode   string `json:"partCode,omitempty"`
	Message    string `json:"message"`
}

func (v Violation) Error() string { return v.Message }

func errorf(code string, sheetIndex int, partCode, format string, args ...any) Violation {
	return Violation{
		Severity:   "error",
		Code:       code,
		SheetIndex: sheetIndex,
		PartCode:   partCode,
		Message:    fmt.Sprintf(format, args...),
	}
}

func warnf(code string, sheetIndex int, partCode, format string, args ...any) Violation {
	return Violation{
		Severity:   "warning",
		Code:       code,
		SheetIndex: sheetIndex,
		PartCode:   partCode,
		Message:    fmt.Sprintf(format, args...),
	}
}

// Validate returns every violation found in a solution.
func Validate(p core.Problem, s core.Solution) []Violation {
	partByID := map[string]core.Part{}
	for _, part := range p.Parts {
		partByID[part.ID] = part
	}
	// Defect regions are piece-local; a sheet cut from a physical piece must not
	// place a part over one. Sheets without a stock id (catalog stock) have none.
	defectsByStock := map[string][]core.Rect{}
	for _, stock := range p.Stocks {
		if stock.ID != "" && len(stock.Defects) > 0 {
			defectsByStock[stock.ID] = stock.Defects
		}
	}

	var violations []Violation
	placed := map[string]int{}

	for _, sheet := range s.Sheets {
		if sheet.Width <= 0 || sheet.Height <= 0 {
			violations = append(violations, errorf("bad_sheet_size", sheet.Index, "",
				"sheet %d has non-positive size %d x %d µm", sheet.Index+1, sheet.Width, sheet.Height))
			continue
		}
		sheetRegion := core.Rect{X: 0, Y: 0, W: sheet.Width, H: sheet.Height}
		sheetDefects := defectsByStock[sheet.StockID]

		for i, pl := range sheet.Placements {
			part, known := partByID[pl.PartID]
			r := core.Rect{X: pl.X, Y: pl.Y, W: pl.W, H: pl.H}

			if pl.W <= 0 || pl.H <= 0 {
				violations = append(violations, errorf("bad_piece_size", sheet.Index, pl.PartCode,
					"piece %s on sheet %d has non-positive size", pl.PartCode, sheet.Index+1))
				continue
			}
			if !sheetRegion.Contains(r) {
				violations = append(violations, errorf("out_of_bounds", sheet.Index, pl.PartCode,
					"piece %s on sheet %d extends beyond the sheet", pl.PartCode, sheet.Index+1))
			}
			for _, d := range sheetDefects {
				if geom.Intersects(r, d) {
					violations = append(violations, errorf("defect_overlap", sheet.Index, pl.PartCode,
						"piece %s on sheet %d overlaps an unusable region of the stock",
						pl.PartCode, sheet.Index+1))
					break
				}
			}
			if known {
				if err := checkOrientation(p, part, pl); err != nil {
					violations = append(violations, errorf("bad_orientation", sheet.Index, pl.PartCode, "%v", err))
				}
			} else {
				violations = append(violations, warnf("unknown_part", sheet.Index, pl.PartCode,
					"piece %s does not match any requested part", pl.PartCode))
			}

			// Kerf: compare with all later pieces on the same sheet.
			for j := i + 1; j < len(sheet.Placements); j++ {
				other := core.Rect{
					X: sheet.Placements[j].X, Y: sheet.Placements[j].Y,
					W: sheet.Placements[j].W, H: sheet.Placements[j].H,
				}
				if !geom.Separated(r, other, p.Rules.Kerf) {
					violations = append(violations, errorf("overlap", sheet.Index, pl.PartCode,
						"pieces %s and %s on sheet %d overlap or ignore the %.1f mm kerf",
						pl.PartCode, sheet.Placements[j].PartCode, sheet.Index+1, core.ToMM(p.Rules.Kerf)))
					j = len(sheet.Placements)
				}
			}
			placed[pl.PartID]++
		}

		if p.Rules.CutMode == core.CutGuillotine && len(sheet.Placements) > 0 {
			if _, ok := cutter.ForSheet(sheet, p.Rules.Kerf); !ok {
				violations = append(violations, errorf("not_guillotine", sheet.Index, "",
					"sheet %d has no valid guillotine cut sequence", sheet.Index+1))
			} else if p.Rules.MaxCutStages > 0 {
				if _, staged := cutter.ForSheetStages(sheet, p.Rules.Kerf, p.Rules.MaxCutStages); !staged {
					violations = append(violations, errorf("too_many_stages", sheet.Index, "",
						"sheet %d needs more than the %d allowed guillotine cut stage(s)",
						sheet.Index+1, p.Rules.MaxCutStages))
				}
			}
		}
	}

	for _, part := range p.Parts {
		count := placed[part.ID]
		limit := float64(part.Quantity) * (1 + p.Rules.OversAllowedPct)
		if float64(count) > limit {
			violations = append(violations, errorf("demand_exceeded", 0, part.Code,
				"produced %d pieces of %s but only %d were requested (overs %.0f%%)",
				count, part.Code, part.Quantity, p.Rules.OversAllowedPct*100))
		}
	}

	if len(s.Unplaced) == 0 && len(s.Sheets) > 0 {
		totalPlaced := 0
		for _, n := range placed {
			totalPlaced += n
		}
		totalRequested := 0
		for _, part := range p.Parts {
			totalRequested += part.Quantity
		}
		if totalPlaced < totalRequested {
			violations = append(violations, warnf("missing_unplaced_report", 0, "",
				"%d requested pieces are missing from both the layout and the unplaced report",
				totalRequested-totalPlaced))
		}
	}
	for _, u := range s.Unplaced {
		if u.Quantity > 0 {
			violations = append(violations, warnf("unplaced_parts", 0, u.PartCode,
				"%d x %s not placed: %s", u.Quantity, u.PartCode, u.Reason))
		}
	}
	return violations
}

func checkOrientation(p core.Problem, part core.Part, pl core.Placement) error {
	if part.Width <= 0 || part.Height <= 0 {
		return nil // 1d parts have no orientation
	}
	if pl.Rotated {
		if pl.W != part.Height || pl.H != part.Width {
			return fmt.Errorf("piece %s is marked rotated but its size does not match the part", part.Code)
		}
		if !p.Rules.AllowRotate || !part.AllowRotate {
			return fmt.Errorf("piece %s is rotated although rotation is not permitted", part.Code)
		}
		if part.Grain != core.GrainNone {
			return fmt.Errorf("piece %s is rotated although it requires grain direction %q", part.Code, part.Grain)
		}
		return nil
	}
	if pl.W != part.Width || pl.H != part.Height {
		return fmt.Errorf("piece %s size %.1f x %.1f mm does not match the part size %.1f x %.1f mm",
			part.Code, core.ToMM(pl.W), core.ToMM(pl.H), core.ToMM(part.Width), core.ToMM(part.Height))
	}
	return nil
}
