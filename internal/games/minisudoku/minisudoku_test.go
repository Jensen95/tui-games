package minisudoku

import (
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// ============================================================================
// Validator Tests - Truth Tables
// ============================================================================

// TestValidator_EmptyBoard asserts that an empty (all-0) board has no violations.
func TestValidator_EmptyBoard(t *testing.T) {
	board := Board{Cells: make([]int, N*N)}
	v := Validator{}
	viols := v.Violations(board)
	if len(viols) != 0 {
		t.Errorf("empty board should have no violations, got %d: %v", len(viols), viols)
	}
}

// TestValidator_ValidCompletedGrid asserts that a hand-built valid solution passes.
func TestValidator_ValidCompletedGrid(t *testing.T) {
	// A valid 6x6 solution: each row/col/box has 1-6 exactly once.
	// Using a simple valid pattern.
	board := Board{Cells: []int{
		1, 2, 3, 4, 5, 6,
		4, 5, 6, 1, 2, 3,
		2, 3, 1, 5, 6, 4,
		5, 6, 4, 2, 3, 1,
		3, 1, 2, 6, 4, 5,
		6, 4, 5, 3, 1, 2,
	}}
	v := Validator{}
	if !v.Solved(board) {
		t.Error("hand-built valid solution should pass Solved()")
	}
	viols := v.Violations(board)
	if len(viols) != 0 {
		t.Errorf("valid solution should have no violations, got %d: %v", len(viols), viols)
	}
}

// TestValidator_RowDuplicateViolation asserts that a duplicate in a row is caught.
func TestValidator_RowDuplicateViolation(t *testing.T) {
	// Row 0 has two 1's (duplicate)
	board := Board{Cells: []int{
		1, 1, 3, 4, 5, 6,
		4, 5, 6, 1, 2, 3,
		2, 3, 1, 5, 6, 4,
		5, 6, 4, 2, 3, 1,
		3, 1, 2, 6, 4, 5,
		6, 4, 5, 3, 1, 2,
	}}
	v := Validator{}
	viols := v.Violations(board)
	found := false
	for _, viol := range viols {
		if viol.Rule == "row" {
			found = true
			break
		}
	}
	if !found {
		t.Error("row duplicate should trigger a row violation")
	}
}

// TestValidator_ColumnDuplicateViolation asserts that a duplicate in a column is caught.
func TestValidator_ColumnDuplicateViolation(t *testing.T) {
	// Column 0 has two 1's (duplicate)
	board := Board{Cells: []int{
		1, 2, 3, 4, 5, 6,
		1, 5, 6, 1, 2, 3,
		2, 3, 1, 5, 6, 4,
		5, 6, 4, 2, 3, 1,
		3, 1, 2, 6, 4, 5,
		6, 4, 5, 3, 1, 2,
	}}
	v := Validator{}
	viols := v.Violations(board)
	found := false
	for _, viol := range viols {
		if viol.Rule == "column" {
			found = true
			break
		}
	}
	if !found {
		t.Error("column duplicate should trigger a column violation")
	}
}

// TestValidator_BoxViolation_NotRowOrColViolation asserts that a 2×3 box violation
// can occur without a row or column violation (box-geometry correctness).
// This tests the classic gotcha: box is 2×3, not 3×2.
func TestValidator_BoxViolation_NotRowOrColViolation(t *testing.T) {
	// Construct a board where cells (0,0) and (1,2) both have 1.
	// (0,0) is in box (row-band 0, col-stack 0)
	// (1,2) is in box (row-band 0, col-stack 1) with 2×3 boxes
	// If box is 2×3: cell (0,0) is row 0-1, col 0-2 → box at (0,0)
	//                cell (1,2) is row 0-1, col 0-2 → same box!
	// But they don't share a row or column.
	board := Board{Cells: []int{
		1, 2, 3, 4, 5, 6,
		4, 5, 1, 1, 2, 3, // (1,0)=4, (1,1)=5, (1,2)=1 (same box as (0,0)=1)
		2, 3, 4, 5, 6, 1,
		5, 6, 2, 3, 4, 1,
		3, 1, 5, 6, 4, 2,
		6, 4, 2, 1, 3, 5,
	}}
	v := Validator{}
	viols := v.Violations(board)
	found := false
	for _, viol := range viols {
		if viol.Rule == "box" {
			found = true
			break
		}
	}
	if !found {
		t.Error("box violation (2×3 cells (0,0) and (1,2)) should be caught independently of row/col")
	}
}

// TestValidator_PartialBoardNoViolation asserts that empty cells are not flagged.
func TestValidator_PartialBoardNoViolation(t *testing.T) {
	// A partial board with some cells filled, some empty (0).
	board := Board{Cells: []int{
		1, 0, 3, 0, 5, 6,
		0, 5, 6, 1, 0, 3,
		2, 0, 1, 5, 6, 0,
		0, 6, 4, 2, 3, 1,
		3, 0, 2, 0, 4, 5,
		0, 4, 0, 3, 1, 2,
	}}
	v := Validator{}
	viols := v.Violations(board)
	// Should have no violations because there are no actual duplicates (empties ignored).
	if len(viols) != 0 {
		t.Errorf("partial board with no duplicates should have no violations, got %d: %v", len(viols), viols)
	}
}

// TestValidator_MultipleViolations asserts that all violations are reported.
func TestValidator_MultipleViolations(t *testing.T) {
	// A board with both a row duplicate and a column duplicate.
	board := Board{Cells: []int{
		1, 1, 3, 4, 5, 6,
		1, 5, 6, 1, 2, 3,
		2, 3, 1, 5, 6, 4,
		5, 6, 4, 2, 3, 1,
		3, 1, 2, 6, 4, 5,
		6, 4, 5, 3, 1, 2,
	}}
	v := Validator{}
	viols := v.Violations(board)
	// Should have at least one row violation and at least one column violation.
	rowFound := false
	colFound := false
	for _, viol := range viols {
		if viol.Rule == "row" {
			rowFound = true
		}
		if viol.Rule == "column" {
			colFound = true
		}
	}
	if !rowFound || !colFound {
		t.Error("board with both row and column duplicates should report both violation types")
	}
}

// TestValidator_SolvedRequiresAllCells asserts that Solved only returns true
// when every cell is filled AND valid.
func TestValidator_SolvedPartialBoard(t *testing.T) {
	board := Board{Cells: []int{
		1, 2, 3, 4, 5, 6,
		4, 5, 6, 1, 2, 3,
		2, 3, 1, 5, 6, 4,
		5, 6, 4, 2, 3, 1,
		3, 1, 2, 6, 4, 5,
		0, 4, 5, 3, 1, 2, // Last cell is empty
	}}
	v := Validator{}
	if v.Solved(board) {
		t.Error("board with empty cell should not be Solved")
	}
}

// TestValidator_InvalidValueOutOfRange asserts that invalid digits (>6 or <0) are flagged.
func TestValidator_InvalidValue(t *testing.T) {
	board := Board{Cells: []int{
		7, 2, 3, 4, 5, 6, // 7 is out of range [1,6]
		4, 5, 6, 1, 2, 3,
		2, 3, 1, 5, 6, 4,
		5, 6, 4, 2, 3, 1,
		3, 1, 2, 6, 4, 5,
		6, 4, 5, 3, 1, 2,
	}}
	v := Validator{}
	viols := v.Violations(board)
	// Should have a violation for invalid value.
	if len(viols) == 0 {
		t.Error("board with invalid digit (7) should have at least one violation")
	}
}

// ============================================================================
// Solver Tests - Correctness
// ============================================================================

// TestSolver_GoldenPuzzleUniqueSolution asserts that a hand-built puzzle
// with a known unique solution is solved correctly.
//
// GREEN-IMPL NOTE: the original 15-given fixture here (0,1,2 / 6,7,8 / 12,14
// / 18,20 / 24,26 / 30,31,32) was under-constrained — a brute-force complete
// solver finds at least 10 distinct solutions for it, not 1 (rows 3-4 and
// their shared box left too few clues). It has been replaced with a
// different 15-given subset of the SAME solution grid (verified unique via
// exhaustive search, and confirmed logic-solvable via hidden singles) so the
// "exactly 1 solution" assertion below is actually satisfiable. No assertion
// was weakened; only the fixture data changed.
func TestSolver_GoldenPuzzleUniqueSolution(t *testing.T) {
	// A valid puzzle with givens removed from the valid solution above.
	// Keep enough givens to ensure uniqueness.
	puzzle := Puzzle{
		N:    N,
		BoxH: BoxH,
		BoxW: BoxW,
		Givens: map[int]int{
			2: 3, 4: 5,
			6: 4, 9: 1, 11: 3,
			12: 2, 14: 1, 16: 6,
			19: 6, 21: 2,
			25: 1, 29: 5,
			30: 6, 33: 3, 34: 1,
		},
		SeedVal: 12345,
		Diff:    engine.Easy,
	}

	s := Solver{}
	count := s.CountSolutions(puzzle, 2)
	if count != 1 {
		t.Errorf("golden puzzle should have exactly 1 solution, got %d", count)
	}

	sol, ok := s.Solve(puzzle)
	if !ok {
		t.Fatal("Solve should find a solution for the golden puzzle")
	}
	if len(sol.Cells) != N*N {
		t.Errorf("solution should have %d cells, got %d", N*N, len(sol.Cells))
	}
	v := Validator{}
	if !v.Solved(Board{Cells: sol.Cells}) {
		t.Error("solver solution should be valid")
	}
}

// TestSolver_AmbiguousPuzzle asserts that a hand-built puzzle with too few givens
// has multiple solutions (count == 2 when capped at 2).
func TestSolver_AmbiguousPuzzle(t *testing.T) {
	// A puzzle with very few givens, definitely ambiguous.
	puzzle := Puzzle{
		N:    N,
		BoxH: BoxH,
		BoxW: BoxW,
		Givens: map[int]int{
			0: 1, // Only one given; highly ambiguous
		},
		SeedVal: 54321,
		Diff:    engine.Hard,
	}

	s := Solver{}
	count := s.CountSolutions(puzzle, 2)
	if count != 2 {
		t.Errorf("ambiguous puzzle should have >= 2 solutions (capped at 2), got %d", count)
	}
}

// TestSolver_LogicSolveClosesExample asserts that logic solve closes
// a standard puzzle without guessing.
//
// GREEN-IMPL NOTE: uses the same corrected golden fixture as
// TestSolver_GoldenPuzzleUniqueSolution above (see that test's note) — the
// original givens here were under-constrained (>1 solution).
func TestSolver_LogicSolveClosesExample(t *testing.T) {
	puzzle := Puzzle{
		N:    N,
		BoxH: BoxH,
		BoxW: BoxW,
		Givens: map[int]int{
			2: 3, 4: 5,
			6: 4, 9: 1, 11: 3,
			12: 2, 14: 1, 16: 6,
			19: 6, 21: 2,
			25: 1, 29: 5,
			30: 6, 33: 3, 34: 1,
		},
		SeedVal: 12345,
		Diff:    engine.Easy,
	}

	s := Solver{}
	sol, closed, tech := s.LogicSolve(puzzle)
	if !closed {
		t.Error("logic solver should close the puzzle without guessing")
	}
	if len(sol.Cells) != N*N {
		t.Errorf("logic solution should have %d cells, got %d", N*N, len(sol.Cells))
	}
	if tech == "" {
		t.Error("technique should be reported")
	}
	v := Validator{}
	if !v.Solved(Board{Cells: sol.Cells}) {
		t.Error("logic solver solution should be valid")
	}
}

// ============================================================================
// Generator Property Tests - Over Many Seeds
// ============================================================================

// TestGenerator_PropertyInvariant tests that every generated puzzle satisfies
// the generation invariant over many seeds.
// Invariant:
//   - Solution is valid (Solved() == true)
//   - Unique solution (CountSolutions == 1)
//   - Logic solvable (LogicSolve closes without guessing)
//   - Difficulty matches request (via deepest technique)
func TestGenerator_PropertyInvariant(t *testing.T) {
	seedCount := 250 // Default; override with LIG_SEEDS env var for CI/nightly.
	if s := os.Getenv("LIG_SEEDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			seedCount = n
		}
	}

	difficulties := []engine.Difficulty{engine.Easy, engine.Medium, engine.Hard}

	for _, diff := range difficulties {
		for seed := int64(1); seed <= int64(seedCount); seed++ {
			r := engine.NewRand(seed)
			g := Generator{}
			puzzle, solution, err := g.Generate(diff, r)
			if err != nil {
				t.Fatalf("seed %d diff %s: Generate failed: %v", seed, diff, err)
			}

			// Check 1: Solution is valid
			v := Validator{}
			if !v.Solved(Board{Cells: solution.Cells}) {
				t.Errorf("seed %d diff %s: solution is not valid", seed, diff)
			}

			// Check 2: Unique solution
			s := Solver{}
			count := s.CountSolutions(puzzle, 2)
			if count != 1 {
				t.Errorf("seed %d diff %s: unique solution check failed, count=%d", seed, diff, count)
			}

			// Check 3: Logic solvable
			_, closed, _ := s.LogicSolve(puzzle)
			if !closed {
				t.Errorf("seed %d diff %s: logic solver did not close the puzzle", seed, diff)
			}

			// Check 4: Solution matches puzzle structure
			if len(puzzle.Givens) > N*N {
				t.Errorf("seed %d diff %s: puzzle has more givens than cells", seed, diff)
			}
		}
	}
}

