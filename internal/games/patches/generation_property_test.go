package patches

import (
	"os"
	"strconv"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// getSeedCount reads LIG_SEEDS environment variable, defaults to 250.
func getSeedCount() int {
	if s := os.Getenv("LIG_SEEDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return 250
}

// TestGeneration_InvariantClueNumbersSum tests that generated puzzles have clue numbers summing to R*C.
// Invariant: sum(clue.Number for all clues) == R * C (exact cover property)
func TestGeneration_InvariantClueNumbersSum(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping property test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Easy, r)

		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		if p == nil {
			t.Fatalf("seed %d: Generate returned nil puzzle", seed)
		}

		sum := 0
		for _, clue := range p.Clues {
			sum += clue.Number
		}

		expected := p.R * p.C
		if sum != expected {
			t.Errorf("seed %d: clue numbers sum to %d, expected %d", seed, sum, expected)
		}
	}
}

// TestGeneration_InvariantSolutionIsValid tests that the recorded solution passes Solved().
// Invariant: Validator.Solved(solution) == true
func TestGeneration_InvariantSolutionIsValid(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping property test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		p, sol, err := gen.Generate(engine.Easy, r)

		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		if p == nil || sol == nil {
			t.Fatalf("seed %d: Generate returned nil", seed)
		}

		b := boardFromSolution(p, sol)
		v := NewValidator(p)

		if !v.Solved(b) {
			t.Errorf("seed %d: recorded solution does not pass Solved()", seed)
		}
	}
}

// TestGeneration_InvariantUniqueSolution tests that CountSolutions == 1 for all generated puzzles.
// Invariant: Solver.CountSolutions(p, cap=2) == 1
func TestGeneration_InvariantUniqueSolution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping property test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Easy, r)

		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		if p == nil {
			t.Fatalf("seed %d: Generate returned nil puzzle", seed)
		}

		s := NewSolver(p)
		count := s.CountSolutions(p, 2)

		if count != 1 {
			t.Errorf("seed %d: CountSolutions returned %d, expected 1", seed, count)
		}
	}
}

// TestGeneration_InvariantClueShapesMatchSolution tests that each clue's labeled shape matches its solution rectangle.
// Invariant: For each clue, the solution rectangle's dimensions must satisfy the shape constraint.
func TestGeneration_InvariantClueShapesMatchSolution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping property test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		p, sol, err := gen.Generate(engine.Easy, r)

		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		if p == nil || sol == nil {
			t.Fatalf("seed %d: Generate returned nil", seed)
		}

		// Map clue indices to rectangles in solution
		for rectIdx, rect := range sol.Rects {
			// Find which cell in this rectangle contains a clue
			var clue Clue
			found := false
			for cellIdx, c := range p.Clues {
				r := cellIdx / p.C
				col := cellIdx % p.C
				if r >= rect.R0 && r < rect.R0+rect.H && col >= rect.C0 && col < rect.C0+rect.W {
					clue = c
					found = true
					break
				}
			}

			if !found {
				t.Errorf("seed %d: rectangle %d has no clue", seed, rectIdx)
				continue
			}

			// Check that the rectangle's shape matches the clue's shape
			w, h := rect.W, rect.H
			switch clue.Shape {
			case Square:
				if w != h {
					t.Errorf("seed %d: Square clue realized as %dx%d", seed, w, h)
				}
			case Wide:
				if w <= h {
					t.Errorf("seed %d: Wide clue realized as %dx%d (not w > h)", seed, w, h)
				}
			case Tall:
				if h <= w {
					t.Errorf("seed %d: Tall clue realized as %dx%d (not h > w)", seed, w, h)
				}
			case Free:
				// No constraint
			}
		}
	}
}

// TestGeneration_InvariantLogicSolvable tests that Easy puzzles are logic-solvable (for no-guess).
// Invariant: For Easy difficulty, Solver.LogicSolve closes the board
func TestGeneration_InvariantLogicSolvable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping property test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Easy, r)

		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		if p == nil {
			t.Fatalf("seed %d: Generate returned nil puzzle", seed)
		}

		s := NewSolver(p)
		_, closed, _ := s.LogicSolve(p)

		if !closed {
			t.Errorf("seed %d: Easy puzzle not logic-solvable", seed)
		}
	}
}

// TestGeneration_InvariantMediumLogicSolvable tests that Medium puzzles are logic-solvable.
func TestGeneration_InvariantMediumLogicSolvable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping property test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Medium, r)

		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		if p == nil {
			t.Fatalf("seed %d: Generate returned nil puzzle", seed)
		}

		s := NewSolver(p)
		_, closed, _ := s.LogicSolve(p)

		if !closed {
			t.Errorf("seed %d: Medium puzzle not logic-solvable", seed)
		}
	}
}

// TestGeneration_InvariantHardLogicSolvable tests that Hard puzzles are logic-solvable.
func TestGeneration_InvariantHardLogicSolvable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping property test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Hard, r)

		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		if p == nil {
			t.Fatalf("seed %d: Generate returned nil puzzle", seed)
		}

		s := NewSolver(p)
		_, closed, _ := s.LogicSolve(p)

		if !closed {
			t.Errorf("seed %d: Hard puzzle not logic-solvable", seed)
		}
	}
}

// TestGeneration_InvariantAreaConstraint tests the structural invariant: clues' areas and shapes are consistent.
// This is implicitly tested by other invariants, but explicitly verify no clue is impossible.
func TestGeneration_InvariantAreaConstraint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping property test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Easy, r)

		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		if p == nil {
			t.Fatalf("seed %d: Generate returned nil puzzle", seed)
		}

		for _, clue := range p.Clues {
			// Validate that the clue's area and shape are at least theoretically compatible
			switch clue.Shape {
			case Square:
				// Area must be a perfect square
				sqrtArea := 1
				for sqrtArea*sqrtArea < clue.Number {
					sqrtArea++
				}
				if sqrtArea*sqrtArea != clue.Number {
					t.Errorf("seed %d: Square clue with non-perfect-square area %d", seed, clue.Number)
				}
			case Free:
				// Any area is fine
			default:
				// Wide or Tall: any area >= 2 works
				if clue.Number < 1 {
					t.Errorf("seed %d: clue with non-positive area %d", seed, clue.Number)
				}
			}
		}
	}
}
