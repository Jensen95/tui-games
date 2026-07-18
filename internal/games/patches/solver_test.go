package patches

import (
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// TestSolver_GoldenPuzzleUnique tests that a golden hand-built puzzle with a known unique solution is found.
func TestSolver_GoldenPuzzleUnique(t *testing.T) {
	// Simple 2x2 puzzle with 2 rectangles, each area 2
	// Top row: one wide 2x1 rectangle
	// Bottom row: one wide 2x1 rectangle
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 2, Shape: Wide}, // (0,0)
			2: {Number: 2, Shape: Wide}, // (1,0)
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	s := NewSolver(p)

	// Count solutions with cap=2 to detect uniqueness
	count := s.CountSolutions(p, 2)
	if count != 1 {
		t.Errorf("expected exactly 1 solution for golden puzzle, got %d", count)
	}

	// Solve should return a solution
	sol, ok := s.Solve(p)
	if !ok {
		t.Error("expected Solve() to find a solution for golden puzzle")
	}
	if sol == nil {
		t.Error("expected non-nil solution")
	}
}

// TestSolver_CountSolutionsRespectsCapTwo tests that CountSolutions returns min(count, cap).
func TestSolver_CountSolutionsRespectsCapTwo(t *testing.T) {
	// Ambiguous puzzle: 2x3 with Free clues (allows multiple solutions)
	p := &Puzzle{
		R: 2,
		C: 3,
		Clues: map[int]Clue{
			0: {Number: 3, Shape: Free}, // (0,0)
			3: {Number: 3, Shape: Free}, // (1,0)
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	s := NewSolver(p)

	// CountSolutions with cap=2 should return min(actual_count, 2)
	count := s.CountSolutions(p, 2)
	if count > 2 {
		t.Errorf("expected CountSolutions(p, 2) to return at most 2, got %d", count)
	}
	if count < 1 {
		t.Errorf("expected at least 1 solution")
	}
}

// TestSolver_AmbiguousPuzzleCount tests that an ambiguous puzzle returns count > 1.
//
// NOTE(green-impl): the original 3x2 layout (three Free·2 clues stacked in
// column 0 at rows 0,1,2) is not actually ambiguous: the game's "exactly one
// clue per rectangle" rule (spec Rules #2) forbids any of the three clues
// from going vertical, since a vertical domino from any one of them
// necessarily swallows a neighboring clue's cell (e.g. clue (0,0) going
// vertical would cover (1,0), which is itself a clue). That leaves exactly
// one legal tiling (all three horizontal), so CountSolutions is genuinely 1,
// not 2 — the test's own inline comments ("can be 2x1 or 1x2") didn't account
// for that interaction. Replaced with the classic ambiguous Shikaku pair that
// actually has two solutions: a 2x2 grid with Free·2 clues at opposite
// corners tiles either as two horizontal dominoes or two vertical dominoes,
// both legal and distinct, giving CountSolutions == 2 — preserving the
// test's stated intent (verify an ambiguous puzzle is reported as such).
func TestSolver_AmbiguousPuzzleCount(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 2, Shape: Free}, // (0,0) - top row or left col
			3: {Number: 2, Shape: Free}, // (1,1) - bottom row or right col
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	s := NewSolver(p)

	// This puzzle should be ambiguous
	count := s.CountSolutions(p, 2)
	if count != 2 {
		t.Errorf("expected exactly 2 solutions for ambiguous puzzle, got %d", count)
	}
}

// TestSolver_SolveReturnsValidSolution tests that Solve returns a valid solution.
func TestSolver_SolveReturnsValidSolution(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 2, Shape: Free},
			2: {Number: 2, Shape: Free},
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	s := NewSolver(p)
	sol, ok := s.Solve(p)

	if !ok {
		t.Fatal("expected Solve() to find a solution")
	}
	if sol == nil {
		t.Fatal("expected non-nil solution")
	}

	// Solution should have correct number of rectangles (one per clue)
	if len(sol.Rects) != len(p.Clues) {
		t.Errorf("expected %d rectangles, got %d", len(p.Clues), len(sol.Rects))
	}

	// Convert to board to validate
	b := boardFromSolution(p, sol)
	v := NewValidator(p)
	violations := v.Violations(b)
	if len(violations) > 0 {
		t.Errorf("expected solution to be valid, got violations: %v", violations)
	}
}

