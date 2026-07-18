package queens

import (
	"os"
	"sort"
	"strconv"

	"github.com/Jensen95/tui-games/internal/engine"
)

// --- fixture construction helpers (test-only; no package logic here) ---

// newBoard builds a Board with queens at exactly the given cells, Empty
// elsewhere.
func newBoard(n int, region []int, queens []engine.Cell) Board {
	cells := make([]Cell, n*n)
	for _, q := range queens {
		cells[engine.Index(q, n)] = Queen
	}
	return Board{N: n, Region: append([]int(nil), region...), Cells: cells}
}

// baseRegion5x5 is a 5x5 region grid where every cell has a distinct region
// id (id == row-major index), except that cell (3,3) is forced into cell
// (1,1)'s region. That single override lets one shared fixture isolate every
// validator rule in the truth table:
//
//   - (0,0) vs (0,3): same row, different regions, chebyshev distance 3 (not adjacent)
//   - (0,0) vs (3,0): same col, different regions, chebyshev distance 3 (not adjacent)
//   - (1,1) vs (3,3): different row/col, SAME region (the forced override), chebyshev distance 2 (not adjacent)
//   - (1,1) vs (2,2): different row/col/region, chebyshev distance 1 (corner-adjacent)
//   - (1,1) vs (1,2): same row, different region, chebyshev distance 1 (edge-adjacent) -> both same-row and adjacent fire
//   - (0,0) vs (3,3): different row/col/region, on the same full diagonal, chebyshev distance 3 (NOT adjacent) -> guards the classic full-diagonal chess bug
func baseRegion5x5() []int {
	const n = 5
	region := make([]int, n*n)
	for i := range region {
		region[i] = i
	}
	region[engine.Index(engine.Cell{Row: 3, Col: 3}, n)] = engine.Index(engine.Cell{Row: 1, Col: 1}, n)
	return region
}

