// Package lp solves small dense linear programs in the standard equality form
//
//	minimise c·x   subject to   A x = b,   x >= 0
//
// It exists for the cutting-stock column generation loop: the restricted master
// problem has few rows (one per part type) and a growing number of columns
// (patterns), which a dense tableau simplex handles comfortably. It is not a
// general purpose numerical LP solver.
//
// Duals are read straight from the objective row of the artificial columns, a
// standard tableau trick: the reduced cost of artificial column e_i is
// c_i - y·e_i = -y_i, so y_i = -r_i.
package lp

import (
	"errors"
	"fmt"
	"math"
)

type Status string

const (
	StatusOptimal        Status = "optimal"
	StatusInfeasible     Status = "infeasible"
	StatusUnbounded      Status = "unbounded"
	StatusIterationLimit Status = "iteration_limit"
)

type Options struct {
	MaxIterations int
	Tolerance     float64
}

type Result struct {
	Status Status
	// Objective is c·x at the returned point.
	Objective float64
	// DualObjective is b·y; it equals Objective at optimality.
	DualObjective float64
	// X holds one value per column of A.
	X []float64
	// Duals holds one value per row of A (y with y^T A <= c).
	Duals []float64
	Iterations int
}

// Solve runs the two-phase simplex. A is the m x n constraint matrix, b the
// right hand side (must be non-negative; negate the row otherwise) and c the
// cost vector.
func Solve(A [][]float64, b, c []float64, opts Options) (Result, error) {
	m := len(A)
	if m == 0 {
		return Result{}, errors.New("lp: matrix has no rows")
	}
	n := len(A[0])
	if len(b) != m {
		return Result{}, fmt.Errorf("lp: len(b)=%d but matrix has %d rows", len(b), m)
	}
	if len(c) != n {
		return Result{}, fmt.Errorf("lp: len(c)=%d but matrix has %d columns", len(c), n)
	}
	for i, row := range A {
		if len(row) != n {
			return Result{}, fmt.Errorf("lp: row %d has %d entries, expected %d", i, len(row), n)
		}
		for j, v := range row {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return Result{}, fmt.Errorf("lp: matrix entry [%d][%d] is not finite", i, j)
			}
		}
	}
	for i, v := range b {
		if v < 0 {
			return Result{}, fmt.Errorf("lp: b[%d] is negative; the caller must orient rows", i)
		}
	}

	tol := opts.Tolerance
	if tol <= 0 {
		tol = 1e-9
	}
	maxIterations := opts.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 20000
	}

	// Tableau columns: [0..n) original, [n..n+m) artificial, [n+m] right hand side.
	width := n + m + 1
	tab := make([][]float64, m)
	for i := range tab {
		row := make([]float64, width)
		copy(row, A[i])
		row[n+i] = 1
		row[n+m] = b[i]
		tab[i] = row
	}

	// Phase 1: minimise the artificial sum. Reduced costs for the original
	// columns are (0 - column sum); the artificials form the initial basis.
	phase1 := make([]float64, width)
	phase2 := make([]float64, width)
	sumB := 0.0
	for i := 0; i < m; i++ {
		sumB += b[i]
	}
	for j := 0; j < n; j++ {
		columnSum := 0.0
		for i := 0; i < m; i++ {
			columnSum += tab[i][j]
		}
		phase1[j] = -columnSum
		phase2[j] = c[j]
	}
	phase1[n+m] = -sumB // objective row keeps -z
	phase2[n+m] = 0

	basis := make([]int, m)
	for i := 0; i < m; i++ {
		basis[i] = n + i
	}

	objectives := [][]float64{phase1, phase2}
	iterations := 0

	// ---------------------------------------------------------------- phase 1
	for iterations < maxIterations {
		enter := blandEnter(phase1, n+m, tol)
		if enter < 0 {
			break
		}
		leave, ok := ratioTest(tab, basis, enter, m, tol)
		if !ok {
			return Result{Status: StatusUnbounded}, nil
		}
		pivot(tab, objectives, leave, enter)
		basis[leave] = enter
		iterations++
	}

	if artificialSum := -phase1[n+m]; artificialSum > tol*float64(1+m) {
		return Result{Status: StatusInfeasible, Iterations: iterations}, nil
	}

	// ---------------------------------------------------------------- phase 2
	limitHit := true
	for iterations < maxIterations {
		enter := blandEnter(phase2, n, tol) // artificials may not enter
		if enter < 0 {
			limitHit = false
			break
		}
		leave, ok := ratioTest(tab, basis, enter, m, tol)
		if !ok {
			return Result{Status: StatusUnbounded, Iterations: iterations}, nil
		}
		pivot(tab, objectives, leave, enter)
		basis[leave] = enter
		iterations++
	}
	if limitHit {
		return Result{Status: StatusIterationLimit, Iterations: iterations}, nil
	}

	x := make([]float64, n)
	for i, col := range basis {
		if col < n {
			x[col] = tab[i][n+m]
			if x[col] < 0 && x[col] > -tol {
				x[col] = 0 // clean tiny negative values from pivoting
			}
		}
	}
	duals := make([]float64, m)
	dualObjective := 0.0
	for i := 0; i < m; i++ {
		duals[i] = -phase2[n+i]
		dualObjective += b[i] * duals[i]
	}

	return Result{
		Status:        StatusOptimal,
		Objective:     -phase2[n+m],
		DualObjective: dualObjective,
		X:             x,
		Duals:         duals,
		Iterations:    iterations,
	}, nil
}

