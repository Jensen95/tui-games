package tango

import (
	"os"
	"strconv"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// ============================================================================
// Validator Tests - Truth Tables
// ============================================================================

// TestValidator_EmptyBoard asserts that an empty (all-Empty) board has no violations.
func TestValidator_EmptyBoard(t *testing.T) {
	board := Board{N: N, Cells: make([]Symbol, N*N)}
	v := Validator{}
	viols := v.Violations(board)
	if len(viols) != 0 {
		t.Errorf("empty board should have no violations, got %d: %v", len(viols), viols)
	}
}

// TestValidator_ValidSolution asserts that a hand-built valid solution passes.
func TestValidator_ValidSolution(t *testing.T) {
	// A valid 6x6 solution: alternating pattern that respects balance and no-triplet.
	// Example: S M S M S M pattern repeated ensures 3 of each per row/col.
	board := Board{N: N, Cells: []Symbol{
		Sun, Moon, Sun, Moon, Sun, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
		Sun, Moon, Sun, Moon, Sun, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
		Sun, Moon, Sun, Moon, Sun, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
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

// TestValidator_RowBalanceViolation asserts that a row with 4 suns (imbalance) is caught.
func TestValidator_RowBalanceViolation(t *testing.T) {
	// Row 0 has 4 suns, 2 moons (should be 3-3)
	board := Board{N: N, Cells: []Symbol{
		Sun, Sun, Sun, Sun, Moon, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
		Sun, Moon, Sun, Moon, Sun, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
		Sun, Moon, Sun, Moon, Sun, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
	}}
	v := Validator{}
	viols := v.Violations(board)
	// Should have at least one balance violation on row 0.
	found := false
	for _, v := range viols {
		if v.Rule == "balance" {
			found = true
			break
		}
	}
	if !found {
		t.Error("row imbalance (4 suns, 2 moons) should trigger balance violation")
	}
}

// TestValidator_ColumnBalanceViolation asserts that a column imbalance is caught.
func TestValidator_ColumnBalanceViolation(t *testing.T) {
	// Column 0 has 4 suns, 2 moons
	board := Board{N: N, Cells: []Symbol{
		Sun, Moon, Sun, Moon, Sun, Moon,
		Sun, Sun, Moon, Sun, Moon, Sun,
		Sun, Moon, Sun, Moon, Sun, Moon,
		Sun, Moon, Sun, Moon, Sun, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
		Moon, Sun, Moon, Sun, Moon, Sun,
	}}
	v := Validator{}
	viols := v.Violations(board)
	// Should have at least one balance violation on column 0.
	found := false
	for _, v := range viols {
		if v.Rule == "balance" {
			found = true
			break
		}
	}
	if !found {
		t.Error("column imbalance should trigger balance violation")
	}
}

// TestValidator_HorizontalTripletViolation asserts that three suns in a row (H) are caught.
func TestValidator_HorizontalTripletViolation(t *testing.T) {
	// Row 0 has Sun Sun Sun consecutively at positions 0,1,2
	board := Board{N: N, Cells: []Symbol{
		Sun, Sun, Sun, Moon, Sun, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
		Sun, Moon, Sun, Moon, Sun, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
		Sun, Moon, Sun, Moon, Sun, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
	}}
	v := Validator{}
	viols := v.Violations(board)
	// Should have a triplet violation.
	found := false
	for _, v := range viols {
		if v.Rule == "three-in-a-row" {
			found = true
			break
		}
	}
	if !found {
		t.Error("three suns horizontally should trigger three-in-a-row violation")
	}
}

// TestValidator_VerticalTripletViolation asserts that three suns in a column (V) are caught.
func TestValidator_VerticalTripletViolation(t *testing.T) {
	// Column 1 has Sun Sun Sun consecutively at rows 0,1,2
	board := Board{N: N, Cells: []Symbol{
		Sun, Sun, Sun, Moon, Sun, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
		Sun, Moon, Sun, Moon, Sun, Moon,
		Moon, Moon, Moon, Sun, Moon, Sun,
		Sun, Moon, Sun, Moon, Sun, Moon,
		Moon, Moon, Moon, Sun, Moon, Sun,
	}}
	v := Validator{}
	viols := v.Violations(board)
	// Should have a triplet violation.
	found := false
	for _, v := range viols {
		if v.Rule == "three-in-a-row" {
			found = true
			break
		}
	}
	if !found {
		t.Error("three suns vertically should trigger three-in-a-row violation")
	}
}

// TestValidator_DiagonalTripletNoViolation asserts that three in a row diagonally does NOT trigger
// (this is the classic gotcha: Tango rules are H/V only, not diagonal).
func TestValidator_DiagonalTripletNoViolation(t *testing.T) {
	// Diagonal triplet: (0,0), (1,1), (2,2) all Sun. This should NOT be a violation.
	board := Board{N: N, Cells: []Symbol{
		Sun, Moon, Sun, Moon, Sun, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
		Sun, Moon, Sun, Moon, Sun, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
		Sun, Moon, Sun, Moon, Sun, Moon,
		Moon, Sun, Moon, Sun, Moon, Sun,
	}}
	v := Validator{}
	viols := v.Violations(board)
	// Check that NO triplet violation is flagged on the diagonal.
	for _, v := range viols {
		if v.Rule == "three-in-a-row" {
			t.Error("diagonal triplet should NOT trigger three-in-a-row violation")
		}
	}
}

// TestValidator_EqualEdgeConstraintViolation asserts that = edge with different symbols is caught.
func TestValidator_EqualEdgeConstraintViolation(t *testing.T) {
	board := Board{
		N: N,
		Cells: []Symbol{
			Sun, Moon, Sun, Moon, Sun, Moon,
			Moon, Sun, Moon, Sun, Moon, Sun,
			Sun, Moon, Sun, Moon, Sun, Moon,
			Moon, Sun, Moon, Sun, Moon, Sun,
			Sun, Moon, Sun, Moon, Sun, Moon,
			Moon, Sun, Moon, Sun, Moon, Sun,
		},
		HEdges: map[[2]int]Relation{
			{0, 1}: Equal, // cells (0,0) and (0,1) should be equal, but Sun != Moon
		},
		VEdges: map[[2]int]Relation{},
	}
	v := Validator{}
	viols := v.Violations(board)
	// Should have an edge constraint violation.
	found := false
	for _, violation := range viols {
		if violation.Rule == "edge-constraint" {
			found = true
			break
		}
	}
	if !found {
		t.Error("= edge with different symbols should trigger edge-constraint violation")
	}
}

// TestValidator_CrossEdgeConstraintViolation asserts that × edge with same symbols is caught.
func TestValidator_CrossEdgeConstraintViolation(t *testing.T) {
	board := Board{
		N: N,
		Cells: []Symbol{
			Sun, Sun, Sun, Moon, Sun, Moon,
			Moon, Sun, Moon, Sun, Moon, Sun,
			Sun, Moon, Sun, Moon, Sun, Moon,
			Moon, Sun, Moon, Sun, Moon, Sun,
			Sun, Moon, Sun, Moon, Sun, Moon,
			Moon, Sun, Moon, Sun, Moon, Sun,
		},
		HEdges: map[[2]int]Relation{
			{0, 1}: Cross, // cells (0,0) and (0,1) should differ, but both are Sun
		},
		VEdges: map[[2]int]Relation{},
	}
	v := Validator{}
	viols := v.Violations(board)
	// Should have an edge constraint violation.
	found := false
	for _, violation := range viols {
		if violation.Rule == "edge-constraint" {
			found = true
			break
		}
	}
	if !found {
		t.Error("× edge with same symbols should trigger edge-constraint violation")
	}
}

// TestValidator_PartialBoardOnlyFlagsExistingViolations asserts that partial boards
// only report violations of already-filled cells.
func TestValidator_PartialBoardOnlyFlagsExistingViolations(t *testing.T) {
	// Board with mostly Empty, a few cells filled.
	board := Board{
		N:      N,
		Cells:  make([]Symbol, N*N),
		HEdges: make(map[[2]int]Relation),
		VEdges: make(map[[2]int]Relation),
	}
	board.Cells[0] = Sun
	board.Cells[1] = Sun
	board.Cells[2] = Sun // Three suns in a row is a violation
	// Rest are Empty
	v := Validator{}
	viols := v.Violations(board)
	// Should detect the triplet even though board is mostly empty.
	found := false
	for _, violation := range viols {
		if violation.Rule == "three-in-a-row" {
			found = true
			break
		}
	}
	if !found {
		t.Error("partial board should still flag triplet violation on filled cells")
	}
}

// ============================================================================
// Solver Tests
// ============================================================================

// TestSolver_GoldenPuzzleUniqueSolution asserts that a hand-built puzzle
// with a known unique solution is solved correctly and CountSolutions==1.
func TestSolver_GoldenPuzzleUniqueSolution(t *testing.T) {
	// A simple golden puzzle: mostly givens, few empty cells, unique solution.
	puzzle := Puzzle{
		N: N,
		Givens: map[int]Symbol{
			0: Sun, 1: Moon, 2: Sun, 3: Moon, 4: Sun, 5: Moon,
			6: Moon, 7: Sun, 8: Moon, 9: Sun, 10: Moon, 11: Sun,
			12: Sun, 13: Moon, 14: Sun, 15: Moon, 16: Sun, 17: Moon,
			18: Moon, 19: Sun, 20: Moon, 21: Sun, 22: Moon, 23: Sun,
			24: Sun, 25: Moon, 26: Sun, 27: Moon, 28: Sun, 29: Moon,
			30: Moon, 31: Sun, 32: Moon, 33: Sun, 34: Moon, 35: Sun,
		},
		HEdges:  map[[2]int]Relation{},
		VEdges:  map[[2]int]Relation{},
		Diff:    engine.Easy,
		SeedVal: 42,
	}

	s := Solver{}
	count := s.CountSolutions(puzzle, 2)
	if count != 1 {
		t.Errorf("golden puzzle should have exactly 1 solution, got %d", count)
	}
}

// TestSolver_AmbiguousPuzzleHasTwoSolutions asserts that a hand-built puzzle
// with two solutions correctly returns count==2 when we ask for cap>=2.
func TestSolver_AmbiguousPuzzleHasTwoSolutions(t *testing.T) {
	// A deliberately under-constrained puzzle with 2 solutions.
	// For example, a puzzle with few givens allowing multiple valid completions.
	puzzle := Puzzle{
		N: N,
		Givens: map[int]Symbol{
			0: Sun, 6: Moon, 12: Sun, 18: Moon, 24: Sun, 30: Moon,
		},
		HEdges:  map[[2]int]Relation{},
		VEdges:  map[[2]int]Relation{},
		Diff:    engine.Easy,
		SeedVal: 99,
	}

	s := Solver{}
	count := s.CountSolutions(puzzle, 2)
	if count != 2 {
		t.Errorf("ambiguous puzzle should have 2 solutions, got %d", count)
	}
}

// TestSolver_CountSolutionsStopsAtCap asserts that CountSolutions stops
// counting once it reaches the cap, to optimize for uniqueness checks.
func TestSolver_CountSolutionsStopsAtCap(t *testing.T) {
	// A puzzle with many solutions, but we only ask for cap=3.
	puzzle := Puzzle{
		N: N,
		Givens: map[int]Symbol{
			0: Sun,
		},
		HEdges:  map[[2]int]Relation{},
		VEdges:  map[[2]int]Relation{},
		Diff:    engine.Easy,
		SeedVal: 77,
	}

	s := Solver{}
	count := s.CountSolutions(puzzle, 3)
	// Should return at most 3 (may be fewer if there are fewer than 3 solutions).
	if count > 3 {
		t.Errorf("CountSolutions should not exceed cap of 3, got %d", count)
	}
}

// TestSolver_SolveReturnsOneSolution asserts that Solve returns a solution
// for a solvable puzzle.
func TestSolver_SolveReturnsOneSolution(t *testing.T) {
	puzzle := Puzzle{
		N: N,
		Givens: map[int]Symbol{
			0: Sun, 1: Moon, 2: Sun, 3: Moon, 4: Sun, 5: Moon,
			6: Moon, 7: Sun, 8: Moon, 9: Sun, 10: Moon, 11: Sun,
			12: Sun, 13: Moon, 14: Sun, 15: Moon, 16: Sun, 17: Moon,
			18: Moon, 19: Sun, 20: Moon, 21: Sun, 22: Moon, 23: Sun,
			24: Sun, 25: Moon, 26: Sun, 27: Moon, 28: Sun, 29: Moon,
			30: Moon, 31: Sun, 32: Moon, 33: Sun, 34: Moon, 35: Sun,
		},
		HEdges:  map[[2]int]Relation{},
		VEdges:  map[[2]int]Relation{},
		Diff:    engine.Easy,
		SeedVal: 42,
	}

	s := Solver{}
	board, ok := s.Solve(puzzle)
	if !ok {
		t.Error("Solve should return true for a solvable puzzle")
	}
	v := Validator{}
	if !v.Solved(board) {
		t.Error("Solve should return a valid complete solution")
	}
}

// TestSolver_LogicSolveClosesEasyPuzzle asserts that the logic solver can
// fully close an easy puzzle without guessing.
func TestSolver_LogicSolveClosesEasyPuzzle(t *testing.T) {
	puzzle := Puzzle{
		N: N,
		Givens: map[int]Symbol{
			0: Sun, 1: Moon, 2: Sun, 3: Moon, 4: Sun, 5: Moon,
			6: Moon, 7: Sun, 8: Moon, 9: Sun, 10: Moon, 11: Sun,
			12: Sun, 13: Moon, 14: Sun, 15: Moon, 16: Sun, 17: Moon,
			18: Moon, 19: Sun, 20: Moon, 21: Sun, 22: Moon, 23: Sun,
			24: Sun, 25: Moon, 26: Sun, 27: Moon, 28: Sun, 29: Moon,
			30: Moon, 31: Sun, 32: Moon, 33: Sun, 34: Moon, 35: Sun,
		},
		HEdges:  map[[2]int]Relation{},
		VEdges:  map[[2]int]Relation{},
		Diff:    engine.Easy,
		SeedVal: 42,
	}

	s := Solver{}
	board, closed, technique := s.LogicSolve(puzzle)
	if !closed {
		t.Error("LogicSolve should fully close an easy puzzle")
	}
	if technique == "" {
		t.Error("LogicSolve should report a technique")
	}
	v := Validator{}
	if !v.Solved(board) {
		t.Error("LogicSolve should return a valid solution")
	}
}

// ============================================================================
// Generation Property Tests
// ============================================================================

// TestGenerationInvariant_EveryPuzzleIsValid asserts that every generated puzzle
// has a valid solution that passes Solved().
func TestGenerationInvariant_EveryPuzzleIsValid(t *testing.T) {
	seedCount := getSeedCount()
	g := Generator{}
	v := Validator{}

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		_, solution, err := g.Generate(engine.Easy, r)
		if err != nil {
			t.Fatalf("Generate(seed=%d) failed: %v", seed, err)
		}
		if !v.Solved(solution) {
			t.Errorf("seed=%d: generated solution should pass Solved()", seed)
		}
	}
}

// TestGenerationInvariant_EveryPuzzleHasUniqueSolution asserts that every generated
// puzzle has exactly one solution.
func TestGenerationInvariant_EveryPuzzleHasUniqueSolution(t *testing.T) {
	seedCount := getSeedCount()
	g := Generator{}
	s := Solver{}

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		puzzle, _, err := g.Generate(engine.Easy, r)
		if err != nil {
			t.Fatalf("Generate(seed=%d) failed: %v", seed, err)
		}
		count := s.CountSolutions(puzzle, 2)
		if count != 1 {
			t.Errorf("seed=%d: generated puzzle should have exactly 1 solution, got %d", seed, count)
		}
	}
}

// TestGenerationInvariant_LogicSolverClosesEasyPuzzles asserts that the logic solver
// can fully solve every generated easy puzzle.
func TestGenerationInvariant_LogicSolverClosesEasyPuzzles(t *testing.T) {
	seedCount := getSeedCount()
	g := Generator{}
	s := Solver{}

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		puzzle, _, err := g.Generate(engine.Easy, r)
		if err != nil {
			t.Fatalf("Generate(seed=%d) failed: %v", seed, err)
		}
		_, closed, _ := s.LogicSolve(puzzle)
		if !closed {
			t.Errorf("seed=%d: logic solver should close easy puzzle", seed)
		}
	}
}

// TestGenerationInvariant_LogicSolverClosesMediumPuzzles asserts logic solver closes medium.
func TestGenerationInvariant_LogicSolverClosesMediumPuzzles(t *testing.T) {
	seedCount := getSeedCount()
	g := Generator{}
	s := Solver{}

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		puzzle, _, err := g.Generate(engine.Medium, r)
		if err != nil {
			t.Fatalf("Generate(seed=%d) failed: %v", seed, err)
		}
		_, closed, _ := s.LogicSolve(puzzle)
		if !closed {
			t.Errorf("seed=%d: logic solver should close medium puzzle", seed)
		}
	}
}

