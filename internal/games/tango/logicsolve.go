package tango

import (
	"github.com/Jensen95/tui-games/internal/engine"
)

// Technique name constants for the no-guess deduction ladder, ordered
// cheapest to most expensive per docs/plan/games/tango.md "Deduction
// ladder". TechniqueGiven is the baseline reported when a puzzle needed no
// deduction at all (e.g. every cell already given); TechniqueUniqueness is
// reserved for the advanced, optional line-uniqueness rule and is not
// currently applied by the ladder (see logicSolve's doc comment).
const (
	TechniqueGiven           engine.Technique = "given"
	TechniqueEdgePropagation engine.Technique = "edge-propagation"
	TechniquePairDoublet     engine.Technique = "pair-doublet"
	TechniqueGapSandwich     engine.Technique = "gap-sandwich"
	TechniqueLineCount       engine.Technique = "line-count"
	TechniqueUniqueness      engine.Technique = "uniqueness"
)

// techniqueRank orders techniques cheapest to most expensive so bumpTechnique
// can track the single deepest technique used across a whole solve.
var techniqueRank = map[engine.Technique]int{
	TechniqueGiven:           0,
	TechniqueEdgePropagation: 1,
	TechniquePairDoublet:     2,
	TechniqueGapSandwich:     3,
	TechniqueLineCount:       4,
	TechniqueUniqueness:      5,
}

func bumpTechnique(cur *engine.Technique, t engine.Technique) {
	if techniqueRank[t] > techniqueRank[*cur] {
		*cur = t
	}
}

// edgeNeighbor is one edge-constrained neighbor of a cell.
type edgeNeighbor struct {
	other int
	rel   Relation
}

// buildEdgeIndex flattens a puzzle's H/V edge maps into a per-cell adjacency
// list so edge propagation doesn't rescan every edge for every cell.
func buildEdgeIndex(n int, hedges, vedges map[[2]int]Relation) [][]edgeNeighbor {
	idx := make([][]edgeNeighbor, n*n)
	add := func(m map[[2]int]Relation) {
		for pair, rel := range m {
			a, b := pair[0], pair[1]
			idx[a] = append(idx[a], edgeNeighbor{other: b, rel: rel})
			idx[b] = append(idx[b], edgeNeighbor{other: a, rel: rel})
		}
	}
	add(hedges)
	add(vedges)
	return idx
}

// LogicSolve attempts a no-guess solve using the deduction ladder from
// docs/plan/games/tango.md: edge propagation, pair/doublet, gap/sandwich,
// then line-count, iterated to a fixpoint. It returns the resulting board
// (partial if it didn't close), whether the board fully closed, and the
// single deepest technique needed anywhere during the solve. The advanced,
// optional "uniqueness" (line-difference) rule is not implemented: a puzzle
// that would require it simply won't close here, which is safe because the
// generator only accepts carves that DO close under this ladder.
func (s Solver) LogicSolve(p Puzzle) (Board, bool, engine.Technique) {
	n := p.N
	cells := make([]Symbol, n*n)
	for idx, sym := range p.Givens {
		cells[idx] = sym
	}
	deepest := TechniqueGiven
	edgeIdx := buildEdgeIndex(n, p.HEdges, p.VEdges)

	for {
		progress := false
		if applyEdgePropagation(cells, edgeIdx, &deepest) {
			progress = true
		}
		if applyPairDoublet(cells, n, &deepest) {
			progress = true
		}
		if applyGapSandwich(cells, n, &deepest) {
			progress = true
		}
		if applyLineCount(cells, n, &deepest) {
			progress = true
		}
		if !progress {
			break
		}
	}

	closed := true
	for _, c := range cells {
		if c == Empty {
			closed = false
			break
		}
	}
	board := Board{N: n, Cells: cells, HEdges: p.HEdges, VEdges: p.VEdges}
	// A board can only be reported "closed" if it is a genuine solution. If the
	// clue set is self-contradictory (e.g. a given-only three-in-a-row), the
	// unchecked deduction ladder could fill every cell yet leave a rule broken;
	// never advertise such a board as a no-guess solve.
	if closed && len((Validator{}).Violations(board)) > 0 {
		closed = false
	}
	return board, closed, deepest
}

// applyEdgePropagation: a known cell + "="/"×" edge forces its neighbor.
func applyEdgePropagation(cells []Symbol, edgeIdx [][]edgeNeighbor, deepest *engine.Technique) bool {
	progress := false
	for idx := range cells {
		if cells[idx] != Empty {
			continue
		}
		for _, e := range edgeIdx[idx] {
			if cells[e.other] == Empty {
				continue
			}
			want := cells[e.other]
			if e.rel == Cross {
				want = flip(want)
			}
			cells[idx] = want
			progress = true
			bumpTechnique(deepest, TechniqueEdgePropagation)
			break
		}
	}
	return progress
}