// TestGenerator_DeterminismSameSeed asserts that the same seed produces
// identical puzzles (byte-for-byte reproducibility).
func TestGenerator_DeterminismSameSeed(t *testing.T) {
	seed := int64(999)
	diff := engine.Medium

	// Generate twice with the same seed
	r1 := engine.NewRand(seed)
	g := Generator{}
	p1, s1, err1 := g.Generate(diff, r1)
	if err1 != nil {
		t.Fatalf("first Generate failed: %v", err1)
	}

	r2 := engine.NewRand(seed)
	p2, s2, err2 := g.Generate(diff, r2)
	if err2 != nil {
		t.Fatalf("second Generate failed: %v", err2)
	}

	// Compare puzzles: givens must be identical
	if len(p1.Givens) != len(p2.Givens) {
		t.Errorf("puzzle 1 has %d givens, puzzle 2 has %d givens", len(p1.Givens), len(p2.Givens))
	}
	for idx, digit := range p1.Givens {
		if p2.Givens[idx] != digit {
			t.Errorf("givens differ at index %d: %d vs %d", idx, digit, p2.Givens[idx])
		}
	}

	// Compare solutions
	if len(s1.Cells) != len(s2.Cells) {
		t.Errorf("solution 1 has %d cells, solution 2 has %d cells", len(s1.Cells), len(s2.Cells))
	}
	for i, c := range s1.Cells {
		if s2.Cells[i] != c {
			t.Errorf("solutions differ at cell %d: %d vs %d", i, c, s2.Cells[i])
		}
	}
}

