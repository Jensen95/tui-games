package queens

import "github.com/Jensen95/tui-games/internal/engine"

// CrossSolver is a SECOND, independently-structured complete solver used
// only to cross-check Solver (solver.go). docs/plan/games/queens.md's
// generation invariant rests on "two independently written solvers agree";
// solver.go's search recurses row-by-row (one queen per row, tracked with
// bool slices) and prunes adjacency by checking only the immediately
// preceding row — a valid optimization since queens two-or-more rows apart
// can never be chebyshev-adjacent, but an assumption this file deliberately
// does NOT share, so a bug in that assumption (or any shared adjacency bug)
// would surface as a disagreement rather than being silently confirmed by
// both solvers making the same mistake.
//
// Structural differences from solver.go, by design:
//   - Recurses over COLUMNS (assigns each column a row), not rows.
//   - Occupancy (rows, regions) is tracked with bitmasks, not []bool.
//   - Adjacency is re-verified by a full pairwise chebyshev-distance check
//     against every previously placed queen, not just the adjacent line.
//
// This file is read-only re-derivation of the spec; it does not read or
// mirror solver.go's internal helpers.
type CrossSolver struct{}

// NewCrossSolver returns a Queens cross-check solver.
func NewCrossSolver() *CrossSolver { return &CrossSolver{} }

// crossForcedRows returns, for each column, the row a given forces that
// column's queen to (or -1 if the column is unconstrained by a given).
func crossForcedRows(p Puzzle) []int {
	forced := make([]int, p.N)
	for i := range forced {
		forced[i] = -1
	}
	for _, g := range p.Givens {
		c := engine.CellAt(g, p.N)
		if c.Col >= 0 && c.Col < p.N {
			forced[c.Col] = c.Row
		}
	}
	return forced
}

// crossChebyshevAdjacent reports whether (r1,c1) and (r2,c2) are 8-neighbor
// adjacent (chebyshev distance exactly 1). Computed independently of
// validator.go's adjacent() — duplicated intentionally so a shared bug in
// the adjacency formula isn't hidden by reusing the same code.
func crossChebyshevAdjacent(r1, c1, r2, c2 int) bool {
	dr := r1 - r2
	if dr < 0 {
		dr = -dr
	}
	dc := c1 - c2
	if dc < 0 {
		dc = -dc
	}
	if dr == 0 && dc == 0 {
		return false
	}
	return dr <= 1 && dc <= 1
}

// crossSearch performs the independent column-major backtracking search. It
// stops once cap solutions are found and returns the count (capped). If
// collect is non-nil, the first solution found is written there (converted
// to the row-indexed Solution shape).
func crossSearch(p Puzzle, cap int, collect *Solution) int {
	n := p.N
	if n <= 0 || cap <= 0 {
		return 0
	}
	forced := crossForcedRows(p)
	queenRow := make([]int, n) // queenRow[col] = row, -1 while unfilled
	for i := range queenRow {
		queenRow[i] = -1
	}
	var rowMask uint32
	var regionMask uint32
	count := 0
	found := false

	var rec func(col int)
	rec = func(col int) {
		if count >= cap {
			return
		}
		if col == n {
			count++
			if !found && collect != nil {
				sol := Solution{N: n, QueenAt: make([]int, n)}
				for c, r := range queenRow {
					sol.QueenAt[r] = c
				}
				*collect = sol
				found = true
			}
			return
		}
		for row := 0; row < n; row++ {
			if forced[col] >= 0 && row != forced[col] {
				continue
			}
			bit := uint32(1) << uint(row)
			if rowMask&bit != 0 {
				continue
			}
			reg := p.Region[row*n+col]
			rbit := uint32(1) << uint(reg)
			if regionMask&rbit != 0 {
				continue
			}
			ok := true
			for c2 := 0; c2 < col; c2++ {
				if crossChebyshevAdjacent(row, col, queenRow[c2], c2) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			queenRow[col] = row
			rowMask |= bit
			regionMask |= rbit
			rec(col + 1)
			rowMask &^= bit
			regionMask &^= rbit
			queenRow[col] = -1
			if count >= cap {
				return
			}
		}
	}
	rec(0)
	return count
}

// CountSolutions returns min(#solutions, cap), found via the independent
// column-major search.
func (c *CrossSolver) CountSolutions(p Puzzle, cap int) int {
	return crossSearch(p, cap, nil)
}

// Solve returns one full solution to p, if any exists.
func (c *CrossSolver) Solve(p Puzzle) (Solution, bool) {
	var sol Solution
	if crossSearch(p, 1, &sol) == 0 {
		return Solution{}, false
	}
	return sol, true
}
