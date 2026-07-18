package minisudoku

// This file holds an INDEPENDENT second solver used only to cross-validate the
// primary complete solver (solver.go) and the logic solver (logicsolve.go).
// It was written from the spec (docs/plan/games/mini-sudoku.md), NOT by reading
// solver.go, and is deliberately structured differently:
//
//   - The primary solver fills cells in fixed row-major order and prunes with
//     incrementally-maintained row/col/box bitmasks.
//   - This solver selects the next cell by the minimum-remaining-values (MRV)
//     heuristic and computes candidates by directly SCANNING the current board
//     (peers in the same row, column, and box) every time.
//   - Box membership is derived here from first principles (a boxH-row ×
//     boxW-column partition) rather than through the shared boxID helper, so a
//     shared box-geometry mistake would surface as a disagreement rather than
//     being silently mirrored.
//
// If the two solvers ever disagree on a count or on the unique solution, one of
// them (and possibly the generator that trusts the primary) has a bug.

// xchecker is the independent search state.
type xchecker struct {
	n     int
	cells []int
	// boxCells[b] lists the cell indices belonging to box b, built once from
	// the boxH×boxW geometry.
	boxCells [][]int
	boxOf    []int
}

// xcheckBoxIndex computes, from first principles, which box cell (row,col)
// belongs to for an n×n grid partitioned into boxH-row × boxW-column boxes.
// Boxes are numbered band-major: band = row/boxH (top to bottom), stack =
// col/boxW (left to right), index = band*(n/boxW) + stack. For the default
// 6×6 / 2×3 game this yields six boxes, each 2 rows tall and 3 columns wide.
func xcheckBoxIndex(row, col, boxH, boxW, n int) int {
	stacksPerBand := n / boxW
	band := row / boxH
	stack := col / boxW
	return band*stacksPerBand + stack
}

// newXchecker builds the independent search state for p, seeding it with the
// givens. ok is false if the givens themselves are inconsistent (a duplicate in
// some row/col/box, or an out-of-range digit) — meaning the puzzle has no
// solution.
func newXchecker(p Puzzle) (*xchecker, bool) {
	n, boxH, boxW := p.N, p.BoxH, p.BoxW
	if n == 0 {
		n, boxH, boxW = N, BoxH, BoxW
	}
	x := &xchecker{
		n:        n,
		cells:    make([]int, n*n),
		boxCells: make([][]int, n),
		boxOf:    make([]int, n*n),
	}
	for i := 0; i < n*n; i++ {
		row, col := i/n, i%n
		b := xcheckBoxIndex(row, col, boxH, boxW, n)
		x.boxOf[i] = b
		x.boxCells[b] = append(x.boxCells[b], i)
	}
	for idx, val := range p.Givens {
		if idx < 0 || idx >= n*n || val < 1 || val > n {
			return x, false
		}
		if !x.legal(idx, val) {
			return x, false
		}
		x.cells[idx] = val
	}
	return x, true
}

// legal reports whether placing val at cell pos conflicts with any digit
// already present in pos's row, column, or box. Peers are scanned directly from
// the board — no incremental masks — so this check shares no state with the
// primary solver.
func (x *xchecker) legal(pos, val int) bool {
	n := x.n
	row, col := pos/n, pos%n
	for c := 0; c < n; c++ {
		if x.cells[row*n+c] == val {
			return false
		}
		if x.cells[c*n+col] == val {
			return false
		}
	}
	for _, idx := range x.boxCells[x.boxOf[pos]] {
		if x.cells[idx] == val {
			return false
		}
	}
	return true
}

// candidatesAt returns every digit 1..n that could legally occupy empty cell
// pos given the current board, computed by scanning pos's peers.
func (x *xchecker) candidatesAt(pos int) []int {
	n := x.n
	row, col := pos/n, pos%n
	used := make([]bool, n+1)
	for c := 0; c < n; c++ {
		used[x.cells[row*n+c]] = true
		used[x.cells[c*n+col]] = true
	}
	for _, idx := range x.boxCells[x.boxOf[pos]] {
		used[x.cells[idx]] = true
	}
	cands := make([]int, 0, n)
	for d := 1; d <= n; d++ {
		if !used[d] {
			cands = append(cands, d)
		}
	}
	return cands
}

// search counts solutions of the current board up to capN, invoking the DFS
// with MRV cell selection. When collect is non-nil and still empty, the first
// full solution found is copied into it. It returns as soon as the running
// count reaches capN.
func (x *xchecker) search(count *int, capN int, collect *[]int) {
	if *count >= capN {
		return
	}
	// Pick the empty cell with the fewest candidates (MRV). A cell with zero
	// candidates is a dead end; a cell with one candidate is optimal.
	best := -1
	var bestCands []int
	for i := 0; i < x.n*x.n; i++ {
		if x.cells[i] != 0 {
			continue
		}
		cands := x.candidatesAt(i)
		if len(cands) == 0 {
			return // contradiction: this branch has no solution
		}
		if best == -1 || len(cands) < len(bestCands) {
			best = i
			bestCands = cands
			if len(cands) == 1 {
				break
			}
		}
	}
	if best == -1 {
		// No empty cells remain: the board is a complete solution.
		*count++
		if collect != nil && *collect == nil {
			*collect = append([]int(nil), x.cells...)
		}
		return
	}
	for _, d := range bestCands {
		x.cells[best] = d
		x.search(count, capN, collect)
		x.cells[best] = 0
		if *count >= capN {
			return
		}
	}
}

// xcheckCount returns min(#solutions, capN) for p using the independent solver.
func xcheckCount(p Puzzle, capN int) int {
	if capN <= 0 {
		return 0
	}
	x, ok := newXchecker(p)
	if !ok {
		return 0
	}
	count := 0
	x.search(&count, capN, nil)
	return count
}

// xcheckSolve returns one solution for p using the independent solver, or ok ==
// false if none exists.
func xcheckSolve(p Puzzle) (Solution, bool) {
	x, ok := newXchecker(p)
	if !ok {
		return Solution{}, false
	}
	count := 0
	var collected []int
	x.search(&count, 1, &collected)
	if count == 0 {
		return Solution{}, false
	}
	return Solution{Cells: collected}, true
}