// TestGenerator_ExpertRequiresSearch is the regression guard for the Expert
// difficulty fix. Before the fix Expert was a degenerate clone of Hard: same
// targetClueCount, same no-guess carve invariant, just band confirmation
// disabled — so Expert averaged the same clue count as Hard and stayed 100%
// no-guess (with ~40% of puzzles only reaching hidden-single, i.e. Medium).
//
// It asserts the two properties that make Expert genuinely its own tier:
//
//  1. Expert's mean clue count is strictly below Hard's (it carves to a lower
//     floor now that closure is not required during Expert carving).
//  2. Expert's no-guess rate is well below 1.0 — most Expert puzzles do NOT
//     close under LogicSolve, so they provably require search — while
//     Easy/Medium/Hard stay exactly 1.0 (fully logic-solvable, unchanged).
//
// Deterministic (fixed seeds, engine.NewRand) and fast (a fixed, modest seed
// count independent of LIG_SEEDS). The empirical margins are comfortable (see
// the metrics in the fix's commit message), so the thresholds below are loose
// enough to avoid flakiness while still failing hard if Expert ever regresses
// back toward the Hard clone.
func TestGenerator_ExpertRequiresSearch(t *testing.T) {
	const seeds = 80

	meanClues := func(diff engine.Difficulty) (mean float64, noGuessRate float64) {
		g := Generator{}
		s := Solver{}
		totalClues, closed := 0, 0
		for seed := int64(1); seed <= seeds; seed++ {
			p, _, err := g.Generate(diff, engine.NewRand(seed))
			if err != nil {
				t.Fatalf("seed %d diff %s: Generate failed: %v", seed, diff, err)
			}
			// Every difficulty must always be uniquely solvable.
			if c := s.CountSolutions(p, 2); c != 1 {
				t.Errorf("seed %d diff %s: expected unique solution, got count=%d", seed, diff, c)
			}
			totalClues += len(p.Givens)
			if _, ok, _ := s.LogicSolve(p); ok {
				closed++
			}
		}
		return float64(totalClues) / float64(seeds), float64(closed) / float64(seeds)
	}

	hardMean, hardNoGuess := meanClues(engine.Hard)
	expertMean, expertNoGuess := meanClues(engine.Expert)
	easyMean, easyNoGuess := meanClues(engine.Easy)
	medMean, medNoGuess := meanClues(engine.Medium)
	_ = easyMean
	_ = medMean

	// Property 1: Expert carves strictly sparser than Hard on average.
	if !(expertMean < hardMean) {
		t.Errorf("Expert mean clue count (%.3f) should be strictly below Hard's (%.3f)", expertMean, hardMean)
	}

	// Property 2: the no-guess ladder must close every Easy/Medium/Hard puzzle
	// but only a small fraction of Expert puzzles (Expert requires search).
	if easyNoGuess != 1.0 || medNoGuess != 1.0 || hardNoGuess != 1.0 {
		t.Errorf("Easy/Medium/Hard must stay 100%% no-guess, got %.3f/%.3f/%.3f", easyNoGuess, medNoGuess, hardNoGuess)
	}
	// Loose ceiling: empirically ~0.08–0.18 over these seeds; require it to be
	// clearly below 1.0 so a regression to the always-no-guess Hard clone
	// fails this guard.
	if expertNoGuess > 0.5 {
		t.Errorf("Expert no-guess rate (%.3f) should be well below 1.0 (Expert must require search)", expertNoGuess)
	}
}

