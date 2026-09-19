package geom

import (
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
)

func TestBuildCutTreeStages(t *testing.T) {
	// A 2x2 grid of quarters is one rip plus a crosscut in each column: exactly
	// 2 guillotine stages, which is the classic shelf layout.
	region := core.Rect{X: 0, Y: 0, W: core.FromMM(1000), H: core.FromMM(1000)}
	half := core.FromMM(500)
	rects := []core.Rect{
		{X: 0, Y: 0, W: half, H: half},
		{X: 0, Y: half, W: half, H: half},
		{X: half, Y: 0, W: half, H: half},
		{X: half, Y: half, W: half, H: half},
	}
	ids := []string{"a", "b", "c", "d"}

	tree, ok := BuildCutTreeStages(region, rects, ids, 0, 0, 2)
	if !ok {
		t.Fatal("a 2-stage grid should be cuttable within 2 stages")
	}
	if stages := CutTreeStages(tree); stages != 2 {
		t.Fatalf("expected 2 stages, got %d", stages)
	}

	if _, ok := BuildCutTreeStages(region, rects, ids, 0, 0, 1); ok {
		t.Fatal("a 2-stage grid must not be cuttable within 1 stage")
	}
	if _, ok := BuildCutTree(region, rects, ids, 0, 0); !ok {
		t.Fatal("the unlimited builder must still accept the grid")
	}
}