// hasRule reports whether violations contains a Violation with the given rule.
func hasRule(violations []engine.Violation, rule string) bool {
	for _, v := range violations {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

// ruleSet returns the distinct set of Rule values present in violations.
func ruleSet(violations []engine.Violation) map[string]bool {
	out := make(map[string]bool, len(violations))
	for _, v := range violations {
		out[v.Rule] = true
	}
	return out
}

// --- golden fixtures (hand-built, verified offline for uniqueness) ---

// goldenUniqueRegion6 is a 6x6 region grid (hand-verified via brute-force
// search, not part of this package) whose unique solution is
// goldenUniqueSolution6. It is the golden board for solver correctness tests.
//
// Region grid (row-major, one digit per cell):
//
//	2 0 0 0 0 0
//	2 0 0 1 1 1
//	2 2 1 1 1 1
//	4 2 1 1 1 3
//	4 5 1 5 1 5
//	5 5 5 5 5 5
func goldenUniqueRegion6() []int {
	return []int{
		2, 0, 0, 0, 0, 0,
		2, 0, 0, 1, 1, 1,
		2, 2, 1, 1, 1, 1,
		4, 2, 1, 1, 1, 3,
		4, 5, 1, 5, 1, 5,
		5, 5, 5, 5, 5, 5,
	}
}

// goldenUniqueSolution6 is the sole solution to goldenUniqueRegion6:
// row -> col is 0->2, 1->4, 2->1, 3->5, 4->0, 5->3.
func goldenUniqueSolution6() Solution {
	return Solution{N: 6, QueenAt: []int{2, 4, 1, 5, 0, 3}}
}

func goldenUniquePuzzle6() Puzzle {
	return Puzzle{N: 6, Region: goldenUniqueRegion6(), DiffV: engine.Easy, SeedV: 1}
}

// ambiguousRegion5 is a 5x5 region grid (hand-verified via brute-force
// search) with exactly two solutions, ambiguousSolutions5. Rows 2-4 are
// forced (cols 4,1,3); rows 0-1 can swap between cols {0,2} without breaking
// any rule, which is exactly the kind of border ambiguity the generator's
// "enforce uniqueness by reshaping" step must eliminate.
//
// Region grid (row-major, one digit per cell):
//
//	1 1 0 2 2
//	1 1 0 2 2
//	3 3 3 2 2
//	3 3 3 4 2
//	3 3 4 4 2
func ambiguousRegion5() []int {
	return []int{
		1, 1, 0, 2, 2,
		1, 1, 0, 2, 2,
		3, 3, 3, 2, 2,
		3, 3, 3, 4, 2,
		3, 3, 4, 4, 2,
	}
}

func ambiguousPuzzle5() Puzzle {
	return Puzzle{N: 5, Region: ambiguousRegion5(), DiffV: engine.Easy, SeedV: 2}
}

// ambiguousSolutions5 are the two distinct solutions to ambiguousRegion5.
func ambiguousSolutions5() []Solution {
	return []Solution{
		{N: 5, QueenAt: []int{0, 2, 4, 1, 3}},
		{N: 5, QueenAt: []int{2, 0, 4, 1, 3}},
	}
}

// classicNonAttacking5 is a hand-verified valid, complete N=5 solved board:
// a non-attacking placement (so trivially non-touching too) with region ==
// row (each row its own connected region). Used for the Validator.Solved
// happy path, where uniqueness doesn't matter.
func classicNonAttacking5() Board {
	const n = 5
	region := make([]int, n*n)
	for r := 0; r < n; r++ {
		for c := 0; c < n; c++ {
			region[engine.Index(engine.Cell{Row: r, Col: c}, n)] = r
		}
	}
	cols := []int{1, 3, 0, 2, 4}
	queens := make([]engine.Cell, n)
	for r, c := range cols {
		queens[r] = engine.Cell{Row: r, Col: c}
	}
	return newBoard(n, region, queens)
}

// --- generation-invariant / canonicalization test helpers ---

// seedCount returns the number of seeds property tests should exercise,
// honoring LIG_SEEDS (default 250) so CI stays fast and nightly can go heavy.
func seedCount() int {
	if v := os.Getenv("LIG_SEEDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 250
}

// distinctRegionIDs returns the set of region ids appearing in region.
func distinctRegionIDs(region []int) map[int]bool {
	out := make(map[int]bool)
	for _, id := range region {
		out[id] = true
	}
	return out
}

// regionsConnected reports whether every region in an n x n row-major region
// grid is 4-connected (shares edges, not just corners) — the connectivity
// invariant the generator's flood-grow step must preserve.
func regionsConnected(n int, region []int) bool {
	byID := make(map[int][]engine.Cell)
	for i, id := range region {
		byID[id] = append(byID[id], engine.CellAt(i, n))
	}
	for _, cells := range byID {
		if len(cells) == 0 {
			continue
		}
		seen := map[engine.Cell]bool{cells[0]: true}
		queue := []engine.Cell{cells[0]}
		want := make(map[engine.Cell]bool, len(cells))
		for _, c := range cells {
			want[c] = true
		}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, nb := range engine.Neighbors4(cur, n, n) {
				if want[nb] && !seen[nb] {
					seen[nb] = true
					queue = append(queue, nb)
				}
			}
		}
		if len(seen) != len(cells) {
			return false
		}
	}
	return true
}

// boardFromSolution builds a complete Board (all N queens placed) for the
// given puzzle's region grid and recorded solution.
func boardFromSolution(p Puzzle, sol Solution) Board {
	queens := make([]engine.Cell, sol.N)
	for row, col := range sol.QueenAt {
		queens[row] = engine.Cell{Row: row, Col: col}
	}
	return newBoard(p.N, p.Region, queens)
}

// transformPuzzle applies dihedral transform t to p's region grid and givens,
// re-normalizing region labels by first appearance afterward (canonicalization
// must be color-agnostic — see docs/plan/games/queens.md). This is
// independent, test-only fixture logic used to prove the package's own
// Fingerprinter treats every symmetric variant of a puzzle identically.
func transformPuzzle(p Puzzle, t engine.Transform) Puzzle {
	n := p.N
	newRegion := make([]int, n*n)
	for r := 0; r < n; r++ {
		for c := 0; c < n; c++ {
			src := engine.Cell{Row: r, Col: c}
			dst := t.Apply(src, n, n)
			newRegion[engine.Index(dst, n)] = p.Region[engine.Index(src, n)]
		}
	}
	newRegion = engine.RelabelFirstAppearance(newRegion)

	var newGivens []int
	if len(p.Givens) > 0 {
		newGivens = make([]int, len(p.Givens))
		for i, g := range p.Givens {
			src := engine.CellAt(g, n)
			dst := t.Apply(src, n, n)
			newGivens[i] = engine.Index(dst, n)
		}
		sort.Ints(newGivens)
	}

	return Puzzle{N: n, Region: newRegion, Givens: newGivens, SeedV: p.SeedV, DiffV: p.DiffV}
}
