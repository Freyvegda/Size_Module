package lp

import (
	"math"
	"math/rand"
	"testing"
)

func TestSolveSingleConstraint(t *testing.T) {
	// minimise x1 + x2 subject to x1 + x2 >= 1, written as x1 + x2 - s = 1.
	A := [][]float64{{1, 1, -1}}
	b := []float64{1}
	c := []float64{1, 1, 0}

	res, err := Solve(A, b, c, Options{})
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if res.Status != StatusOptimal {
		t.Fatalf("expected optimal, got %s", res.Status)
	}
	if math.Abs(res.Objective-1) > 1e-9 {
		t.Fatalf("expected objective 1, got %v", res.Objective)
	}
	if math.Abs(res.Duals[0]-1) > 1e-9 {
		t.Fatalf("expected dual 1, got %v", res.Duals)
	}
}

func TestSolveCuttingStockMaster(t *testing.T) {
	// Patterns over two part types: (1,1), (2,0), (0,2); one sheet of each.
	A := [][]float64{
		{1, 2, 0},
		{1, 0, 2},
	}
	b := []float64{1, 1}
	c := []float64{1, 1, 1}

	res, err := Solve(A, b, c, Options{})
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if res.Status != StatusOptimal {
		t.Fatalf("expected optimal, got %s", res.Status)
	}
	if math.Abs(res.Objective-1) > 1e-9 {
		t.Fatalf("expected one sheet, got %v", res.Objective)
	}
	if math.Abs(res.DualObjective-res.Objective) > 1e-9 {
		t.Fatalf("strong duality violated: primal %v dual %v", res.Objective, res.DualObjective)
	}
	if violation := DualViolation(A, c, res.Duals); violation > 1e-9 {
		t.Fatalf("duals are not feasible: violation %v", violation)
	}
	if res.X[0] < 0.999 {
		t.Fatalf("expected pattern 1 to be used, got %v", res.X)
	}
}

func TestSolveInfeasible(t *testing.T) {
	// x3 has a negative coefficient and x >= 0, so nothing can satisfy the row.
	A := [][]float64{{0, 0, -1}}
	res, err := Solve(A, []float64{1}, []float64{0, 0, 0}, Options{})
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if res.Status != StatusInfeasible {
		t.Fatalf("expected infeasible, got %s", res.Status)
	}
}

func TestSolveUnbounded(t *testing.T) {
	// minimise -x1 subject to x1 - s = 0: x1 can grow forever with s = x1.
	A := [][]float64{{1, -1}}
	res, err := Solve(A, []float64{0}, []float64{-1, 0}, Options{})
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if res.Status != StatusUnbounded {
		t.Fatalf("expected unbounded, got %s", res.Status)
	}
}

func TestRejectsNegativeRHS(t *testing.T) {
	if _, err := Solve([][]float64{{1}}, []float64{-1}, []float64{1}, Options{}); err == nil {
		t.Fatal("expected an error for a negative right hand side")
	}
}

// TestRandomStrongDuality generates cutting-stock-shaped LPs (pattern columns
// plus surplus columns) and checks the three properties a trustworthy simplex
// must have: primal feasibility, dual feasibility and strong duality.
func TestRandomStrongDuality(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 60; trial++ {
		m := 2 + rng.Intn(4)
		patterns := 3 + rng.Intn(6)
		cols := patterns + m

		A := make([][]float64, m)
		for i := range A {
			A[i] = make([]float64, cols)
			for j := 0; j < patterns; j++ {
				A[i][j] = float64(rng.Intn(4))
			}
			A[i][patterns+i] = -1 // surplus: demand may be exceeded, never unmet
			// Guarantee the row can be covered.
			A[i][rng.Intn(patterns)] = float64(1 + rng.Intn(3))
		}
		b := make([]float64, m)
		for i := range b {
			b[i] = float64(1 + rng.Intn(5))
		}
		c := make([]float64, cols)
		for j := 0; j < patterns; j++ {
			c[j] = float64(1 + rng.Intn(3))
		}

		res, err := Solve(A, b, c, Options{})
		if err != nil {
			t.Fatalf("trial %d: solve: %v", trial, err)
		}
		if res.Status != StatusOptimal {
			t.Fatalf("trial %d: expected optimal, got %s", trial, res.Status)
		}

		for i := range A {
			sum := 0.0
			for j := range res.X {
				sum += A[i][j] * res.X[j]
			}
			if math.Abs(sum-b[i]) > 1e-6 {
				t.Fatalf("trial %d: row %d not satisfied: %v != %v", trial, i, sum, b[i])
			}
		}
		if violation := DualViolation(A, c, res.Duals); violation > 1e-6 {
			t.Fatalf("trial %d: dual violation %v", trial, violation)
		}
		if gap := math.Abs(res.Objective - res.DualObjective); gap > 1e-6*(1+math.Abs(res.Objective)) {
			t.Fatalf("trial %d: duality gap %v (primal %v, dual %v)", trial, gap, res.Objective, res.DualObjective)
		}
	}
}

func TestDualViolationFindsBadDuals(t *testing.T) {
	A := [][]float64{{1}, {2}}
	c := []float64{1}
	if violation := DualViolation(A, c, []float64{1, 1}); violation <= 0 {
		t.Fatal("expected a violation for y=(1,1) against 1*y1 + 2*y2 <= 1")
	}
	if violation := DualViolation(A, c, []float64{0, 0.5}); violation > 1e-12 {
		t.Fatal("expected no violation for a feasible dual point")
	}
}