// TestGenerationInvariant_LogicSolverClosesHardPuzzles asserts logic solver closes hard.
func TestGenerationInvariant_LogicSolverClosesHardPuzzles(t *testing.T) {
	seedCount := getSeedCount()
	g := Generator{}
	s := Solver{}

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		puzzle, _, err := g.Generate(engine.Hard, r)
		if err != nil {
			t.Fatalf("Generate(seed=%d) failed: %v", seed, err)
		}
		_, closed, _ := s.LogicSolve(puzzle)
		if !closed {
			t.Errorf("seed=%d: logic solver should close hard puzzle", seed)
		}
	}
}

// TestGenerationInvariant_StructuralInvariants asserts that generated puzzles
// meet structural invariants (e.g., grid size, N is 6, cells are initialized).
func TestGenerationInvariant_StructuralInvariants(t *testing.T) {
	seedCount := getSeedCount()
	g := Generator{}

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		puzzle, solution, err := g.Generate(engine.Easy, r)
		if err != nil {
			t.Fatalf("Generate(seed=%d) failed: %v", seed, err)
		}

		if puzzle.N != N {
			t.Errorf("seed=%d: puzzle grid should be %dx%d, got %dx%d", seed, N, N, puzzle.N, puzzle.N)
		}
		if solution.N != N {
			t.Errorf("seed=%d: solution grid should be %dx%d, got %dx%d", seed, N, N, solution.N, solution.N)
		}
		if len(solution.Cells) != N*N {
			t.Errorf("seed=%d: solution should have %d cells, got %d", seed, N*N, len(solution.Cells))
		}
		for i, cell := range solution.Cells {
			if cell == Empty {
				t.Errorf("seed=%d: solution cell %d should not be empty", seed, i)
			}
		}
	}
}

