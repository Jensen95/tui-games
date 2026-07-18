package queens

import "github.com/Jensen95/tui-games/internal/engine"

// Solver is the Queens complete + logic solver. See engine.Solver[Puzzle, Solution].
type Solver struct{}

// NewSolver returns a Queens solver.
func NewSolver() *Solver { return &Solver{} }

var _ engine.Solver[Puzzle, Solution] = (*Solver)(nil)

// forcedCols reads any givens off p into a per-row forced column (or -1).
func forcedCols(p Puzzle) []int {
	forced := make([]int, p.N)
	for i := range forced {
		forced[i] = -1
	}
	for _, g := range p.Givens {
		c := engine.CellAt(g, p.N)
		if c.Row >= 0 && c.Row < p.N {
			forced[c.Row] = c.Col
		}
	}
	return forced
}

// search runs the complete backtracking solver over rows (one queen per row).
// It stops once cap solutions have been found. If collect is non-nil the first
// solution is stored there. It returns the number of solutions, capped at cap.
//
// Adjacency only needs checking against the previous row: queens two or more
// rows apart have chebyshev distance >= 2 and can never touch.
func search(p Puzzle, cap int, collect *Solution) int {
	n := p.N
	forced := forcedCols(p)
	colUsed := make([]bool, n)
	regionUsed := make([]bool, n)
	placed := make([]int, n)
	count := 0
	found := false

	var rec func(row int)
	rec = func(row int) {
		if count >= cap {
			return
		}
		if row == n {
			count++
			if !found && collect != nil {
				*collect = Solution{N: n, QueenAt: append([]int(nil), placed...)}
				found = true
			}
			return
		}
		for col := 0; col < n; col++ {
			if forced[row] >= 0 && col != forced[row] {
				continue
			}
			if colUsed[col] {
				continue
			}
			reg := p.Region[row*n+col]
			if regionUsed[reg] {
				continue
			}
			if row > 0 && absInt(placed[row-1]-col) <= 1 {
				continue
			}
			placed[row] = col
			colUsed[col] = true
			regionUsed[reg] = true
			rec(row + 1)
			colUsed[col] = false
			regionUsed[reg] = false
			if count >= cap {
				return
			}
		}
	}
	rec(0)
	return count
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Solve returns one solution to p, if any exists.
func (s *Solver) Solve(p Puzzle) (Solution, bool) {
	var sol Solution
	if search(p, 1, &sol) == 0 {
		return Solution{}, false
	}
	return sol, true
}

// CountSolutions returns min(#solutions, cap). Ground truth for uniqueness:
// callers use CountSolutions(p, 2) == 1 to certify a puzzle is unambiguous.
func (s *Solver) CountSolutions(p Puzzle, cap int) int {
	if cap <= 0 {
		return 0
	}
	return search(p, cap, nil)
}

// LogicSolve attempts a no-guess solve via the deduction ladder in
// docs/plan/games/queens.md ("Deduction ladder"). It returns the solution (if
// closed), whether the board fully closed without guessing, and the deepest
// technique required.
func (s *Solver) LogicSolve(p Puzzle) (Solution, bool, engine.Technique) {
	st := newLogicState(p)
	// Seed givens.
	for _, g := range p.Givens {
		c := engine.CellAt(g, p.N)
		if !st.place(c.Row, c.Col) {
			return Solution{}, false, TechniqueNone
		}
	}
	closed := st.run()
	if !closed {
		return Solution{}, false, st.deepest
	}
	return st.solution(), true, st.deepest
}

// logicState holds the candidate grid the deduction ladder narrows to fixpoint.
type logicState struct {
	n       int
	region  []int
	cand    []bool // n*n: cell may still hold a queen
	placed  []int  // per row: chosen col, or -1
	rowDone []bool
	colDone []bool
	regDone []bool
	deepest engine.Technique
	failed  bool
}

func newLogicState(p Puzzle) *logicState {
	n := p.N
	st := &logicState{
		n:       n,
		region:  p.Region,
		cand:    make([]bool, n*n),
		placed:  make([]int, n),
		rowDone: make([]bool, n),
		colDone: make([]bool, n),
		regDone: make([]bool, n),
		deepest: TechniqueNone,
	}
	for i := range st.cand {
		st.cand[i] = true
	}
	for i := range st.placed {
		st.placed[i] = -1
	}
	return st
}

func (st *logicState) clone() *logicState {
	c := &logicState{
		n:       st.n,
		region:  st.region,
		cand:    append([]bool(nil), st.cand...),
		placed:  append([]int(nil), st.placed...),
		rowDone: append([]bool(nil), st.rowDone...),
		colDone: append([]bool(nil), st.colDone...),
		regDone: append([]bool(nil), st.regDone...),
		deepest: st.deepest,
	}
	return c
}

func (st *logicState) deepen(t engine.Technique) {
	if techRank(t) > techRank(st.deepest) {
		st.deepest = t
	}
}

func techRank(t engine.Technique) int {
	switch t {
	case TechniqueSingleton:
		return 1
	case TechniqueElimination:
		return 2
	case TechniqueRegionLineLock:
		return 3
	case TechniqueSetLocking:
		return 4
	case TechniqueAdjacencyExclusion:
		return 5
	default:
		return 0
	}
}

// place puts a queen at (row,col) and eliminates every cell it rules out:
// its row, its column, its region, and its 8 neighbors. Returns false on an
// immediate contradiction (a cell that was required is now impossible).
func (st *logicState) place(row, col int) bool {
	n := st.n
	if st.rowDone[row] {
		return st.placed[row] == col
	}
	reg := st.region[row*n+col]
	st.placed[row] = col
	st.rowDone[row] = true
	st.colDone[col] = true
	st.regDone[reg] = true
	// The placed cell is no longer a free candidate.
	for c := 0; c < n; c++ {
		st.cand[row*n+c] = false // same row
	}
	for rr := 0; rr < n; rr++ {
		st.cand[rr*n+col] = false // same col
	}
	for i := 0; i < n*n; i++ {
		if st.region[i] == reg {
			st.cand[i] = false // same region
		}
	}
	cell := engine.Cell{Row: row, Col: col}
	for _, nb := range engine.Neighbors8(cell, n, n) {
		st.cand[engine.Index(nb, n)] = false // 8-neighbors
	}
	return true
}

// run applies the deduction ladder to fixpoint. It returns true iff the board
// closed (a queen placed in every row) with no guessing.
func (st *logicState) run() bool {
	for {
		if st.solvedAll() {
			return true
		}
		if st.failed || st.hasContradiction() {
			return false
		}
		if st.singletons() {
			continue
		}
		if st.lineLocks() {
			continue
		}
		if st.setLocks() {
			continue
		}
		if st.contradictionElimination() {
			continue
		}
		return false // stuck: needs guessing
	}
}

func (st *logicState) solvedAll() bool {
	for _, d := range st.rowDone {
		if !d {
			return false
		}
	}
	return true
}

// hasContradiction reports whether some unsatisfied row/col/region has no
// remaining candidate cell.
func (st *logicState) hasContradiction() bool {
	n := st.n
	for r := 0; r < n; r++ {
		if st.rowDone[r] {
			continue
		}
		if st.rowCandCount(r) == 0 {
			return true
		}
	}
	for c := 0; c < n; c++ {
		if st.colDone[c] {
			continue
		}
		if st.colCandCount(c) == 0 {
			return true
		}
	}
	for reg := 0; reg < n; reg++ {
		if st.regDone[reg] {
			continue
		}
		if st.regionCandCount(reg) == 0 {
			return true
		}
	}
	return false
}

func (st *logicState) rowCandCount(r int) int {
	n, cnt := st.n, 0
	for c := 0; c < n; c++ {
		if st.cand[r*n+c] {
			cnt++
		}
	}
	return cnt
}

func (st *logicState) colCandCount(c int) int {
	n, cnt := st.n, 0
	for r := 0; r < n; r++ {
		if st.cand[r*n+c] {
			cnt++
		}
	}
	return cnt
}

func (st *logicState) regionCandCount(reg int) int {
	cnt := 0
	for i := 0; i < st.n*st.n; i++ {
		if st.cand[i] && st.region[i] == reg {
			cnt++
		}
	}
	return cnt
}

// singletons places any row/col/region that has exactly one candidate.
func (st *logicState) singletons() bool {
	n := st.n
	// Region singletons first — they encode the region constraint directly.
	for reg := 0; reg < n; reg++ {
		if st.regDone[reg] {
			continue
		}
		idx := -1
		cnt := 0
		for i := 0; i < n*n; i++ {
			if st.cand[i] && st.region[i] == reg {
				cnt++
				idx = i
			}
		}
		if cnt == 1 {
			cell := engine.CellAt(idx, n)
			st.deepen(TechniqueSingleton)
			if !st.place(cell.Row, cell.Col) {
				st.failed = true
			}
			return true
		}
	}
	for r := 0; r < n; r++ {
		if st.rowDone[r] {
			continue
		}
		col := -1
		cnt := 0
		for c := 0; c < n; c++ {
			if st.cand[r*n+c] {
				cnt++
				col = c
			}
		}
		if cnt == 1 {
			st.deepen(TechniqueSingleton)
			if !st.place(r, col) {
				st.failed = true
			}
			return true
		}
	}
	for c := 0; c < n; c++ {
		if st.colDone[c] {
			continue
		}
		row := -1
		cnt := 0
		for r := 0; r < n; r++ {
			if st.cand[r*n+c] {
				cnt++
				row = r
			}
		}
		if cnt == 1 {
			st.deepen(TechniqueSingleton)
			if !st.place(row, c) {
				st.failed = true
			}
			return true
		}
	}
	return false
}

// lineLocks applies single-line confinement in all four directions:
// a region confined to one row/col reserves it; a row/col confined to one
// region reserves that region's line.
func (st *logicState) lineLocks() bool {
	n := st.n
	changed := false

	// Region confined to a single row or column.
	for reg := 0; reg < n; reg++ {
		if st.regDone[reg] {
			continue
		}
		row, col := -1, -1
		singleRow, singleCol := true, true
		for i := 0; i < n*n; i++ {
			if !st.cand[i] || st.region[i] != reg {
				continue
			}
			c := engine.CellAt(i, n)
			if row == -1 {
				row, col = c.Row, c.Col
			} else {
				if c.Row != row {
					singleRow = false
				}
				if c.Col != col {
					singleCol = false
				}
			}
		}
		if row == -1 {
			continue
		}
		if singleRow {
			for c := 0; c < n; c++ {
				i := row*n + c
				if st.cand[i] && st.region[i] != reg {
					st.cand[i] = false
					changed = true
				}
			}
		}
		if singleCol {
			for r := 0; r < n; r++ {
				i := r*n + col
				if st.cand[i] && st.region[i] != reg {
					st.cand[i] = false
					changed = true
				}
			}
		}
	}

	// Row confined to a single region.
	for r := 0; r < n; r++ {
		if st.rowDone[r] {
			continue
		}
		reg := -1
		single := true
		for c := 0; c < n; c++ {
			if !st.cand[r*n+c] {
				continue
			}
			rg := st.region[r*n+c]
			if reg == -1 {
				reg = rg
			} else if rg != reg {
				single = false
				break
			}
		}
		if single && reg != -1 {
			for i := 0; i < n*n; i++ {
				if st.cand[i] && st.region[i] == reg && engine.CellAt(i, n).Row != r {
					st.cand[i] = false
					changed = true
				}
			}
		}
	}

	// Column confined to a single region.
	for c := 0; c < n; c++ {
		if st.colDone[c] {
			continue
		}
		reg := -1
		single := true
		for r := 0; r < n; r++ {
			if !st.cand[r*n+c] {
				continue
			}
			rg := st.region[r*n+c]
			if reg == -1 {
				reg = rg
			} else if rg != reg {
				single = false
				break
			}
		}
		if single && reg != -1 {
			for i := 0; i < n*n; i++ {
				if st.cand[i] && st.region[i] == reg && engine.CellAt(i, n).Col != c {
					st.cand[i] = false
					changed = true
				}
			}
		}
	}

	if changed {
		st.deepen(TechniqueRegionLineLock)
	}
	return changed
}

// setLocks applies naked-subset confinement for rows<->cols: any k rows whose
// combined candidate columns number exactly k consume those columns, so they
// are eliminated from other rows (and symmetrically for columns). Sizes 2..3.
func (st *logicState) setLocks() bool {
	if st.rowColSubset(true) {
		st.deepen(TechniqueSetLocking)
		return true
	}
	if st.rowColSubset(false) {
		st.deepen(TechniqueSetLocking)
		return true
	}
	return false
}

// rowColSubset looks for naked subsets among rows (byRow=true) or columns.
func (st *logicState) rowColSubset(byRow bool) bool {
	n := st.n
	// candidate line -> set of the other axis's positions (as bitmask).
	lines := make([]uint32, 0, n)
	lineIdx := make([]int, 0, n)
	for a := 0; a < n; a++ {
		done := st.rowDone[a]
		if !byRow {
			done = st.colDone[a]
		}
		if done {
			continue
		}
		var mask uint32
		for b := 0; b < n; b++ {
			var i int
			if byRow {
				i = a*n + b
			} else {
				i = b*n + a
			}
			if st.cand[i] {
				mask |= 1 << uint(b)
			}
		}
		lines = append(lines, mask)
		lineIdx = append(lineIdx, a)
	}
	// Try subset sizes 2 and 3.
	m := len(lines)
	for size := 2; size <= 3 && size < m; size++ {
		combo := make([]int, size)
		var rec func(start, depth int, union uint32) bool
		rec = func(start, depth int, union uint32) bool {
			if depth == size {
				if popcount(union) == size {
					// These size lines consume exactly these positions.
					changed := false
					inSet := make(map[int]bool, size)
					for _, ci := range combo {
						inSet[lineIdx[ci]] = true
					}
					for other := 0; other < n; other++ {
						if inSet[other] {
							continue
						}
						for b := 0; b < n; b++ {
							if union&(1<<uint(b)) == 0 {
								continue
							}
							var i int
							if byRow {
								i = other*n + b
							} else {
								i = b*n + other
							}
							if st.cand[i] {
								st.cand[i] = false
								changed = true
							}
						}
					}
					if changed {
						return true
					}
				}
				return false
			}
			for ci := start; ci < m; ci++ {
				newUnion := union | lines[ci]
				if popcount(newUnion) > size {
					continue
				}
				combo[depth] = ci
				if rec(ci+1, depth+1, newUnion) {
					return true
				}
			}
			return false
		}
		if rec(0, 0, 0) {
			return true
		}
	}
	return false
}

func popcount(x uint32) int {
	c := 0
	for x != 0 {
		x &= x - 1
		c++
	}
	return c
}

// contradictionElimination is a depth-1 proof step: if assuming a queen at a
// candidate cell leads (after full no-guess propagation) to a contradiction,
// that cell cannot hold a queen and is eliminated. This is sound — it removes
// only provably-impossible cells — and captures adjacency-driven exclusions.
func (st *logicState) contradictionElimination() bool {
	n := st.n
	for i := 0; i < n*n; i++ {
		if !st.cand[i] {
			continue
		}
		cell := engine.CellAt(i, n)
		if st.rowDone[cell.Row] {
			continue
		}
		trial := st.clone()
		if !trial.place(cell.Row, cell.Col) {
			st.cand[i] = false
			st.deepen(TechniqueAdjacencyExclusion)
			return true
		}
		if !trial.propagate() {
			st.cand[i] = false
			st.deepen(TechniqueAdjacencyExclusion)
			return true
		}
	}
	return false
}

// propagate runs only the cheap, non-lookahead techniques to fixpoint, used
// inside contradictionElimination's trials. Returns false on contradiction.
func (st *logicState) propagate() bool {
	for {
		if st.solvedAll() {
			return true
		}
		if st.failed || st.hasContradiction() {
			return false
		}
		if st.singletons() {
			continue
		}
		if st.lineLocks() {
			continue
		}
		if st.setLocks() {
			continue
		}
		return true // stuck but not contradictory
	}
}

func (st *logicState) solution() Solution {
	return Solution{N: st.n, QueenAt: append([]int(nil), st.placed...)}
}