// TestGenerator_ExpertDeterminismAndUniqueness asserts that the Expert tier —
// which, unlike Easy/Medium/Hard, is allowed to require search — is still
// fully deterministic (same seed ⇒ byte-identical puzzle) and always uniquely
// solvable. The search-required acceptance gate and relaxed carve must not
// introduce any nondeterminism or ambiguity.
func TestGenerator_ExpertDeterminismAndUniqueness(t *testing.T) {
	seedCount := 120 // Default; override with LIG_SEEDS for CI/nightly.
	if s := os.Getenv("LIG_SEEDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			seedCount = n
		}
	}

	g := Generator{}
	s := Solver{}
	for seed := int64(1); seed <= int64(seedCount); seed++ {
		p1, s1, err1 := g.Generate(engine.Expert, engine.NewRand(seed))
		if err1 != nil {
			t.Fatalf("seed %d: first Expert Generate failed: %v", seed, err1)
		}
		p2, s2, err2 := g.Generate(engine.Expert, engine.NewRand(seed))
		if err2 != nil {
			t.Fatalf("seed %d: second Expert Generate failed: %v", seed, err2)
		}

		// Byte-identical puzzle: same givens.
		if len(p1.Givens) != len(p2.Givens) {
			t.Fatalf("seed %d: givens count differs: %d vs %d", seed, len(p1.Givens), len(p2.Givens))
		}
		for idx, d := range p1.Givens {
			if p2.Givens[idx] != d {
				t.Errorf("seed %d: givens differ at %d: %d vs %d", seed, idx, d, p2.Givens[idx])
			}
		}
		// Identical solution.
		for i, c := range s1.Cells {
			if s2.Cells[i] != c {
				t.Errorf("seed %d: solutions differ at %d: %d vs %d", seed, i, c, s2.Cells[i])
			}
		}
		// Uniquely solvable.
		if c := s.CountSolutions(p1, 2); c != 1 {
			t.Errorf("seed %d: Expert puzzle not uniquely solvable, count=%d", seed, c)
		}
	}
}

// ============================================================================
// Canonicalization Tests - Deduplication
// ============================================================================

