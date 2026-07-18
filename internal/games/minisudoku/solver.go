package minisudoku

import (
	"github.com/Jensen95/tui-games/internal/engine"
)

// Solver implements engine.Solver[Puzzle, Solution] via a complete
// backtracking search over row/column/box candidate bitmasks. It is the
// ground truth for existence and uniqueness; LogicSolve (logicsolve.go) is
// the independent no-guess oracle used for difficulty labeling and hints.
// See docs/plan/games/mini-sudoku.md "Solver approach".
type Solver struct{}

var _ engine.Solver[Puzzle, Solution] = Solver{}

// solveState holds the mutable search state for the backtracking solver.
type solveState struct {
	n, boxH, boxW int
	cells         []int
	rowMask       []uint16
	colMask       []uint16
	boxMask       []uint16
}

// newSolveState builds the initial (givens-only) search state for p. ok is
// false if the givens themselves already contain a duplicate (no solution
// possible).
func newSolveState(p Puzzle) (*solveState, bool) {
	n, boxH, boxW := p.N, p.BoxH, p.BoxW
	if n == 0 {
		n, boxH, boxW = N, BoxH, BoxW
	}
	st := &solveState{
		n: n, boxH: boxH, boxW: boxW,
		cells:   make([]int, n*n),
		rowMask: make([]uint16, n),
		colMask: make([]uint16, n),
		boxMask: make([]uint16, n),
	}
	for idx, val := range p.Givens {
		row, col := idx/n, idx%n
		box := boxID(row, col, boxH, boxW, n)
		bit := uint16(1) << uint(val-1)
		if st.rowMask[row]&bit != 0 || st.colMask[col]&bit != 0 || st.boxMask[box]&bit != 0 {
			return st, false
		}
		st.cells[idx] = val
		st.rowMask[row] |= bit
		st.colMask[col] |= bit
		st.boxMask[box] |= bit
	}
	return st, true
}

// place sets cells[pos] = digit and updates the masks.
func (st *solveState) place(pos, row, col, box, digit int) {
	bit := uint16(1) << uint(digit-1)
	st.cells[pos] = digit
	st.rowMask[row] |= bit
	st.colMask[col] |= bit
	st.boxMask[box] |= bit
}

// unplace clears cells[pos] and its mask bits.
func (st *solveState) unplace(pos, row, col, box, digit int) {
	bit := uint16(1) << uint(digit-1)
	st.cells[pos] = 0
	st.rowMask[row] &^= bit
	st.colMask[col] &^= bit
	st.boxMask[box] &^= bit
}

// backtrack fills st.cells[pos:] in row-major order, skipping already-filled
// (given) cells. onSolution is invoked for every complete assignment found;
// once it returns true, the whole search stops early (used by
// CountSolutions to respect its cap without exploring further; Solve passes
// a callback that always returns true after copying the first solution).
func (st *solveState) backtrack(pos int, onSolution func() bool) bool {
	n := st.n
	if pos == n*n {
		return onSolution()
	}
	if st.cells[pos] != 0 {
		return st.backtrack(pos+1, onSolution)
	}
	row, col := pos/n, pos%n
	box := boxID(row, col, st.boxH, st.boxW, n)
	used := st.rowMask[row] | st.colMask[col] | st.boxMask[box]
	for d := 1; d <= n; d++ {
		bit := uint16(1) << uint(d-1)
		if used&bit != 0 {
			continue
		}
		st.place(pos, row, col, box, d)
		if st.backtrack(pos+1, onSolution) {
			st.unplace(pos, row, col, box, d)
			return true
		}
		st.unplace(pos, row, col, box, d)
	}
	return false
}

// Solve returns one solution via backtracking search, if any exists.
func (s Solver) Solve(p Puzzle) (Solution, bool) {
	st, ok := newSolveState(p)
	if !ok {
		return Solution{}, false
	}
	var found []int
	ok = st.backtrack(0, func() bool {
		found = append([]int(nil), st.cells...)
		return true
	})
	if !ok {
		return Solution{}, false
	}
	return Solution{Cells: found}, true
}

// CountSolutions returns min(#solutions, capN); uniqueness is
// capN>=2 && result==1.
func (s Solver) CountSolutions(p Puzzle, capN int) int {
	if capN <= 0 {
		return 0
	}
	st, ok := newSolveState(p)
	if !ok {
		return 0
	}
	count := 0
	st.backtrack(0, func() bool {
		count++
		return count >= capN
	})
	return count
}
