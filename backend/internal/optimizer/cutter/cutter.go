// Package cutter turns a layout into a cutting sequence. For guillotine
// machines it reconstructs the cut tree first, which also proves the layout is
// physically producible.
package cutter

import (
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/geom"
)

// ForSheet returns the ordered cut instructions for one sheet. The second
// return value is false when no guillotine cut sequence exists.
func ForSheet(sheet core.SheetPlan, kerf core.Dim) ([]string, bool) {
	if len(sheet.Placements) == 0 {
		return nil, true
	}
	rects := make([]core.Rect, len(sheet.Placements))
	ids := make([]string, len(sheet.Placements))
	for i, pl := range sheet.Placements {
		rects[i] = core.Rect{X: pl.X, Y: pl.Y, W: pl.W, H: pl.H}
		ids[i] = pl.PartCode
	}
	region := core.Rect{X: 0, Y: 0, W: sheet.Width, H: sheet.Height}
	tree, ok := geom.BuildCutTree(region, rects, ids, kerf, 0)
	if !ok {
		return nil, false
	}
	return geom.Instructions(tree, kerf), true
}
