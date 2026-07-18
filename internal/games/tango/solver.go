package tango

import (
	"github.com/Jensen95/tui-games/internal/engine"
)

// Solver implements engine.Solver[Puzzle, Board] via a complete backtracking
// search. It is the ground truth for existence and uniqueness; LogicSolve
// (logicsolve.go) is the independent no-guess oracle used for difficulty
// labeling and hints. See docs/plan/games/tango.md "Solver approach".
type Solver struct{}

var _ engine.Solver[Puzzle, Board] = Solver{}

// boardFromPuzzle builds the initial (givens-only) board for solving.
func boardFromPuzzle(p Puzzle) Board {
	cells := make([]Symbol, p.N*p.N)
	for idx, sym := range p.Givens {
		cells[idx] = sym
	}
	return Board{N: p.N, Cells: cells, HEdges: p.HEdges, VEdges: p.VEdges}
}

// violatesAt reports whether the (just-assigned) symbol at idx breaks any
// rule that touches idx: row/col balance capacity, an already-filled
// horizontal/vertical three-in-a-row through idx (diagonals are never
// checked), or an edge constraint incident to idx whose other endpoint is
// already filled. It assumes every other cell in b already satisfies these
// checks, so only idx's neighborhood needs (re)checking — the standard
// incremental-backtracking shortcut that keeps Solve/CountSolutions fast.
func violatesAt(b *Board, idx int) bool {
	n := b.N
	sym := b.Cells[idx]
	if sym == Empty {
		return false
	}
	row, col := idx/n, idx%n
	half := n / 2

	// Row balance.
	sunN, moonN := 0, 0
	rowStart := row * n
	for c := 0; c < n; c++ {
		switch b.Cells[rowStart+c] {
		case Sun:
			sunN++
		case Moon:
			moonN++
		}
	}
	if sunN > half || moonN > half {
		return true
	}

	// Column balance.
	sunN, moonN = 0, 0
	for r := 0; r < n; r++ {
		switch b.Cells[r*n+col] {
		case Sun:
			sunN++
		case Moon:
			moonN++
		}
	}
	if sunN > half || moonN > half {
		return true
	}

	// Horizontal triplets touching col.
	for start := col - 2; start <= col; start++ {
		if start < 0 || start+2 >= n {
			continue
		}
		i0 := rowStart + start
		a, b1, c := b.Cells[i0], b.Cells[i0+1], b.Cells[i0+2]
		if a != Empty && a == b1 && b1 == c {
			return true
		}
	}

	// Vertical triplets touching row.
	for start := row - 2; start <= row; start++ {
		if start < 0 || start+2 >= n {
			continue
		}
		i0 := start*n + col
		a, b1, c := b.Cells[i0], b.Cells[i0+n], b.Cells[i0+2*n]
		if a != Empty && a == b1 && b1 == c {
			return true
		}
	}

	// Edge constraints incident to idx.
	if edgeViolatesAt(b.Cells, b.HEdges, idx, sym) || edgeViolatesAt(b.Cells, b.VEdges, idx, sym) {
		return true
	}
	return false
}

// edgeViolatesAt checks the edges (from one edge set) incident to idx
// against an already-filled other endpoint.
func edgeViolatesAt(cells []Symbol, edges map[[2]int]Relation, idx int, sym Symbol) bool {
	for pair, rel := range edges {
		var other int
		switch idx {
		case pair[0]:
			other = pair[1]
		case pair[1]:
			other = pair[0]
		default:
			continue
		}
		os := cells[other]
		if os == Empty {
			continue
		}
		if rel == Equal && os != sym {
			return true
		}
		if rel == Cross && os == sym {
			return true
		}
	}
	return false
}

// backtrack fills b.Cells[idx:] in row-major order, skipping cells already
// fixed by a given. If onSolution is nil, backtrack stops and returns true
// at the first complete valid assignment (used by Solve). If onSolution is
// non-nil, it is invoked for every complete valid assignment found; once it
// returns true, the whole search stops early (used by CountSolutions to
// respect its cap without exploring further).
func backtrack(b *Board, idx int, onSolution func() bool) bool {
	if idx == len(b.Cells) {
		if onSolution != nil {
			return onSolution()
		}
		return true
	}
	if b.Cells[idx] != Empty {
		return backtrack(b, idx+1, onSolution)
	}
	for _, sym := range [2]Symbol{Sun, Moon} {
		b.Cells[idx] = sym
		if !violatesAt(b, idx) {
			if backtrack(b, idx+1, onSolution) {
				return true
			}
		}
		b.Cells[idx] = Empty
	}
	return false
}

// givensViolated reports whether the initial givens-only board already breaks
// a rule that lies entirely among filled (given) cells: a given-only
// three-in-a-row, a given-only "="/"×" edge conflict, or a row/column already
// holding more than N/2 of one symbol. backtrack skips given cells and
// violatesAt only re-checks constraints touching the just-placed cell, so a
// constraint whose cells are ALL givens is otherwise never validated during
// the search. Any such violation makes the puzzle unsatisfiable (0 solutions),
// so Solve/CountSolutions must reject it up front rather than reporting phantom
// completions of a self-contradictory clue set.
func givensViolated(b Board) bool {
	return len((Validator{}).Violations(b)) > 0
}

// Solve returns one solution via backtracking search, if any exists.
func (s Solver) Solve(p Puzzle) (Board, bool) {
	b := boardFromPuzzle(p)
	if givensViolated(b) {
		return b, false
	}
	ok := backtrack(&b, 0, nil)
	return b, ok
}

// CountSolutions returns min(#solutions, capN); uniqueness is
// capN>=2 && result==1.
func (s Solver) CountSolutions(p Puzzle, capN int) int {
	if capN <= 0 {
		return 0
	}
	b := boardFromPuzzle(p)
	if givensViolated(b) {
		return 0
	}
	count := 0
	backtrack(&b, 0, func() bool {
		count++
		return count >= capN
	})
	return count
}