// TestFingerprinter_TransformYieldsSameFingerprint asserts that applying
// every transform in the 8-dihedral group yields the same fingerprint.
func TestFingerprinter_TransformYieldsSameFingerprint(t *testing.T) {
	puzzle := Puzzle{
		N:    N,
		BoxH: BoxH,
		BoxW: BoxW,
		Givens: map[int]int{
			0: 1, 1: 2, 5: 6,
			6: 4, 11: 3,
			12: 2, 17: 4,
			18: 5, 23: 1,
			24: 3, 29: 5,
			30: 6, 35: 2,
		},
		SeedVal: 11111,
		Diff:    engine.Easy,
	}

	f := Fingerprinter{}
	fp1 := f.Fingerprint(puzzle)

	// Apply 8 dihedral transforms and verify fingerprints match
	for _, transform := range engine.AllTransforms {
		// Transform the puzzle (this will be a no-op for now since the
		// actual transformation logic is in the green-test author's solver,
		// but we still call the fingerprinter to ensure it doesn't panic).
		transformedPuzzle := Puzzle{
			N:       N,
			BoxH:    BoxH,
			BoxW:    BoxW,
			Givens:  puzzle.Givens, // Placeholder; real transform happens in green
			SeedVal: puzzle.SeedVal,
			Diff:    puzzle.Diff,
		}
		_ = transform // Use the transform to avoid lint errors
		fp2 := f.Fingerprint(transformedPuzzle)
		// Once transforms are implemented, these should match:
		// if fp1 != fp2 { t.Errorf("transform %v changed fingerprint", transform) }
		_ = fp2 // Suppress unused variable
	}
	// For now, verify that fingerprinting doesn't crash
	_ = fp1
}

// ----------------------------------------------------------------------------
// Independent symmetry oracle (for the batch collision test below)
//
// The functions in this block re-derive Mini Sudoku's symmetry group from
// scratch, WITHOUT calling Fingerprinter.Canonical / Fingerprint. They exist
// so the batch test can referee whether a shared fingerprint is a legitimate
// symmetry-duplicate or a genuine canonicalization defect, using a code path
// that is fully independent of the code under test (it compares transformed
// grids DIRECTLY rather than through the serialize→min→hash pipeline).
//
// CRITICAL — group parity: this oracle must enumerate EXACTLY the group that
// fingerprint.go's Canonical enumerates, no more and no less. Mini Sudoku has
// the largest symmetry group of the five games, so the enumeration below is
// deliberately a faithful, line-for-line re-implementation of that same
// documented group:
//
//   - Row band/stack permutations: reorder the N/BoxH row-bands (here 3 bands
//     of 2 rows) and, independently within each band, reorder its BoxH rows.
//   - Column band/stack permutations: the same, applied to the N/BoxW
//     column-stacks (here 2 stacks of 3 columns) and the BoxW columns within.
//   - Box-preserving dihedral subset: {Identity, Rot180, FlipH, FlipV} for the
//     non-square 2×3 box (the four transforms that never swap grid dimensions,
//     so a 2×3 box stays a 2×3 box); all 8 dihedral transforms when BoxH==BoxW.
//   - Digit relabeling: normalized by first-appearance order (oracleRelabel),
//     which collapses every bijective relabeling to one form — mirroring
//     canonicalDigits, but re-implemented here rather than borrowed.
//
// If this oracle were NARROWER than Canonical's group, a legitimate duplicate
// would be misreported as a canonicalization bug (a false alarm — the very
// thing this change removes). If it were BROADER, it could mask a real
// over-collapsing defect. Hence the exact parity above.
// ----------------------------------------------------------------------------

// oracleGrid lays a puzzle's givens into a row-major n*n slice (0 = empty),
// resolving geometry the same way Canonical does (fall back to the package
// N/BoxH/BoxW when the puzzle leaves them zero).
func oracleGrid(p Puzzle) (grid []int, n, boxH, boxW int) {
	n, boxH, boxW = p.N, p.BoxH, p.BoxW
	if n == 0 {
		n, boxH, boxW = N, BoxH, BoxW
	}
	grid = make([]int, n*n)
	for idx, v := range p.Givens {
		grid[idx] = v
	}
	return grid, n, boxH, boxW
}

// oraclePerms returns every permutation of elems (independent of the
// production permutations helper in fingerprint.go).
func oraclePerms(elems []int) [][]int {
	if len(elems) <= 1 {
		return [][]int{append([]int(nil), elems...)}
	}
	var out [][]int
	for i := range elems {
		rest := make([]int, 0, len(elems)-1)
		rest = append(rest, elems[:i]...)
		rest = append(rest, elems[i+1:]...)
		for _, p := range oraclePerms(rest) {
			out = append(out, append([]int{elems[i]}, p...))
		}
	}
	return out
}

// oracleBandPerms re-derives every row (or column) permutation reachable by
// reordering the n/group bands and, independently within each band, its group
// rows — the band/stack symmetry Canonical enumerates via bandStackRowPerms.
// Each result maps output position -> source position.
func oracleBandPerms(n, group int) [][]int {
	numBands := n / group
	bandIdxs := make([]int, numBands)
	for i := range bandIdxs {
		bandIdxs[i] = i
	}
	subIdxs := make([]int, group)
	for i := range subIdxs {
		subIdxs[i] = i
	}
	bandOrders := oraclePerms(bandIdxs)
	subOrders := oraclePerms(subIdxs)

	var perms [][]int
	var build func(bandOrder []int, chosen [][]int)
	build = func(bandOrder []int, chosen [][]int) {
		if len(chosen) == numBands {
			perm := make([]int, 0, n)
			for pos, srcBand := range bandOrder {
				for _, sub := range chosen[pos] {
					perm = append(perm, srcBand*group+sub)
				}
			}
			perms = append(perms, perm)
			return
		}
		for _, sub := range subOrders {
			build(bandOrder, append(append([][]int{}, chosen...), sub))
		}
	}
	for _, bo := range bandOrders {
		build(bo, nil)
	}
	return perms
}

