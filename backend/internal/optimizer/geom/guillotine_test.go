package geom

import (
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
)

func mm(v float64) core.Dim { return core.FromMM(v) }

func TestBuildCutTreeGrid(t *testing.T) {
	// A 4-piece 2x2 grid with kerf: guillotine-cuttable by construction.
	region := core.Rect{X: 0, Y: 0, W: mm(1000), H: mm(500)}
	rects := []core.Rect{
		{X: 0, Y: 0, W: mm(500), H: mm(250)},
		{X: mm(504), Y: 0, W: mm(496), H: mm(250)},
		{X: 0, Y: mm(254), W: mm(500), H: mm(246)},
		{X: mm(504), Y: mm(254), W: mm(496), H: mm(246)},
	}
	ids := []string{"a", "b", "c", "d"}
	tree, ok := BuildCutTree(region, rects, ids, mm(4), 0)
	if !ok {
		t.Fatal("expected a guillotine cut tree for a grid layout")
	}
	steps := Instructions(tree, mm(4))
	if len(steps) != 3 {
		t.Fatalf("expected 3 cuts for a 2x2 grid, got %d: %v", len(steps), steps)
	}
}

func TestBuildCutTreeRejectsPinwheel(t *testing.T) {
	// The classic pinwheel arrangement has no guillotine cut sequence.
	region := core.Rect{X: 0, Y: 0, W: mm(100), H: mm(100)}
	rects := []core.Rect{
		{X: 0, Y: 0, W: mm(60), H: mm(40)},
		{X: mm(60), Y: 0, W: mm(40), H: mm(60)},
		{X: mm(40), Y: mm(60), W: mm(60), H: mm(40)},
		{X: 0, Y: mm(40), W: mm(40), H: mm(60)},
	}
	ids := []string{"a", "b", "c", "d"}
	if _, ok := BuildCutTree(region, rects, ids, 0, 0); ok {
		t.Fatal("pinwheel layout must not be considered guillotine-cuttable")
	}
}

func TestSeparated(t *testing.T) {
	a := core.Rect{X: 0, Y: 0, W: mm(100), H: mm(100)}
	b := core.Rect{X: mm(103), Y: 0, W: mm(100), H: mm(100)}
	if Separated(a, b, mm(4)) {
		t.Fatal("3 mm apart is not enough for a 4 mm kerf")
	}
	b.X = mm(104)
	if !Separated(a, b, mm(4)) {
		t.Fatal("exactly 4 mm apart satisfies the kerf")
	}
}
