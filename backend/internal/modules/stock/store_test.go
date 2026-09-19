package stock

import (
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
)

func TestValidateDefects(t *testing.T) {
	width, height := core.FromMM(1000), core.FromMM(500)
	ok := []core.Rect{{X: core.FromMM(100), Y: core.FromMM(50), W: core.FromMM(200), H: core.FromMM(200)}}
	if err := ValidateDefects(ok, width, height); err != nil {
		t.Fatalf("well-formed defect rejected: %v", err)
	}

	cases := map[string][]core.Rect{
		"zero width":    {{X: 0, Y: 0, W: 0, H: core.FromMM(100)}},
		"negative":      {{X: -1, Y: 0, W: core.FromMM(100), H: core.FromMM(100)}},
		"out of right":  {{X: core.FromMM(900), Y: 0, W: core.FromMM(200), H: core.FromMM(100)}},
		"out of bottom": {{X: 0, Y: core.FromMM(450), W: core.FromMM(100), H: core.FromMM(100)}},
	}
	for name, defects := range cases {
		if err := ValidateDefects(defects, width, height); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
}

func TestValidateDefectsWithoutKnownBounds(t *testing.T) {
	// When dimensions are inherited from a format, the handler only checks the
	// shape and the store enforces the bounds once the size is known.
	shaped := []core.Rect{{X: core.FromMM(10), Y: core.FromMM(10), W: core.FromMM(50), H: core.FromMM(50)}}
	if err := ValidateDefects(shaped, 0, 0); err != nil {
		t.Fatalf("shape-only validation rejected a valid rectangle: %v", err)
	}
}