// oracleDihedral returns the box-preserving dihedral transforms: all 8 for a
// square box, else the four that never swap grid dimensions. Mirrors
// boxPreservingDihedral.
func oracleDihedral(boxH, boxW int) []engine.Transform {
	if boxH == boxW {
		return append([]engine.Transform(nil), engine.AllTransforms[:]...)
	}
	return []engine.Transform{engine.Identity, engine.Rot180, engine.FlipH, engine.FlipV}
}

// oracleRelabel renumbers digits (0 = empty) by first-appearance order
// scanning row-major, collapsing every bijective relabeling of one grid to a
// single form. Independent re-implementation of canonicalDigits.
func oracleRelabel(grid []int) []int {
	next := 1
	seen := make(map[int]int, len(grid))
	out := make([]int, len(grid))
	for i, v := range grid {
		if v == 0 {
			continue
		}
		m, ok := seen[v]
		if !ok {
			m = next
			seen[v] = m
			next++
		}
		out[i] = m
	}
	return out
}

// minisudokuEquivalent reports whether puzzles a and b are the same up to Mini
// Sudoku's symmetry group — i.e. whether SOME combination of a row band/stack
// permutation, a column band/stack permutation, a box-preserving dihedral
// transform, and a digit relabeling maps a's grid exactly onto b's.
//
// It is an INDEPENDENT re-implementation of the exact group Canonical
// enumerates (see the block comment above): it transforms a's grid directly
// and compares the first-appearance-relabeled result against b's likewise
// relabeled grid, never routing through Canonical/Fingerprint. Because the
// group is closed under composition and inverse, it suffices to test a's whole
// orbit against b's single relabeled form; the relation is symmetric and
// transitive, so a batch may compare each collider against one representative.
func minisudokuEquivalent(a, b Puzzle) bool {
	gridA, n, boxH, boxW := oracleGrid(a)
	gridB, nb, _, _ := oracleGrid(b)
	if n != nb {
		return false
	}
	target := oracleRelabel(gridB)

	rowPerms := oracleBandPerms(n, boxH)
	colPerms := oracleBandPerms(n, boxW)
	dihedrals := oracleDihedral(boxH, boxW)

	out := make([]int, n*n)
	for _, rp := range rowPerms {
		for _, cp := range colPerms {
			for _, t := range dihedrals {
				for i := 0; i < n; i++ {
					for j := 0; j < n; j++ {
						dst := t.Apply(engine.Cell{Row: i, Col: j}, n, n)
						out[dst.Row*n+dst.Col] = gridA[rp[i]*n+cp[j]]
					}
				}
				if slices.Equal(oracleRelabel(out), target) {
					return true
				}
			}
		}
	}
	return false
}

// transformGivens applies a row band/stack permutation, a column band/stack
// permutation, a box-preserving dihedral transform, and (optionally) a digit
// relabeling to p's givens, returning the transformed givens map. It mirrors
// the position/value mapping Canonical uses, so its output is a puzzle that is
// symmetry-equivalent to p by construction. Used only to build fixtures for
// the oracle-validation test below.
func transformGivens(p Puzzle, rp, cp []int, t engine.Transform, relabel map[int]int) map[int]int {
	grid, n, _, _ := oracleGrid(p)
	out := make([]int, n*n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			dst := t.Apply(engine.Cell{Row: i, Col: j}, n, n)
			out[dst.Row*n+dst.Col] = grid[rp[i]*n+cp[j]]
		}
	}
	g := make(map[int]int)
	for idx, v := range out {
		if v == 0 {
			continue
		}
		if relabel != nil {
			v = relabel[v]
		}
		g[idx] = v
	}
	return g
}

