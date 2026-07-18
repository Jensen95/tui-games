package zip

import (
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// hasRule reports whether violations contains one with the given Rule.
func hasRule(violations []engine.Violation, rule string) bool {
	for _, v := range violations {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

// fullyNumberedPuzzle is a 2x3 grid, open (no walls), with every cell
// numbered 1..6 in serpentine order:
//
//	0 1 2
//	3 4 5
//
// serpentine path: 0,1,2,5,4,3 -> waypoints 1,2,3,4,5,6 respectively.
func fullyNumberedPuzzle() Puzzle {
	return Puzzle{
		R: 2, C: 3,
		Waypoint: map[int]int{0: 1, 1: 2, 2: 3, 5: 4, 4: 5, 3: 6},
		Walls:    map[[2]int]bool{},
	}
}

// minimalWaypointPuzzle is the same 2x3 open grid but only start (1) and
// end (6) are numbered; its unique Hamiltonian path is still 0,1,2,5,4,3
// (see solver_test.go for the by-hand proof of uniqueness).
func minimalWaypointPuzzle() Puzzle {
	return Puzzle{
		R: 2, C: 3,
		Waypoint: map[int]int{0: 1, 3: 6},
		Walls:    map[[2]int]bool{},
	}
}

func TestValidator_Solved_FullSerpentinePath_Valid(t *testing.T) {
	p := fullyNumberedPuzzle()
	b := Board{Puzzle: p, Path: serpentinePath(2, 3)}

	if got := mustSolved(t, Validator{}, b); !got {
		t.Errorf("Solved(full serpentine path) = false, want true")
	}
	if v := mustViolations(t, Validator{}, b); len(v) != 0 {
		t.Errorf("Violations(full serpentine path) = %v, want none", v)
	}
}

func TestValidator_Solved_MissingCell_Invalid(t *testing.T) {
	p := fullyNumberedPuzzle()
	// Path skips cell 3 (the last serpentine cell) -- not Hamiltonian.
	b := Board{Puzzle: p, Path: []int{0, 1, 2, 5, 4}}

	if got := mustSolved(t, Validator{}, b); got {
		t.Errorf("Solved(path missing a cell) = true, want false")
	}
	// An incomplete-but-not-yet-broken path must report no violations: it
	// hasn't done anything wrong, it's just unfinished (spec: Violations on
	// a partial board returns only already-broken rules).
	if v := mustViolations(t, Validator{}, b); len(v) != 0 {
		t.Errorf("Violations(incomplete but not-yet-invalid path) = %v, want none", v)
	}
}

func TestValidator_Violations_Revisit(t *testing.T) {
	p := minimalWaypointPuzzle()
	b := Board{Puzzle: p, Path: []int{0, 1, 2, 1}} // revisits cell 1

	v := mustViolations(t, Validator{}, b)
	if !hasRule(v, RuleRevisit) {
		t.Errorf("Violations(revisit) = %v, want to contain %q", v, RuleRevisit)
	}
	if mustSolved(t, Validator{}, b) {
		t.Errorf("Solved(revisiting path) = true, want false")
	}
}

func TestValidator_Violations_DiagonalStep(t *testing.T) {
	p := minimalWaypointPuzzle()
	// cell 0 = (row0,col0), cell 4 = (row1,col1): a diagonal step.
	b := Board{Puzzle: p, Path: []int{0, 4}}

	v := mustViolations(t, Validator{}, b)
	if !hasRule(v, RuleNonAdjacentStep) {
		t.Errorf("Violations(diagonal step) = %v, want to contain %q", v, RuleNonAdjacentStep)
	}
}

func TestValidator_Violations_NonAdjacentJump(t *testing.T) {
	p := minimalWaypointPuzzle()
	// cell 0 and cell 2 are both in row 0 but two columns apart: not
	// diagonal, but still not orthogonally adjacent.
	b := Board{Puzzle: p, Path: []int{0, 2}}

	v := mustViolations(t, Validator{}, b)
	if !hasRule(v, RuleNonAdjacentStep) {
		t.Errorf("Violations(same-row jump) = %v, want to contain %q", v, RuleNonAdjacentStep)
	}
}

func TestValidator_Violations_WallCrossing(t *testing.T) {
	p := minimalWaypointPuzzle()
	p.Walls = map[[2]int]bool{WallKey(1, 4): true} // wall between cell 1 and cell 4
	b := Board{Puzzle: p, Path: []int{0, 1, 4}}    // steps across the wall

	v := mustViolations(t, Validator{}, b)
	if !hasRule(v, RuleWallCrossing) {
		t.Errorf("Violations(step across wall) = %v, want to contain %q", v, RuleWallCrossing)
	}
}

func TestValidator_Violations_WallOnUnrelatedEdge_NoTrigger(t *testing.T) {
	p := minimalWaypointPuzzle()
	// Wall sits on a completely different edge (2-5); the path below never
	// crosses it, so no wall-crossing violation should fire. Walls are
	// edges, not cells -- a wall touching neither endpoint of a step must
	// not block that step.
	p.Walls = map[[2]int]bool{WallKey(2, 5): true}
	b := Board{Puzzle: p, Path: []int{0, 1, 4}}

	v := mustViolations(t, Validator{}, b)
	if hasRule(v, RuleWallCrossing) {
		t.Errorf("Violations(unrelated wall) = %v, want no %q", v, RuleWallCrossing)
	}
}

func TestValidator_Violations_WallBlocksRegardlessOfDirection(t *testing.T) {
	// A wall is a property of the edge, not a direction: walking B->A across
	// it must be flagged exactly like walking A->B.
	p := Puzzle{
		R: 2, C: 3,
		Waypoint: map[int]int{3: 1, 2: 6},
		Walls:    map[[2]int]bool{WallKey(1, 4): true},
	}
	b := Board{Puzzle: p, Path: []int{3, 4, 1}} // steps 4 -> 1, crossing the same wall

	v := mustViolations(t, Validator{}, b)
	if !hasRule(v, RuleWallCrossing) {
		t.Errorf("Violations(wall crossed in reverse direction) = %v, want to contain %q", v, RuleWallCrossing)
	}
}

func TestValidator_Violations_WaypointOrderViolation(t *testing.T) {
	p := fullyNumberedPuzzle()
	// cell 2 is numbered 3, cell 1 is numbered 2: reaching cell 2 before
	// cell 1 means hitting waypoint 3 before waypoint 2.
	b := Board{Puzzle: p, Path: []int{0, 2, 1}}

	v := mustViolations(t, Validator{}, b)
	if !hasRule(v, RuleWaypointOrder) {
		t.Errorf("Violations(3 before 2) = %v, want to contain %q", v, RuleWaypointOrder)
	}
}

func TestValidator_Violations_WaypointOrder_PartialInProgress_NoTrigger(t *testing.T) {
	p := fullyNumberedPuzzle()
	// Only waypoints 1 then 2 reached so far, in the correct order; waypoint
	// 3 hasn't been reached yet. This must NOT be a violation -- an
	// in-progress path that hasn't gotten out of order yet is fine.
	b := Board{Puzzle: p, Path: []int{0, 1}}

	v := mustViolations(t, Validator{}, b)
	if hasRule(v, RuleWaypointOrder) {
		t.Errorf("Violations(in-order partial path) = %v, want no %q", v, RuleWaypointOrder)
	}
}

func TestValidator_Violations_WrongStart(t *testing.T) {
	p := fullyNumberedPuzzle()
	// Path starts at cell 1 (numbered 2), not cell 0 (numbered 1).
	b := Board{Puzzle: p, Path: []int{1, 0}}

	v := mustViolations(t, Validator{}, b)
	if !hasRule(v, RuleWrongStart) {
		t.Errorf("Violations(wrong start cell) = %v, want to contain %q", v, RuleWrongStart)
	}
	if mustSolved(t, Validator{}, b) {
		t.Errorf("Solved(path not starting at 1) = true, want false")
	}
}

func TestValidator_Violations_TrappedDeadEnd_NotFlagged(t *testing.T) {
	// Path 0-1-4-5-2 is a simple, orthogonal, wall-free, in-order chain, but
	// it strands cell 3 (the K=6 endpoint): 3's only neighbors (0 and 4) are
	// both already visited, so the puzzle can never be completed from here.
	// Per spec, dead-end "trapping" is a live-hint UX nicety, not a hard
	// validator rule -- it must not appear in Violations.
	p := minimalWaypointPuzzle()
	b := Board{Puzzle: p, Path: []int{0, 1, 4, 5, 2}}

	v := mustViolations(t, Validator{}, b)
	if len(v) != 0 {
		t.Errorf("Violations(dead-end trapped path) = %v, want none (trapping is not a hard rule)", v)
	}
}