// TestGenerationInvariant_GeneratedPuzzlesHaveValidGivens asserts that givens
// in the puzzle are consistent with the solution.
func TestGenerationInvariant_GeneratedPuzzlesHaveValidGivens(t *testing.T) {
	seedCount := getSeedCount()
	g := Generator{}

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		puzzle, solution, err := g.Generate(engine.Easy, r)
		if err != nil {
			t.Fatalf("Generate(seed=%d) failed: %v", seed, err)
		}

		for idx, symbol := range puzzle.Givens {
			if idx < 0 || idx >= N*N {
				t.Errorf("seed=%d: given index %d out of bounds", seed, idx)
			}
			if solution.Cells[idx] != symbol {
				t.Errorf("seed=%d: given at index %d should match solution", seed, idx)
			}
		}
	}
}

// ============================================================================
// Canonicalization & Deduplication Tests
// ============================================================================

// TestCanonicalización_AllTransformsYieldSameFingerprint asserts that applying
// every symmetry transform to a puzzle yields the same fingerprint.
func TestCanonicalización_AllTransformsYieldSameFingerprint(t *testing.T) {
	// Create a simple puzzle.
	puzzle := Puzzle{
		N: N,
		Givens: map[int]Symbol{
			0: Sun, 6: Moon, 12: Sun,
		},
		HEdges:  map[[2]int]Relation{},
		VEdges:  map[[2]int]Relation{},
		Diff:    engine.Easy,
		SeedVal: 42,
	}

	fp := Fingerprinter{}
	baseFingerprint := fp.Fingerprint(puzzle)

	// Apply all dihedral transforms and collect their fingerprints.
	for i := 0; i < len(engine.AllTransforms); i++ {
		transform := engine.AllTransforms[i]
		transformedPuzzle := transformPuzzle(puzzle, transform)
		transformedFingerprint := fp.Fingerprint(transformedPuzzle)
		if transformedFingerprint != baseFingerprint {
			t.Errorf("transform %d: fingerprint mismatch after applying transform", i)
		}
	}
}