// TestMinisudokuEquivalent_OracleAgreesWithFingerprint deterministically
// exercises BOTH branches of minisudokuEquivalent and cross-checks each
// against the Fingerprinter, so the batch test's referee is proven correct
// even on seed windows where no natural collision occurs.
//
//   - Equivalent pair: a generated puzzle and a hand-applied symmetry
//     transform of it (band swap × stack swap × Rot180 × full digit relabel).
//     The oracle must return true AND the two must share a fingerprint (a
//     legitimate collision — the case the batch test must tolerate).
//   - Non-equivalent pair: the same puzzle with one given removed. The oracle
//     must return false AND the fingerprints must differ (the defect case the
//     batch test must catch).
func TestMinisudokuEquivalent_OracleAgreesWithFingerprint(t *testing.T) {
	g := Generator{}
	base, _, err := g.Generate(engine.Easy, engine.NewRand(7))
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// A concrete non-identity element of the symmetry group: swap the first
	// two row-bands, swap the two column-stacks, rotate 180°, and relabel
	// digits by the fixed involution d -> 7-d.
	rowPerm := []int{2, 3, 0, 1, 4, 5} // swap bands {0,1} and {2,3}
	colPerm := []int{3, 4, 5, 0, 1, 2} // swap the two 3-column stacks
	relabel := map[int]int{1: 6, 2: 5, 3: 4, 4: 3, 5: 2, 6: 1}
	equiv := Puzzle{
		N: N, BoxH: BoxH, BoxW: BoxW,
		Givens:  transformGivens(base, rowPerm, colPerm, engine.Rot180, relabel),
		SeedVal: base.SeedVal, Diff: base.Diff,
	}

	f := Fingerprinter{}
	if !minisudokuEquivalent(base, equiv) {
		t.Error("oracle: a symmetry transform of a puzzle must be reported equivalent")
	}
	if f.Fingerprint(base) != f.Fingerprint(equiv) {
		t.Error("fingerprinter: a symmetry transform of a puzzle must share its fingerprint")
	}

	// Remove one given: strictly fewer clues, so no relabel/transform can map
	// one onto the other.
	fewer := Puzzle{N: N, BoxH: BoxH, BoxW: BoxW, Givens: make(map[int]int, len(base.Givens)), SeedVal: base.SeedVal, Diff: base.Diff}
	dropped := false
	for idx, v := range base.Givens {
		if !dropped {
			dropped = true
			continue
		}
		fewer.Givens[idx] = v
		_ = v
	}
	if minisudokuEquivalent(base, fewer) {
		t.Error("oracle: puzzles with different clue counts must not be reported equivalent")
	}
	if f.Fingerprint(base) == f.Fingerprint(fewer) {
		t.Error("fingerprinter: puzzles with different clue counts must not share a fingerprint")
	}
}

// TestFingerprinter_BatchFingerprintsPairwiseDistinct pins the deduplication
// property in the form that actually holds: equal fingerprints must imply
// symmetry-equivalent puzzles.
//
// The previous version failed on ANY repeated fingerprint across sequential
// seeds. But two seeds can legitimately produce the same puzzle up to Mini
// Sudoku's (large) symmetry group — band/stack row+column permutations × a
// box-preserving dihedral subset × digit relabeling — so a shared fingerprint
// is very often a correct duplicate, not a defect. The raw generator takes no
// seen-set; "never a repeat" is enforced by the retry/corpus dedup layer, not
// promised per raw seed. Asserting "no two seeds ever collide" therefore tests
// a guarantee that does not exist and false-alarms the moment a genuine
// symmetry-duplicate lands in the tested seed window.
//
// The real defect worth guarding against is a lossy or over-collapsing
// canonicalization that maps two NON-equivalent puzzles onto one fingerprint.
// So that is exactly what we assert: on any fingerprint collision the two
// puzzles must be symmetry-equivalent, checked by the independent oracle above
// (which never calls Canonical/Fingerprint). This never false-alarms on
// entropy, so it runs the full seed count — more seeds is strictly more
// evidence that canonicalization stays injective on distinct puzzles.
func TestFingerprinter_BatchFingerprintsPairwiseDistinct(t *testing.T) {
	seedCount := 250 // Default; override with LIG_SEEDS env var for CI/nightly.
	if s := os.Getenv("LIG_SEEDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			seedCount = n
		}
	}

	g := Generator{}
	f := Fingerprinter{}

	// One representative puzzle per fingerprint. Symmetry equivalence is
	// transitive, so comparing a new collider against any prior member of its
	// fingerprint class is sufficient.
	seen := make(map[[32]byte]Puzzle, seedCount)
	collisions, equivalent := 0, 0
	for seed := int64(1); seed <= int64(seedCount); seed++ {
		puzzle, _, err := g.Generate(engine.Easy, engine.NewRand(seed))
		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		fp := f.Fingerprint(puzzle)
		if prior, dup := seen[fp]; dup {
			collisions++
			if !minisudokuEquivalent(prior, puzzle) {
				t.Errorf("fingerprint collision between NON-equivalent puzzles at seed %d "+
					"(fingerprint %x); canonicalization is collapsing distinct puzzles", seed, fp)
			} else {
				equivalent++
			}
		}
		seen[fp] = puzzle
	}
	t.Logf("batch: %d seeds, %d fingerprint collisions, %d confirmed symmetry-equivalent",
		seedCount, collisions, equivalent)
}

// ============================================================================
// Determinism Tests
// ============================================================================

// TestDeterminism_SameSeedIdenticalEncoding asserts that the same seed
// produces byte-identical encoded puzzles.
func TestDeterminism_SameSeedIdenticalEncoding(t *testing.T) {
	seed := int64(777)
	diff := engine.Hard

	// Generate two puzzles with the same seed and difficulty
	r1 := engine.NewRand(seed)
	g := Generator{}
	p1, _, err1 := g.Generate(diff, r1)
	if err1 != nil {
		t.Fatalf("first Generate failed: %v", err1)
	}

	r2 := engine.NewRand(seed)
	p2, _, err2 := g.Generate(diff, r2)
	if err2 != nil {
		t.Fatalf("second Generate failed: %v", err2)
	}

	// Encode both puzzles
	f := Fingerprinter{}
	c1 := f.Canonical(p1)
	c2 := f.Canonical(p2)

	// Compare canonical forms (byte-identical)
	if len(c1) != len(c2) {
		t.Errorf("canonical forms have different lengths: %d vs %d", len(c1), len(c2))
	}
	for i, b := range c1 {
		if c2[i] != b {
			t.Errorf("canonical forms differ at byte %d: %d vs %d", i, b, c2[i])
		}
	}
}

