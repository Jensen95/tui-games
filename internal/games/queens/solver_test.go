package queens

import (
	"reflect"
	"testing"
)

// TestSolver_Solve_GoldenUniqueBoard pins: "Golden board (known unique
// solution) -> complete solver returns it."
func TestSolver_Solve_GoldenUniqueBoard(t *testing.T) {
	s := NewSolver()
	p := goldenUniquePuzzle6()
	want := goldenUniqueSolution6()

	got, ok := s.Solve(p)
	if !ok {
		t.Fatalf("Solve(golden unique N=6 board) returned ok=false, want true")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Solve(golden unique N=6 board) = %+v, want %+v", got, want)
	}
}

// TestSolver_CountSolutions_GoldenUniqueBoard_ReturnsOne pins: "... ; count
// == 1."
func TestSolver_CountSolutions_GoldenUniqueBoard_ReturnsOne(t *testing.T) {
	s := NewSolver()
	p := goldenUniquePuzzle6()

	if got := s.CountSolutions(p, 2); got != 1 {
		t.Errorf("CountSolutions(golden unique N=6 board, cap=2) = %d, want 1", got)
	}
}

// TestSolver_CountSolutions_AmbiguousBoard_ReturnsTwo pins: "Hand-built
// ambiguous board -> count == 2."
func TestSolver_CountSolutions_AmbiguousBoard_ReturnsTwo(t *testing.T) {
	s := NewSolver()
	p := ambiguousPuzzle5()

	if got := s.CountSolutions(p, 2); got != 2 {
		t.Errorf("CountSolutions(ambiguous N=5 board, cap=2) = %d, want 2", got)
	}
}

// TestSolver_Solve_AmbiguousBoard_ReturnsOneOfTheKnownSolutions checks Solve
// still returns *some* legal solution (one of the two known ones) even
// though the board is not uniquely solvable.
func TestSolver_Solve_AmbiguousBoard_ReturnsOneOfTheKnownSolutions(t *testing.T) {
	s := NewSolver()
	p := ambiguousPuzzle5()

	got, ok := s.Solve(p)
	if !ok {
		t.Fatalf("Solve(ambiguous N=5 board) returned ok=false, want true (a solution exists)")
	}
	for _, want := range ambiguousSolutions5() {
		if reflect.DeepEqual(got, want) {
			return
		}
	}
	t.Errorf("Solve(ambiguous N=5 board) = %+v, want one of %+v", got, ambiguousSolutions5())
}

// TestSolver_LogicSolve_GoldenBoard_ClosesAndMatchesComplete pins: "Logic
// solver closes all shipped examples with no guessing" and the
// cross-validation invariant from docs/02: LogicSolve's result, when closed,
// must equal the complete solver's unique solution.
func TestSolver_LogicSolve_GoldenBoard_ClosesAndMatchesComplete(t *testing.T) {
	s := NewSolver()
	p := goldenUniquePuzzle6()

	complete, ok := s.Solve(p)
	if !ok {
		t.Fatalf("Solve(golden unique N=6 board) returned ok=false, want true")
	}

	logic, closed, tech := s.LogicSolve(p)
	if !closed {
		t.Fatalf("LogicSolve(golden unique N=6 board) closed=false, want true (well-designed board, no guessing required)")
	}
	if !reflect.DeepEqual(logic, complete) {
		t.Errorf("LogicSolve(golden unique N=6 board) = %+v, want it to match Solve()'s %+v", logic, complete)
	}
	if tech == "" {
		t.Errorf("LogicSolve(golden unique N=6 board) reported empty Technique for a closed board")
	}
}