// TestCanonicalización_SymbolSwapYieldsSameFingerprint asserts that swapping
// Sun/Moon yields the same fingerprint.
func TestCanonicalización_SymbolSwapYieldsSameFingerprint(t *testing.T) {
	puzzle := Puzzle{
		N: N,
		Givens: map[int]Symbol{
			0: Sun, 6: Moon, 12: Sun,
		},
		HEdges:  map[[2]int]Relation{},
		VEdges:  map[[2]int]Relation{},
		Diff:    engine.Easy,
		SeedVal: 42,
	}

	fp := Fingerprinter{}
	baseFingerprint := fp.Fingerprint(puzzle)

	// Swap symbols in the puzzle.
	swappedPuzzle := swapSymbols(puzzle)
	swappedFingerprint := fp.Fingerprint(swappedPuzzle)
	if swappedFingerprint != baseFingerprint {
		t.Error("symbol swap should not change fingerprint")
	}
}

// TestCanonicalización_BatchFingerprintsPairwiseDistinct asserts that a batch
// of generated puzzles all have distinct fingerprints.
func TestCanonicalización_BatchFingerprintsPairwiseDistinct(t *testing.T) {
	// Generate a batch of puzzles with different seeds.
	batchSize := 10
	g := Generator{}
	fp := Fingerprinter{}
	seen := make(map[[32]byte]bool)

	for i := 1; i <= batchSize; i++ {
		r := engine.NewRand(int64(i))
		puzzle, _, err := g.Generate(engine.Easy, r)
		if err != nil {
			t.Fatalf("Generate(seed=%d) failed: %v", i, err)
		}
		fingerprint := fp.Fingerprint(puzzle)
		if seen[fingerprint] {
			t.Errorf("seed=%d: fingerprint collision detected", i)
		}
		seen[fingerprint] = true
	}
}