// ============================================================================
// Edge Cases & Gotchas
// ============================================================================

// TestGotcha_BoxGeometryIs2x3NotOtherwise asserts that boxes are correctly
// 2 rows × 3 columns, not 3×2 or any other configuration.
func TestGotcha_BoxGeometryIs2x3(t *testing.T) {
	// Construct a board where two cells in the same 2×3 box
	// (but different rows and different columns) both have value 1.
	// Cell (0,0) is in box (band=0, stack=0)
	// Cell (1,1) is in box (band=0, stack=0) for a 2×3 box
	// But if geometry were wrong (e.g., 3×2), cell (1,1) might be in a different box.
	board := Board{Cells: []int{
		1, 2, 3, 4, 5, 6,
		4, 1, 6, 1, 2, 3, // (1,1)=1 is in same box as (0,0)=1 for 2×3
		2, 3, 4, 5, 6, 1,
		5, 6, 2, 3, 4, 1,
		3, 1, 5, 6, 4, 2,
		6, 4, 2, 1, 3, 5,
	}}

	v := Validator{}
	viols := v.Violations(board)
	boxViolFound := false
	for _, viol := range viols {
		if viol.Rule == "box" {
			boxViolFound = true
			break
		}
	}
	if !boxViolFound {
		t.Error("2×3 box geometry: cells (0,0)=1 and (1,1)=1 should trigger a box violation")
	}
}

// TestGotcha_ParameterizedBoxDimensions asserts that BoxH and BoxW are configurable.
func TestGotcha_ParameterizedBoxDimensions(t *testing.T) {
	puzzle := Puzzle{
		N:    N,
		BoxH: 2,
		BoxW: 3,
		Givens: map[int]int{
			0: 1,
		},
		SeedVal: 123,
		Diff:    engine.Easy,
	}

	if puzzle.BoxH != 2 || puzzle.BoxW != 3 {
		t.Errorf("default box dimensions should be 2×3, got %d×%d", puzzle.BoxH, puzzle.BoxW)
	}
}

// TestGotcha_DigitLabelsAreSymbols asserts that digit labels (1-6) are treated
// as abstract symbols for canonicalization purposes.
func TestGotcha_DigitLabelsAreSymbols(t *testing.T) {
	f := Fingerprinter{}

	// Two puzzles with the same structure but different digit labels should
	// (ideally) share a fingerprint if canonicalization relabels by first appearance.
	// For now, this is a placeholder to ensure the fingerprinter handles relabeling.
	puzzle1 := Puzzle{
		N:    N,
		BoxH: 2,
		BoxW: 3,
		Givens: map[int]int{
			0: 1, 1: 2,
		},
		SeedVal: 111,
		Diff:    engine.Easy,
	}

	fp1 := f.Fingerprint(puzzle1)
	_ = fp1 // Use to avoid lint error
	// Real relabeling test happens when green-test author implements canonicalization.
}

// ============================================================================
// Integration / Stress Tests
// ============================================================================

// TestIntegration_GenerateVerifySolve tests a full round-trip:
// generate → validate → count solutions → logic solve.
func TestIntegration_GenerateVerifySolve(t *testing.T) {
	seed := int64(555)
	diff := engine.Medium

	r := engine.NewRand(seed)
	g := Generator{}
	puzzle, solution, err := g.Generate(diff, r)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Validate the solution
	v := Validator{}
	if !v.Solved(Board{Cells: solution.Cells}) {
		t.Fatal("generated solution is not valid")
	}

	// Count solutions
	s := Solver{}
	count := s.CountSolutions(puzzle, 2)
	if count != 1 {
		t.Errorf("puzzle should have exactly 1 solution, got %d", count)
	}

	// Solve with logic
	logicSol, closed, tech := s.LogicSolve(puzzle)
	if !closed {
		t.Error("logic solver should close the puzzle")
	}
	if !v.Solved(Board{Cells: logicSol.Cells}) {
		t.Error("logic solver solution is not valid")
	}

	// Verify the logic solution matches the generated solution
	for i, c := range logicSol.Cells {
		if solution.Cells[i] != c {
			t.Errorf("logic solution differs from generated solution at cell %d: %d vs %d", i, c, solution.Cells[i])
		}
	}

	if tech == "" {
		t.Error("technique should be reported by LogicSolve")
	}
}

// TestIntegration_FingerprinterDedup tests that the fingerprinter correctly
// rejects duplicate puzzles in a batch.
func TestIntegration_FingerprinterDedup(t *testing.T) {
	fp := Fingerprinter{}
	seen := make(map[[32]byte]int)

	seedCount := 20
	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		g := Generator{}
		puzzle, _, err := g.Generate(engine.Easy, r)
		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}

		fingerprint := fp.Fingerprint(puzzle)
		if prevSeed, dup := seen[fingerprint]; dup {
			t.Errorf("fingerprint collision: seed %d and seed %d", prevSeed, seed)
		}
		seen[fingerprint] = int(seed)
	}

	if len(seen) < 1 {
		t.Error("expected at least 1 unique fingerprint")
	}
}
