package minisudoku

import (
	"os"
	"strconv"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// seedCount reads LIG_SEEDS (default 250) so CI stays fast and nightly can go
// heavy, matching the convention in minisudoku_test.go.
func seedCount(def int) int {
	if s := os.Getenv("LIG_SEEDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// theSolution is the valid 6×6 solution reused across several fixtures (the
// same grid used in minisudoku_test.go's validator tests).
var theSolution = []int{
	1, 2, 3, 4, 5, 6,
	4, 5, 6, 1, 2, 3,
	2, 3, 1, 5, 6, 4,
	5, 6, 4, 2, 3, 1,
	3, 1, 2, 6, 4, 5,
	6, 4, 5, 3, 1, 2,
}

// ============================================================================
// A. Second solver agrees with the primary solver + logic solver
// ============================================================================

// TestCrosscheck_SolversAgreeOnGenerated is the core cross-validation: over
// LIG_SEEDS seeds and every no-guess difficulty, the independent solver
// (crosscheck.go) must agree with the primary complete solver AND the logic
// solver on both the solution count (exactly 1) and the actual unique solution.
// A generator or primary-solver bug that produces a non-unique or wrong puzzle
// now surfaces as a disagreement here rather than passing silently.
func TestCrosscheck_SolversAgreeOnGenerated(t *testing.T) {
	seeds := seedCount(250)
	g := Generator{}
	s := Solver{}

	for _, diff := range []engine.Difficulty{engine.Easy, engine.Medium, engine.Hard} {
		for seed := int64(1); seed <= int64(seeds); seed++ {
			r := engine.NewRand(seed)
			puzzle, solution, err := g.Generate(diff, r)
			if err != nil {
				t.Fatalf("seed %d diff %s: Generate failed: %v", seed, diff, err)
			}

			// Independent solver: exactly one solution.
			if got := xcheckCount(puzzle, 2); got != 1 {
				t.Fatalf("seed %d diff %s: independent solver counted %d solutions, want 1", seed, diff, got)
			}
			// Primary solver: exactly one solution (agreement on count).
			if got := s.CountSolutions(puzzle, 2); got != 1 {
				t.Fatalf("seed %d diff %s: primary solver counted %d solutions, want 1", seed, diff, got)
			}

			// Independent solver's unique solution == recorded solution.
			xsol, ok := xcheckSolve(puzzle)
			if !ok {
				t.Fatalf("seed %d diff %s: independent solver found no solution", seed, diff)
			}
			assertCellsEqual(t, seed, diff, "independent vs recorded", xsol.Cells, solution.Cells)

			// Primary solver's solution == recorded solution.
			psol, ok := s.Solve(puzzle)
			if !ok {
				t.Fatalf("seed %d diff %s: primary solver found no solution", seed, diff)
			}
			assertCellsEqual(t, seed, diff, "primary vs recorded", psol.Cells, solution.Cells)

			// Logic solver, when it closes, must equal the same unique solution.
			lsol, closed, tech := s.LogicSolve(puzzle)
			if !closed {
				t.Fatalf("seed %d diff %s: logic solver did not close a no-guess puzzle", seed, diff)
			}
			if tech == "" {
				t.Errorf("seed %d diff %s: logic solver reported empty technique", seed, diff)
			}
			assertCellsEqual(t, seed, diff, "logic vs recorded", lsol.Cells, solution.Cells)
		}
	}
}

func assertCellsEqual(t *testing.T, seed int64, diff engine.Difficulty, what string, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("seed %d diff %s: %s length mismatch %d vs %d", seed, diff, what, len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("seed %d diff %s: %s differ at cell %d: %d vs %d", seed, diff, what, i, got[i], want[i])
		}
	}
}

// TestCrosscheck_CountAgreementOnFixtures asserts the two solvers agree on the
// (capped) solution count for a spread of under-constrained boards, including
// genuinely ambiguous ones. The exact count is not assumed a priori — the
// meaningful signal is that two independently written counters agree.
func TestCrosscheck_CountAgreementOnFixtures(t *testing.T) {
	s := Solver{}
	base := func(g map[int]int) Puzzle {
		return Puzzle{N: N, BoxH: BoxH, BoxW: BoxW, Givens: g, Diff: engine.Easy}
	}

	// Build a partial board directly from theSolution keeping a subset of cells.
	partial := func(keep ...int) map[int]int {
		g := map[int]int{}
		for _, idx := range keep {
			g[idx] = theSolution[idx]
		}
		return g
	}

	fixtures := []Puzzle{
		base(map[int]int{}),                                   // empty: many solutions
		base(map[int]int{0: 1}),                               // one given: many solutions
		base(map[int]int{0: 1, 7: 5, 14: 1, 21: 2, 28: 4}),    // diagonal-ish, ambiguous
		base(partial(0, 1, 2, 3, 4, 5, 6, 12, 18, 24, 30)),    // more constrained
		base(partial(0, 3, 6, 9, 12, 15, 18, 21, 24, 27, 30)), // sparse
	}

	for i, p := range fixtures {
		for capN := 1; capN <= 5; capN++ {
			want := s.CountSolutions(p, capN)
			got := xcheckCount(p, capN)
			if got != want {
				t.Errorf("fixture %d cap %d: independent count %d != primary count %d", i, capN, got, want)
			}
		}
	}
}

// TestCrosscheck_DeadlyRectangleExactlyTwo pins a hand-built near-ambiguous
// fixture: a full solution with a 2×2 "deadly rectangle" (cells (0,0),(0,3),
// (1,0),(1,3) holding the swappable pair {1,4}) blanked out. Every other cell
// is a given, so the puzzle has EXACTLY two solutions — the original and the
// 1↔4 swap. Both solvers must report 2, and neither may claim uniqueness.
func TestCrosscheck_DeadlyRectangleExactlyTwo(t *testing.T) {
	// Confirm the rectangle really is a swappable pair in theSolution.
	if !(theSolution[0] == theSolution[9] && theSolution[3] == theSolution[6] && theSolution[0] != theSolution[3]) {
		t.Fatalf("fixture assumption broken: cells 0,3,6,9 are not a deadly rectangle: %d %d %d %d",
			theSolution[0], theSolution[3], theSolution[6], theSolution[9])
	}
	blanks := map[int]bool{0: true, 3: true, 6: true, 9: true}
	givens := map[int]int{}
	for idx, v := range theSolution {
		if !blanks[idx] {
			givens[idx] = v
		}
	}
	p := Puzzle{N: N, BoxH: BoxH, BoxW: BoxW, Givens: givens, Diff: engine.Hard}

	s := Solver{}
	if got := s.CountSolutions(p, 5); got != 2 {
		t.Errorf("primary solver: deadly-rectangle puzzle should have exactly 2 solutions, got %d", got)
	}
	if got := xcheckCount(p, 5); got != 2 {
		t.Errorf("independent solver: deadly-rectangle puzzle should have exactly 2 solutions, got %d", got)
	}
	// It is therefore NOT unique — the generator must never emit such a puzzle.
	if s.CountSolutions(p, 2) == 1 {
		t.Error("primary solver wrongly reports the ambiguous puzzle as unique")
	}
}

// ============================================================================
// B. Validator ↔ independent brute-force checker (fuzzed over small boards)
// ============================================================================

// bruteHasDuplicate independently reports whether board b has a duplicated
// nonzero digit in any row, column, or 2×3 box, computing box membership from
// first principles (not via boxID or the Validator).
func bruteHasDuplicate(cells []int) bool {
	n := N
	unit := func(idxs []int) bool {
		seen := make(map[int]bool)
		for _, idx := range idxs {
			v := cells[idx]
			if v == 0 {
				continue
			}
			if seen[v] {
				return true
			}
			seen[v] = true
		}
		return false
	}
	for row := 0; row < n; row++ {
		var r []int
		for col := 0; col < n; col++ {
			r = append(r, row*n+col)
		}
		if unit(r) {
			return true
		}
	}
	for col := 0; col < n; col++ {
		var c []int
		for row := 0; row < n; row++ {
			c = append(c, row*n+col)
		}
		if unit(c) {
			return true
		}
	}
	// 2×3 boxes, computed independently.
	for band := 0; band < n/BoxH; band++ {
		for stack := 0; stack < n/BoxW; stack++ {
			var box []int
			for dr := 0; dr < BoxH; dr++ {
				for dc := 0; dc < BoxW; dc++ {
					row := band*BoxH + dr
					col := stack*BoxW + dc
					box = append(box, row*n+col)
				}
			}
			if unit(box) {
				return true
			}
		}
	}
	return false
}

// bruteSolved independently reports whether cells is a fully filled, valid
// solution (every cell 1..N, no duplicates).
func bruteSolved(cells []int) bool {
	if len(cells) != N*N {
		return false
	}
	for _, v := range cells {
		if v < 1 || v > N {
			return false
		}
	}
	return !bruteHasDuplicate(cells)
}

// TestCrosscheck_ValidatorMatchesBruteForce fuzzes random full and partial
// boards and asserts the fast Validator agrees with the independent brute-force
// checker: any-violation ⇔ has-duplicate, and Solved ⇔ complete-and-valid.
// (Random cell values are restricted to 0..N so out-of-range violations don't
// muddy the duplicate comparison.)
func TestCrosscheck_ValidatorMatchesBruteForce(t *testing.T) {
	seeds := seedCount(250)
	v := Validator{}
	for seed := int64(1); seed <= int64(seeds); seed++ {
		r := engine.NewRand(seed)
		cells := make([]int, N*N)
		for i := range cells {
			cells[i] = r.IntN(N + 1) // 0..N
		}
		b := Board{Cells: cells}

		viols := v.Violations(b)
		anyDupRule := false
		for _, vv := range viols {
			if vv.Rule == "row" || vv.Rule == "column" || vv.Rule == "box" {
				anyDupRule = true
			}
			if vv.Rule == "value" {
				t.Fatalf("seed %d: unexpected value violation with in-range cells", seed)
			}
		}
		if anyDupRule != bruteHasDuplicate(cells) {
			t.Errorf("seed %d: Validator any-duplicate=%v, brute=%v\ncells=%v", seed, anyDupRule, bruteHasDuplicate(cells), cells)
		}
		if v.Solved(b) != bruteSolved(cells) {
			t.Errorf("seed %d: Validator.Solved=%v, brute=%v\ncells=%v", seed, v.Solved(b), bruteSolved(cells), cells)
		}
	}
}

// ============================================================================
// C. Gotcha audit — box geometry near-miss + independent cell→box mapping
// ============================================================================

// TestGotcha_BoxNotThreeByTwo_NearMiss is the "should-not-trigger" companion to
// the positive box-geometry tests. Cells (0,1) and (2,0) hold the same digit;
// they are in DIFFERENT 2×3 boxes (boxes 0 and 2) so must NOT raise a box
// violation — but they WOULD share a box under a wrong 3×2 geometry. This nails
// the 2×3-vs-3×2 gotcha from the should-not-fire side. (The positive direction,
// distinguishing 2×3 from both 3×2 and 2×2, is covered by the (0,0)+(1,2)
// tests in minisudoku_test.go.)
func TestGotcha_BoxNotThreeByTwo_NearMiss(t *testing.T) {
	cells := make([]int, N*N)
	cells[engine.Index(engine.Cell{Row: 0, Col: 1}, N)] = 5
	cells[engine.Index(engine.Cell{Row: 2, Col: 0}, N)] = 5
	b := Board{Cells: cells}

	v := Validator{}
	for _, viol := range v.Violations(b) {
		if viol.Rule == "box" {
			t.Errorf("cells (0,1) and (2,0) are in different 2×3 boxes and must not raise a box violation (would only collide under a wrong 3×2 geometry)")
		}
	}
	// Sanity: the SAME two cells must be flagged if the box really were 3×2.
	// Independently confirm they are in different 2×3 boxes here.
	if boxID(0, 1, BoxH, BoxW, N) == boxID(2, 0, BoxH, BoxW, N) {
		t.Fatal("fixture assumption broken: (0,1) and (2,0) unexpectedly share a 2×3 box")
	}
}

// TestGotcha_CellToBoxMappingIndependent asserts the shared boxID helper agrees
// with an independently derived 2×3 partition for every cell, and spot-checks
// representative mappings. This is the "assert the mapping cell→box" the spec
// gotcha asks for.
func TestGotcha_CellToBoxMappingIndependent(t *testing.T) {
	for row := 0; row < N; row++ {
		for col := 0; col < N; col++ {
			got := boxID(row, col, BoxH, BoxW, N)
			want := xcheckBoxIndex(row, col, BoxH, BoxW, N)
			if got != want {
				t.Errorf("boxID(%d,%d)=%d, independent 2×3 partition=%d", row, col, got, want)
			}
		}
	}
	// Representative expectations for the canonical 2×3 layout (six boxes,
	// numbered band-major, each 2 rows tall and 3 cols wide).
	cases := []struct {
		row, col, box int
	}{
		{0, 0, 0}, {1, 2, 0}, // box 0: rows 0-1, cols 0-2
		{0, 3, 1}, {1, 5, 1}, // box 1: rows 0-1, cols 3-5
		{2, 0, 2}, {3, 2, 2}, // box 2: rows 2-3, cols 0-2
		{2, 3, 3}, {3, 5, 3}, // box 3
		{4, 0, 4}, {5, 2, 4}, // box 4: rows 4-5, cols 0-2
		{4, 3, 5}, {5, 5, 5}, // box 5
	}
	for _, c := range cases {
		if got := boxID(c.row, c.col, BoxH, BoxW, N); got != c.box {
			t.Errorf("boxID(%d,%d)=%d, want %d", c.row, c.col, got, c.box)
		}
	}
}

// ============================================================================
// D. Canonicalization / dedup — real transform invariance (the shipped
//    placeholder tests never actually apply a transform)
// ============================================================================

// transformPuzzle rebuilds p under a band/stack row permutation, a band/stack
// column permutation, a box-preserving dihedral transform, and a digit
// relabeling — exactly the composition the Fingerprinter's Canonical enumerates
// over. Any such image must therefore fingerprint identically to p.
func transformPuzzle(p Puzzle, t engine.Transform, rowPerm, colPerm []int, digitPerm map[int]int) Puzzle {
	n := p.N
	grid := make([]int, n*n)
	for idx, v := range p.Givens {
		grid[idx] = v
	}
	out := make([]int, n*n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			src := grid[rowPerm[i]*n+colPerm[j]]
			if src != 0 && digitPerm != nil {
				src = digitPerm[src]
			}
			dst := t.Apply(engine.Cell{Row: i, Col: j}, n, n)
			out[dst.Row*n+dst.Col] = src
		}
	}
	givens := make(map[int]int)
	for idx, v := range out {
		if v != 0 {
			givens[idx] = v
		}
	}
	return Puzzle{N: n, BoxH: p.BoxH, BoxW: p.BoxW, Givens: givens, SeedVal: p.SeedVal, Diff: p.Diff}
}

// TestCanonical_TransformInvariance asserts a generated puzzle and every
// symmetry image (box-preserving dihedral × a nontrivial band/stack permutation
// × a digit relabeling) share one fingerprint. This is the property that makes
// dedup meaningful, and it replaces the shipped placeholder test that never
// actually transformed anything.
func TestCanonical_TransformInvariance(t *testing.T) {
	f := Fingerprinter{}
	g := Generator{}

	// A nontrivial band/stack row permutation (bands reordered, rows swapped
	// within bands) and column permutation (stacks swapped, cols permuted
	// within stacks) — both members of the fingerprinter's own perm group.
	rowPerm := []int{3, 2, 1, 0, 5, 4}
	colPerm := []int{4, 5, 3, 2, 0, 1}
	digitPerm := map[int]int{1: 6, 2: 5, 3: 4, 4: 3, 5: 2, 6: 1}
	boxPreserving := []engine.Transform{engine.Identity, engine.Rot180, engine.FlipH, engine.FlipV}

	seeds := seedCount(20)
	if seeds > 30 {
		seeds = 30 // this test is about invariance, not volume — keep it quick
	}
	for seed := int64(1); seed <= int64(seeds); seed++ {
		r := engine.NewRand(seed)
		p, _, err := g.Generate(engine.Medium, r)
		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		want := f.Fingerprint(p)

		for _, tr := range boxPreserving {
			// Transform alone.
			img := transformPuzzle(p, tr, []int{0, 1, 2, 3, 4, 5}, []int{0, 1, 2, 3, 4, 5}, nil)
			if f.Fingerprint(img) != want {
				t.Errorf("seed %d: transform %v alone changed the fingerprint", seed, tr)
			}
			// Transform composed with band/stack perms + digit relabel.
			img2 := transformPuzzle(p, tr, rowPerm, colPerm, digitPerm)
			if f.Fingerprint(img2) != want {
				t.Errorf("seed %d: transform %v + band/stack perms + relabel changed the fingerprint", seed, tr)
			}
		}
	}
}

// TestCanonical_DigitRelabelSameFingerprint isolates the "digit labels are
// symbols" gotcha: relabeling every digit by a bijection must not change the
// fingerprint (the canonicalizer normalizes by first appearance). This is the
// real assertion the shipped TestGotcha_DigitLabelsAreSymbols placeholder omits.
func TestCanonical_DigitRelabelSameFingerprint(t *testing.T) {
	f := Fingerprinter{}
	p := Puzzle{
		N: N, BoxH: BoxH, BoxW: BoxW,
		Givens: map[int]int{0: 1, 1: 2, 5: 6, 6: 4, 11: 3, 12: 2, 17: 4, 18: 5, 23: 1, 24: 3, 29: 5, 30: 6, 35: 2},
		Diff:   engine.Easy,
	}
	identity := transformPuzzle(p, engine.Identity, []int{0, 1, 2, 3, 4, 5}, []int{0, 1, 2, 3, 4, 5}, nil)
	relabeled := transformPuzzle(p, engine.Identity, []int{0, 1, 2, 3, 4, 5}, []int{0, 1, 2, 3, 4, 5},
		map[int]int{1: 4, 2: 6, 3: 1, 4: 2, 5: 3, 6: 5})

	if f.Fingerprint(identity) != f.Fingerprint(p) {
		t.Fatal("identity transform changed the fingerprint")
	}
	if f.Fingerprint(relabeled) != f.Fingerprint(p) {
		t.Error("digit relabeling changed the fingerprint; canonicalizer must relabel digits as symbols")
	}
}

// TestCanonical_DifferentPuzzlesDifferentFingerprint is the near-miss for the
// canonicalizer: genuinely different clue sets must NOT collide. A puzzle and
// the same puzzle with one extra given differ in clue count, so their canonical
// serializations (and fingerprints) must differ; two different generated
// puzzles must differ too.
func TestCanonical_DifferentPuzzlesDifferentFingerprint(t *testing.T) {
	f := Fingerprinter{}

	// Same givens plus one extra clue at an empty, consistent cell.
	givens := map[int]int{0: 1, 1: 2, 5: 6, 6: 4, 11: 3, 12: 2, 17: 4, 18: 5, 23: 1, 24: 3, 29: 5, 30: 6, 35: 2}
	p := Puzzle{N: N, BoxH: BoxH, BoxW: BoxW, Givens: givens, Diff: engine.Easy}

	extra := make(map[int]int, len(givens)+1)
	for k, v := range givens {
		extra[k] = v
	}
	extra[7] = 5 // cell (1,1); not previously given
	pExtra := Puzzle{N: N, BoxH: BoxH, BoxW: BoxW, Givens: extra, Diff: engine.Easy}

	if f.Fingerprint(p) == f.Fingerprint(pExtra) {
		t.Error("puzzles with different clue counts must not share a fingerprint")
	}

	// Two different generated puzzles differ.
	g := Generator{}
	p1, _, err1 := g.Generate(engine.Easy, engine.NewRand(1))
	p2, _, err2 := g.Generate(engine.Easy, engine.NewRand(2))
	if err1 != nil || err2 != nil {
		t.Fatalf("Generate failed: %v / %v", err1, err2)
	}
	if f.Fingerprint(p1) == f.Fingerprint(p2) {
		t.Error("two independently generated puzzles unexpectedly share a fingerprint")
	}
}

// TestCanonical_NonBoxPreservingTransformSafe documents and guards the SPEC
// deviation: a 2×3 box is not square, so Rot90/Rot270/transpose turn each box
// into an invalid 3×2 region — they are NOT symmetries of Mini Sudoku and are
// (correctly) excluded from the canonicalization group. This test only asserts
// the fingerprinter stays well-defined (no panic) on such transforms; it does
// NOT assert invariance, because a rotated puzzle is a different game object.
func TestCanonical_NonBoxPreservingTransformSafe(t *testing.T) {
	f := Fingerprinter{}
	p := Puzzle{
		N: N, BoxH: BoxH, BoxW: BoxW,
		Givens: map[int]int{0: 1, 1: 2, 5: 6, 6: 4, 11: 3, 12: 2},
		Diff:   engine.Easy,
	}
	for _, tr := range []engine.Transform{engine.Rot90, engine.Rot270, engine.FlipMain, engine.FlipAnti} {
		img := transformPuzzle(p, tr, []int{0, 1, 2, 3, 4, 5}, []int{0, 1, 2, 3, 4, 5}, nil)
		_ = f.Fingerprint(img) // must not panic
	}
}

// ============================================================================
// E. Determinism — re-running with a fixed seed yields an identical puzzle,
//    including the canonical fingerprint (belt-and-suspenders over the shipped
//    determinism tests, now also checking the fingerprint).
// ============================================================================

func TestCrosscheck_DeterministicFingerprint(t *testing.T) {
	g := Generator{}
	f := Fingerprinter{}
	for _, diff := range []engine.Difficulty{engine.Easy, engine.Medium, engine.Hard} {
		p1, _, err1 := g.Generate(diff, engine.NewRand(4242))
		p2, _, err2 := g.Generate(diff, engine.NewRand(4242))
		if err1 != nil || err2 != nil {
			t.Fatalf("diff %s: Generate failed: %v / %v", diff, err1, err2)
		}
		if f.Fingerprint(p1) != f.Fingerprint(p2) {
			t.Errorf("diff %s: same seed produced different fingerprints", diff)
		}
	}
}
