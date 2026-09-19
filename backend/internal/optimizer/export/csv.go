package export

import (
	"encoding/csv"
	"fmt"
	"io"

	"github.com/size-module/backend/internal/optimizer/core"
)

// CSV writes a cut list: one section per sheet, with the pieces, the ordered
// cut steps and the reusable leftovers. Columns are stable so the file can be
// pasted into spreadsheets or shop-floor tools.
//
//	sheet,label,stock,kind,seq,part,x_mm,y_mm,w_mm,h_mm,rotated,locked,step
//
// kind is "sheet", "piece", "cut" or "offcut". Lengths are millimetres with
// 0.01 mm precision; x/y are the top-left corner for pieces and leftovers.
func CSV(w io.Writer, sol core.Solution, opts Options) error {
	out := csv.NewWriter(w)
	if err := out.Write([]string{
		"sheet", "label", "stock", "kind", "seq", "part",
		"x_mm", "y_mm", "w_mm", "h_mm", "rotated", "locked", "step",
	}); err != nil {
		return err
	}

	for _, sheet := range sheetsOf(sol, opts) {
		sheetNo := itoa(core.Dim(sheet.Index + 1))
		writeRow := func(row []string) error { return out.Write(row) }

		if err := writeRow([]string{
			sheetNo, sheet.Label, sheet.StockCode, "sheet", "",
			"", "", "", mm(sheet.Width), mm(sheet.Height), "", "", "",
		}); err != nil {
			return err
		}
		for i, pl := range sheet.Placements {
			rotated := ""
			if pl.Rotated {
				rotated = "true"
			}
			locked := ""
			if pl.Locked {
				locked = "true"
			}
			if err := writeRow([]string{
				sheetNo, sheet.Label, sheet.StockCode, "piece", itoa(core.Dim(i + 1)),
				pl.PartCode, mm(pl.X), mm(pl.Y), mm(pl.W), mm(pl.H), rotated, locked, "",
			}); err != nil {
				return err
			}
		}
		for i, step := range sheet.CutSteps {
			if err := writeRow([]string{
				sheetNo, sheet.Label, sheet.StockCode, "cut", itoa(core.Dim(i + 1)),
				"", "", "", "", "", "", "", step,
			}); err != nil {
				return err
			}
		}
		for i, off := range sheet.Offcuts {
			if err := writeRow([]string{
				sheetNo, sheet.Label, sheet.StockCode, "offcut", itoa(core.Dim(i + 1)),
				"", mm(off.X), mm(off.Y), mm(off.W), mm(off.H), "", "", "",
			}); err != nil {
				return err
			}
		}
	}

	for _, u := range sol.Unplaced {
		if err := out.Write([]string{
			"", "", "", "unplaced", "", u.PartCode,
			"", "", "", "", "", "", fmt.Sprintf("%d x %s: %s", u.Quantity, u.PartCode, u.Reason),
		}); err != nil {
			return err
		}
	}

	out.Flush()
	return out.Error()
}