// applyPairDoublet: two identical adjacent symbols force the opposite
// symbol on both outer flanks (else a three-in-a-row).
func applyPairDoublet(cells []Symbol, n int, deepest *engine.Technique) bool {
	progress := false
	// Rows.
	for row := 0; row < n; row++ {
		base := row * n
		for col := 0; col < n-1; col++ {
			a, b := cells[base+col], cells[base+col+1]
			if a == Empty || a != b {
				continue
			}
			if col-1 >= 0 && cells[base+col-1] == Empty {
				cells[base+col-1] = flip(a)
				progress = true
				bumpTechnique(deepest, TechniquePairDoublet)
			}
			if col+2 < n && cells[base+col+2] == Empty {
				cells[base+col+2] = flip(a)
				progress = true
				bumpTechnique(deepest, TechniquePairDoublet)
			}
		}
	}
	// Columns.
	for col := 0; col < n; col++ {
		for row := 0; row < n-1; row++ {
			i0 := row*n + col
			i1 := i0 + n
			a, b := cells[i0], cells[i1]
			if a == Empty || a != b {
				continue
			}
			if row-1 >= 0 && cells[i0-n] == Empty {
				cells[i0-n] = flip(a)
				progress = true
				bumpTechnique(deepest, TechniquePairDoublet)
			}
			if row+2 < n && cells[i1+n] == Empty {
				cells[i1+n] = flip(a)
				progress = true
				bumpTechnique(deepest, TechniquePairDoublet)
			}
		}
	}
	return progress
}

// applyGapSandwich: pattern A _ A forces the middle to the opposite of A.
func applyGapSandwich(cells []Symbol, n int, deepest *engine.Technique) bool {
	progress := false
	// Rows.
	for row := 0; row < n; row++ {
		base := row * n
		for col := 0; col+2 < n; col++ {
			a, mid, c := cells[base+col], cells[base+col+1], cells[base+col+2]
			if a != Empty && mid == Empty && a == c {
				cells[base+col+1] = flip(a)
				progress = true
				bumpTechnique(deepest, TechniqueGapSandwich)
			}
		}
	}
	// Columns.
	for col := 0; col < n; col++ {
		for row := 0; row+2 < n; row++ {
			i0 := row*n + col
			i1 := i0 + n
			i2 := i0 + 2*n
			a, mid, c := cells[i0], cells[i1], cells[i2]
			if a != Empty && mid == Empty && a == c {
				cells[i1] = flip(a)
				progress = true
				bumpTechnique(deepest, TechniqueGapSandwich)
			}
		}
	}
	return progress
}

// applyLineCount: a line already holding N/2 of one symbol forces the rest
// of that line to the other symbol.
func applyLineCount(cells []Symbol, n int, deepest *engine.Technique) bool {
	progress := false
	half := n / 2

	fillRest := func(count func(int) Symbol, set func(int, Symbol), sunN, moonN int) bool {
		var fill Symbol
		switch {
		case sunN == half && moonN < half:
			fill = Moon
		case moonN == half && sunN < half:
			fill = Sun
		default:
			return false
		}
		changed := false
		for i := 0; i < n; i++ {
			if count(i) == Empty {
				set(i, fill)
				changed = true
			}
		}
		return changed
	}

	// Rows.
	for row := 0; row < n; row++ {
		base := row * n
		sunN, moonN := 0, 0
		for col := 0; col < n; col++ {
			switch cells[base+col] {
			case Sun:
				sunN++
			case Moon:
				moonN++
			}
		}
		if fillRest(
			func(col int) Symbol { return cells[base+col] },
			func(col int, sym Symbol) { cells[base+col] = sym },
			sunN, moonN,
		) {
			progress = true
			bumpTechnique(deepest, TechniqueLineCount)
		}
	}

	// Columns.
	for col := 0; col < n; col++ {
		sunN, moonN := 0, 0
		for row := 0; row < n; row++ {
			switch cells[row*n+col] {
			case Sun:
				sunN++
			case Moon:
				moonN++
			}
		}
		if fillRest(
			func(row int) Symbol { return cells[row*n+col] },
			func(row int, sym Symbol) { cells[row*n+col] = sym },
			sunN, moonN,
		) {
			progress = true
			bumpTechnique(deepest, TechniqueLineCount)
		}
	}

	return progress
}
