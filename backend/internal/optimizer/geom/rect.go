// Package geom contains the geometric primitives and algorithms shared by all
// solvers: rectangle arithmetic and guillotine cut-tree construction.
package geom

import "github.com/size-module/backend/internal/optimizer/core"

// Separated reports whether two rectangles honour a required gap (the kerf).
// Two pieces separated only by less than gap in every axis touch or overlap,
// which a saw cannot produce.
func Separated(a, b core.Rect, gap core.Dim) bool {
	return a.Right()+gap <= b.X ||
		b.Right()+gap <= a.X ||
		a.Bottom()+gap <= b.Y ||
		b.Bottom()+gap <= a.Y
}

// Intersects reports whether two rectangles overlap at all.
func Intersects(a, b core.Rect) bool {
	return a.X < b.Right() && b.X < a.Right() && a.Y < b.Bottom() && b.Y < a.Bottom()
}

// Union returns the bounding box of two rectangles.
func Union(a, b core.Rect) core.Rect {
	x := min(a.X, b.X)
	y := min(a.Y, b.Y)
	r := max(a.Right(), b.Right())
	bt := max(a.Bottom(), b.Bottom())
	return core.Rect{X: x, Y: y, W: r - x, H: bt - y}
}

// SplitV cuts r at absolute x position c, consuming kerf between the halves.
// The second rectangle may be degenerate if the cut leaves no room.
func SplitV(r core.Rect, c, kerf core.Dim) (left, right core.Rect) {
	left = core.Rect{X: r.X, Y: r.Y, W: c - r.X, H: r.H}
	right = core.Rect{X: c + kerf, Y: r.Y, W: r.Right() - (c + kerf), H: r.H}
	return left, right
}

// SplitH cuts r at absolute y position c, consuming kerf between the halves.
func SplitH(r core.Rect, c, kerf core.Dim) (top, bottom core.Rect) {
	top = core.Rect{X: r.X, Y: r.Y, W: r.W, H: c - r.Y}
	bottom = core.Rect{X: r.X, Y: c + kerf, W: r.W, H: r.Bottom() - (c + kerf)}
	return top, bottom
}

// UsableArea returns the area of r after applying an edge trim.
func UsableArea(r core.Rect, trim core.Dim) core.Dim {
	in := r.Inset(trim)
	if in.W <= 0 || in.H <= 0 {
		return 0
	}
	return in.Area()
}