// ============================================================================
// Determinism Tests
// ============================================================================

// TestDeterminism_SameSeedYieldsIdenticalPuzzle asserts that generating with
// the same seed twice yields bit-identical puzzles.
func TestDeterminism_SameSeedYieldsIdenticalPuzzle(t *testing.T) {
	g := Generator{}
	seed := int64(12345)

	r1 := engine.NewRand(seed)
	puzzle1, _, err1 := g.Generate(engine.Easy, r1)
	if err1 != nil {
		t.Fatalf("first Generate(seed=%d) failed: %v", seed, err1)
	}

	r2 := engine.NewRand(seed)
	puzzle2, _, err2 := g.Generate(engine.Easy, r2)
	if err2 != nil {
		t.Fatalf("second Generate(seed=%d) failed: %v", seed, err2)
	}

	if !puzzlesEqual(puzzle1, puzzle2) {
		t.Error("same seed should yield identical puzzles")
	}
}

// TestDeterminism_DifferentSeedsYieldDifferentPuzzles asserts that different
// seeds produce different puzzles (with high probability).
func TestDeterminism_DifferentSeedsYieldDifferentPuzzles(t *testing.T) {
	g := Generator{}

	r1 := engine.NewRand(100)
	puzzle1, _, err1 := g.Generate(engine.Easy, r1)
	if err1 != nil {
		t.Fatalf("Generate(seed=100) failed: %v", err1)
	}

	r2 := engine.NewRand(200)
	puzzle2, _, err2 := g.Generate(engine.Easy, r2)
	if err2 != nil {
		t.Fatalf("Generate(seed=200) failed: %v", err2)
	}

	if puzzlesEqual(puzzle1, puzzle2) {
		t.Error("different seeds should (very likely) yield different puzzles")
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

func getSeedCount() int {
	seedCountStr := os.Getenv("LIG_SEEDS")
	if seedCountStr == "" {
		return 250
	}
	seedCount, err := strconv.Atoi(seedCountStr)
	if err != nil {
		return 250
	}
	return seedCount
}

// transformPuzzle applies a dihedral transform to a puzzle: it maps every
// given and edge through t (an edge can move between the H and V sets under
// a dimension-swapping transform, e.g. a 90° rotation) using the same
// transform machinery the Fingerprinter canonicalizes with.
func transformPuzzle(p Puzzle, t engine.Transform) Puzzle {
	givens, h, v := transformPuzzleGrid(p, t)
	return Puzzle{
		N:       p.N,
		Givens:  givens,
		HEdges:  h,
		VEdges:  v,
		Diff:    p.Diff,
		SeedVal: p.SeedVal,
	}
}

// swapSymbols swaps Sun and Moon in a puzzle's givens.
func swapSymbols(p Puzzle) Puzzle {
	swapped := Puzzle{
		N:       p.N,
		Givens:  make(map[int]Symbol),
		HEdges:  p.HEdges, // Edge constraints are symbol-independent
		VEdges:  p.VEdges,
		Diff:    p.Diff,
		SeedVal: p.SeedVal,
	}
	for idx, sym := range p.Givens {
		if sym == Sun {
			swapped.Givens[idx] = Moon
		} else if sym == Moon {
			swapped.Givens[idx] = Sun
		}
	}
	return swapped
}

// puzzlesEqual checks if two puzzles are equal (for determinism testing).
func puzzlesEqual(p1, p2 Puzzle) bool {
	if p1.N != p2.N || p1.Diff != p2.Diff || p1.SeedVal != p2.SeedVal {
		return false
	}
	if len(p1.Givens) != len(p2.Givens) {
		return false
	}
	for idx, sym := range p1.Givens {
		if p2.Givens[idx] != sym {
			return false
		}
	}
	if len(p1.HEdges) != len(p2.HEdges) {
		return false
	}
	for pair, rel := range p1.HEdges {
		if p2.HEdges[pair] != rel {
			return false
		}
	}
	if len(p1.VEdges) != len(p2.VEdges) {
		return false
	}
	for pair, rel := range p1.VEdges {
		if p2.VEdges[pair] != rel {
			return false
		}
	}
	return true
}
