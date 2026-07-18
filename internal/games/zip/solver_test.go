package zip

import (
	"reflect"
	"testing"
)

// ambiguousPuzzle is a 3x3 open grid (no walls) with only the two endpoints
// numbered:
//
//	0 1 2
//	3 4 5
//	6 7 8
//
// It has at least two distinct Hamiltonian paths from cell 0 to cell 8
// covering all nine cells, verified by hand:
//
//	path A: 0,1,2,5,4,3,6,7,8
//	path B: 0,3,6,7,4,1,2,5,8
//
// Both are simple orthogonal chains visiting every cell exactly once, start
// at 0, end at 8, and use no walls, so both satisfy the puzzle's
// constraints -- this puzzle is deliberately ambiguous.
func ambiguousPuzzle() Puzzle {
	return Puzzle{
		R: 3, C: 3,
		Waypoint: map[int]int{0: 1, 8: 9},
		Walls:    map[[2]int]bool{},
	}
}

func TestSolver_Solve_GoldenPuzzle_ReturnsUniquePath(t *testing.T) {
	p := minimalWaypointPuzzle()
	want := serpentinePath(2, 3) // [0,1,2,5,4,3], the puzzle's only Hamiltonian path

	got, ok := mustSolve(t, Solver{}, p)
	if !ok {
		t.Fatalf("Solve(golden puzzle) returned ok=false, want true")
	}
	if !reflect.DeepEqual(got.Path, want) {
		t.Errorf("Solve(golden puzzle).Path = %v, want %v", got.Path, want)
	}
}

func TestSolver_CountSolutions_GoldenPuzzle_IsOne(t *testing.T) {
	p := minimalWaypointPuzzle()

	if n := mustCountSolutions(t, Solver{}, p, 2); n != 1 {
		t.Errorf("CountSolutions(golden puzzle, cap=2) = %d, want 1", n)
	}
}

func TestSolver_CountSolutions_AmbiguousPuzzle_IsTwo(t *testing.T) {
	p := ambiguousPuzzle()

	if n := mustCountSolutions(t, Solver{}, p, 2); n != 2 {
		t.Errorf("CountSolutions(ambiguous puzzle, cap=2) = %d, want 2", n)
	}
}

func TestSolver_LogicSolve_NoGuessPuzzle_Closes(t *testing.T) {
	// Every cell numbered leaves nothing to search: the forced-move solver
	// must close this puzzle with no guessing.
	p := fullyNumberedPuzzle()
	want := serpentinePath(2, 3)

	sol, closed, _ := mustLogicSolve(t, Solver{}, p)
	if !closed {
		t.Fatalf("LogicSolve(fully-numbered puzzle) closed=false, want true (no-guess)")
	}
	if !reflect.DeepEqual(sol.Path, want) {
		t.Errorf("LogicSolve(fully-numbered puzzle).Path = %v, want %v", sol.Path, want)
	}
}

func TestSolver_LogicSolve_MatchesCompleteSolver_WhenClosed(t *testing.T) {
	// Cross-validation invariant (docs/plan/docs/02-engine-and-generation.md):
	// when the logic solver closes, its solution must equal the complete
	// solver's unique solution.
	p := fullyNumberedPuzzle()

	complete, ok := mustSolve(t, Solver{}, p)
	if !ok {
		t.Fatalf("Solve(fully-numbered puzzle) returned ok=false, want true")
	}

	logic, closed, _ := mustLogicSolve(t, Solver{}, p)
	if !closed {
		t.Fatalf("LogicSolve(fully-numbered puzzle) closed=false, want true")
	}
	if !reflect.DeepEqual(logic.Path, complete.Path) {
		t.Errorf("LogicSolve.Path = %v, Solve.Path = %v, want equal", logic.Path, complete.Path)
	}
}