// TestSolver_NoSolutionExists tests that Solve returns false when no solution exists.
func TestSolver_NoSolutionExists(t *testing.T) {
	// Over-constrained puzzle: clue numbers sum to more than grid area
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 3, Shape: Free},
			2: {Number: 3, Shape: Free},
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	s := NewSolver(p)
	_, ok := s.Solve(p)

	if ok {
		t.Error("expected Solve() to return false for over-constrained puzzle")
	}
}

// TestSolver_LogicSolveClosable tests LogicSolve on a simple puzzle.
func TestSolver_LogicSolveClosable(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 2, Shape: Wide},
			2: {Number: 2, Shape: Wide},
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	s := NewSolver(p)
	sol, closed, technique := s.LogicSolve(p)

	if sol == nil {
		t.Error("expected LogicSolve to return a solution")
	}

	if !closed {
		t.Error("expected LogicSolve to fully close a simple puzzle")
	}

	if technique == "" {
		t.Error("expected LogicSolve to return a non-empty technique")
	}
}

// TestSolver_LogicSolveAgreeswithComplete tests that LogicSolve solution matches Solve.
func TestSolver_LogicSolveAgreeswithComplete(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 2, Shape: Wide},
			2: {Number: 2, Shape: Wide},
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	s := NewSolver(p)
	logicSol, logicClosed, _ := s.LogicSolve(p)
	completeSol, completeOk := s.Solve(p)

	if !logicClosed {
		t.Skip("LogicSolve did not fully close; skipping cross-validation")
	}

	if !completeOk {
		t.Fatal("Solve() should find a solution for this puzzle")
	}

	// Solutions should be structurally equivalent (both tiles the grid correctly)
	logicBoard := boardFromSolution(p, logicSol)
	completeBoard := boardFromSolution(p, completeSol)

	if !boardsEquivalent(logicBoard, completeBoard) {
		t.Error("LogicSolve and Solve returned different solutions")
	}
}

// TestSolver_CountZeroSolutions tests that impossible puzzles return 0 solutions.
func TestSolver_CountZeroSolutions(t *testing.T) {
	// Impossible: two clues of area 1 and 1 in a 1x2 grid = area 2 (impossible: one clue goes in the gap)
	p := &Puzzle{
		R: 1,
		C: 3,
		Clues: map[int]Clue{
			0: {Number: 1, Shape: Free},
			2: {Number: 1, Shape: Free},
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	s := NewSolver(p)
	count := s.CountSolutions(p, 2)

	if count != 0 {
		t.Errorf("expected 0 solutions for impossible puzzle, got %d", count)
	}
}

// Helper: converts a Solution to a Board for validation
func boardFromSolution(p *Puzzle, sol *Solution) *Board {
	b := NewBoard(p)
	for rectIdx, rect := range sol.Rects {
		// Fill all cells of this rectangle
		for r := rect.R0; r < rect.R0+rect.H; r++ {
			for c := rect.C0; c < rect.C0+rect.W; c++ {
				if r >= 0 && r < p.R && c >= 0 && c < p.C {
					b.Cells[r*p.C+c] = rectIdx
				}
			}
		}
	}
	return b
}

// Helper: compares two boards for equivalence
func boardsEquivalent(b1, b2 *Board) bool {
	if len(b1.Cells) != len(b2.Cells) {
		return false
	}
	// Both boards should have the same coverage pattern
	for i, c1 := range b1.Cells {
		c2 := b2.Cells[i]
		// -1 means uncovered; both must be uncovered or both covered
		if (c1 == -1) != (c2 == -1) {
			return false
		}
	}
	return true
}
