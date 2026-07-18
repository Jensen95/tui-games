package tango

// This file holds an INDEPENDENT solution counter used only to cross-check the
// primary complete solver (solver.go) and the generator. It is deliberately a
// different algorithm: solver.go fills the grid cell-by-cell in row-major order
// with an incremental per-cell violation check; this counter instead enumerates
// whole-row candidate patterns (each already guaranteed row-balanced and
// row-triple-free), then does a depth-first search over ROWS, wiring column
// balance/triples and the "="/"×" edges as it stacks rows. Two independently
// structured searches agreeing on the solution count (and the unique solution)
// is what makes the uniqueness guarantee meaningful — a bug shared by the
// primary solver and generator surfaces here as a disagreement.
//
// It is defined in a non-test file so both crosscheck_test.go here and any
// future validation harness can call it, but it is not part of the engine
// Solver interface and is never wired into the registry.

// rowPatterns returns every length-n row assignment of Sun/Moon that already
// satisfies the intra-row rules: exactly n/2 of each symbol and no three equal
// in a row horizontally. On a 6-wide grid there are 14 such patterns.
func rowPatterns(n int) [][]Symbol {
	half := n / 2
	var out [][]Symbol
	cur := make([]Symbol, 0, n)
	var rec func(sun, moon int)
	rec = func(sun, moon int) {
		if len(cur) == n {
			cp := make([]Symbol, n)
			copy(cp, cur)
			out = append(out, cp)
			return
		}
		for _, s := range [2]Symbol{Sun, Moon} {
			if s == Sun && sun >= half {
				continue
			}
			if s == Moon && moon >= half {
				continue
			}
			l := len(cur)
			if l >= 2 && cur[l-1] == s && cur[l-2] == s {
				continue // would make a horizontal triple
			}
			cur = append(cur, s)
			if s == Sun {
				rec(sun+1, moon)
			} else {
				rec(sun, moon+1)
			}
			cur = cur[:l]
		}
	}
	rec(0, 0)
	return out
}

// rowMatchesGivens reports whether pat is consistent with the puzzle's givens
// in row r.
func rowMatchesGivens(p Puzzle, r int, pat []Symbol) bool {
	base := r * p.N
	for c := 0; c < p.N; c++ {
		if g, ok := p.Givens[base+c]; ok && pat[c] != g {
			return false
		}
	}
	return true
}

// rowMatchesHEdges reports whether pat honors every horizontal edge inside row
// r (horizontal edges only ever connect two cells of the same row).
func rowMatchesHEdges(p Puzzle, r int, pat []Symbol) bool {
	base := r * p.N
	for c := 0; c+1 < p.N; c++ {
		rel, ok := p.HEdges[[2]int{base + c, base + c + 1}]
		if !ok {
			continue
		}
		if rel == Equal && pat[c] != pat[c+1] {
			return false
		}
		if rel == Cross && pat[c] == pat[c+1] {
			return false
		}
	}
	return true
}

// rowMatchesVEdges reports whether pat (candidate for row r) honors every
// vertical edge between row r-1 (prev) and row r.
func rowMatchesVEdges(p Puzzle, r int, pat, prev []Symbol) bool {
	if r == 0 {
		return true
	}
	up := (r - 1) * p.N
	down := r * p.N
	for c := 0; c < p.N; c++ {
		rel, ok := p.VEdges[[2]int{up + c, down + c}]
		if !ok {
			continue
		}
		if rel == Equal && prev[c] != pat[c] {
			return false
		}
		if rel == Cross && prev[c] == pat[c] {
			return false
		}
	}
	return true
}

// columnsStayLegal reports whether appending pat as row r keeps every column
// legal so far: no column exceeds n/2 of a symbol, and no vertical triple forms
// with the two rows directly above. grid[0..r-1] hold the committed rows.
func columnsStayLegal(n, half, r int, pat []Symbol, grid [][]Symbol) bool {
	for c := 0; c < n; c++ {
		sun, moon := 0, 0
		for rr := 0; rr < r; rr++ {
			switch grid[rr][c] {
			case Sun:
				sun++
			case Moon:
				moon++
			}
		}
		switch pat[c] {
		case Sun:
			sun++
		case Moon:
			moon++
		}
		if sun > half || moon > half {
			return false
		}
		if r >= 2 && grid[r-1][c] == pat[c] && grid[r-2][c] == pat[c] {
			return false
		}
	}
	return true
}

// countSolutionsCross returns min(#solutions, capN) for puzzle p using the
// row-pattern DFS. It is the independent oracle the cross-check tests compare
// against the primary Solver.CountSolutions.
func countSolutionsCross(p Puzzle, capN int) int {
	if capN <= 0 {
		return 0
	}
	n := p.N
	half := n / 2
	pats := rowPatterns(n)

	// Pre-filter patterns per row by the row-local constraints (givens + H
	// edges) so the DFS only stacks rows that can possibly appear there.
	perRow := make([][][]Symbol, n)
	for r := 0; r < n; r++ {
		for _, pat := range pats {
			if rowMatchesGivens(p, r, pat) && rowMatchesHEdges(p, r, pat) {
				perRow[r] = append(perRow[r], pat)
			}
		}
	}

	grid := make([][]Symbol, n)
	count := 0
	var dfs func(r int)
	dfs = func(r int) {
		if count >= capN {
			return
		}
		if r == n {
			count++
			return
		}
		var prev []Symbol
		if r > 0 {
			prev = grid[r-1]
		}
		for _, pat := range perRow[r] {
			if !rowMatchesVEdges(p, r, pat, prev) {
				continue
			}
			if !columnsStayLegal(n, half, r, pat, grid) {
				continue
			}
			grid[r] = pat
			dfs(r + 1)
			grid[r] = nil
			if count >= capN {
				return
			}
		}
	}
	dfs(0)
	return count
}

// solveCross returns the first complete solution found by the row-pattern DFS,
// or ok=false if the puzzle has none. When the puzzle is uniquely solvable this
// is that unique solution, letting the cross-check compare the two solvers'
// actual boards, not just their counts.
func solveCross(p Puzzle) (Board, bool) {
	n := p.N
	half := n / 2
	pats := rowPatterns(n)

	perRow := make([][][]Symbol, n)
	for r := 0; r < n; r++ {
		for _, pat := range pats {
			if rowMatchesGivens(p, r, pat) && rowMatchesHEdges(p, r, pat) {
				perRow[r] = append(perRow[r], pat)
			}
		}
	}

	grid := make([][]Symbol, n)
	var dfs func(r int) bool
	dfs = func(r int) bool {
		if r == n {
			return true
		}
		var prev []Symbol
		if r > 0 {
			prev = grid[r-1]
		}
		for _, pat := range perRow[r] {
			if !rowMatchesVEdges(p, r, pat, prev) {
				continue
			}
			if !columnsStayLegal(n, half, r, pat, grid) {
				continue
			}
			grid[r] = pat
			if dfs(r + 1) {
				return true
			}
			grid[r] = nil
		}
		return false
	}
	if !dfs(0) {
		return Board{}, false
	}
	cells := make([]Symbol, n*n)
	for r := 0; r < n; r++ {
		for c := 0; c < n; c++ {
			cells[r*n+c] = grid[r][c]
		}
	}
	return Board{N: n, Cells: cells, HEdges: p.HEdges, VEdges: p.VEdges}, true
}