// DualViolation returns the largest violation of y^T A_j <= c_j over all
// columns. It is the caller's safety net: at optimality it is zero up to
// numerical noise, and a large value means the duals must not be trusted.
func DualViolation(A [][]float64, c, y []float64) float64 {
	m := len(A)
	if m == 0 {
		return 0
	}
	worst := 0.0
	for j := range A[0] {
		lhs := 0.0
		for i := 0; i < m; i++ {
			lhs += A[i][j] * y[i]
		}
		if v := lhs - c[j]; v > worst {
			worst = v
		}
	}
	return worst
}

// blandEnter picks the smallest index with a negative reduced cost. Bland's
// rule is slower than Dantzig's but cannot cycle, which matters more here than
// speed: a cycling solver would hang the request.
func blandEnter(costs []float64, limit int, tol float64) int {
	for j := 0; j < limit; j++ {
		if costs[j] < -tol {
			return j
		}
	}
	return -1
}

// ratioTest finds the leaving row for an entering column. Ties are broken by
// the basic variable index (Bland), which together with the entering rule
// guarantees termination.
func ratioTest(tab [][]float64, basis []int, enter, m int, tol float64) (int, bool) {
	leave := -1
	best := math.Inf(1)
	rhs := len(tab[0]) - 1
	for i := 0; i < m; i++ {
		a := tab[i][enter]
		if a <= tol {
			continue
		}
		ratio := tab[i][rhs] / a
		switch {
		case ratio < best-tol:
			best = ratio
			leave = i
		case math.Abs(ratio-best) <= tol && leave >= 0 && basis[i] < basis[leave]:
			leave = i
		}
	}
	if leave < 0 {
		return 0, false
	}
	return leave, true
}

// pivot performs Gauss-Jordan elimination on the pivot row and updates every
// objective row in lockstep.
func pivot(tab [][]float64, objectives [][]float64, leave, enter int) {
	row := tab[leave]
	p := row[enter]
	inv := 1 / p
	for j := range row {
		row[j] *= inv
	}
	for i := range tab {
		if i == leave {
			continue
		}
		factor := tab[i][enter]
		if factor == 0 {
			continue
		}
		for j := range row {
			tab[i][j] -= factor * row[j]
		}
	}
	for _, obj := range objectives {
		factor := obj[enter]
		if factor == 0 {
			continue
		}
		for j := range row {
			obj[j] -= factor * row[j]
		}
	}
}
