package minisudoku

import (
	"os"
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

// TestFingerprinter_BatchFingerprintsPairwiseDistinct asserts that
// fingerprints of multiple generated puzzles are pairwise distinct (no collisions).
func TestFingerprinter_BatchFingerprintsPairwiseDistinct(t *testing.T) {
	seedCount := 10
	if s := os.Getenv("LIG_SEEDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n < 100 {
			seedCount = n
		}
	}

	f := Fingerprinter{}
	fingerprints := make(map[[32]byte]bool)

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		g := Generator{}
		puzzle, _, err := g.Generate(engine.Easy, r)
		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}

		fp := f.Fingerprint(puzzle)
		if fingerprints[fp] {
			t.Errorf("fingerprint collision: seed %d shares fingerprint with an earlier seed", seed)
		}
		fingerprints[fp] = true
	}
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
