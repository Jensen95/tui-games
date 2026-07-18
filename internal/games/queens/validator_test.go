package queens

import (
	"reflect"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// TestValidator_Solved_KnownValidSolution pins the TDD-matrix line:
// "Known valid solution -> true."
func TestValidator_Solved_KnownValidSolution(t *testing.T) {
	v := NewValidator()
	board := classicNonAttacking5()

	if !v.Solved(board) {
		t.Fatalf("Solved(%+v) = false, want true for a valid complete N=5 placement", board)
	}
}

// TestValidator_Solved_RejectsIncomplete pins the partial-validator contract:
// a board with fewer than N queens is never "solved", even if what's placed
// so far is fully legal.
func TestValidator_Solved_RejectsIncomplete(t *testing.T) {
	v := NewValidator()
	region := baseRegion5x5()
	board := newBoard(5, region, []engine.Cell{{Row: 0, Col: 0}})

	if v.Solved(board) {
		t.Fatalf("Solved() = true for a board with only 1 of 5 queens placed")
	}
}

// TestValidator_Violations_SameRow pins: "Two queens same row ... -> the
// specific violation." Isolated so ONLY the same-row rule fires: different
// regions, chebyshev distance 3 (not adjacent).
func TestValidator_Violations_SameRow(t *testing.T) {
	v := NewValidator()
	region := baseRegion5x5()
	board := newBoard(5, region, []engine.Cell{{Row: 0, Col: 0}, {Row: 0, Col: 3}})

	got := v.Violations(board)
	if !hasRule(got, RuleSameRow) {
		t.Errorf("Violations() = %+v, want it to contain rule %q", got, RuleSameRow)
	}
	if len(got) != 1 {
		t.Errorf("Violations() = %+v, want exactly 1 violation (same-row only)", got)
	}
}

// TestValidator_Violations_SameCol pins: "Two queens ... same col ... -> the
// specific violation." Isolated: different regions, chebyshev distance 3.
func TestValidator_Violations_SameCol(t *testing.T) {
	v := NewValidator()
	region := baseRegion5x5()
	board := newBoard(5, region, []engine.Cell{{Row: 0, Col: 0}, {Row: 3, Col: 0}})

	got := v.Violations(board)
	if !hasRule(got, RuleSameCol) {
		t.Errorf("Violations() = %+v, want it to contain rule %q", got, RuleSameCol)
	}
	if len(got) != 1 {
		t.Errorf("Violations() = %+v, want exactly 1 violation (same-col only)", got)
	}
}

// TestValidator_Violations_SameRegion pins: "Two queens ... same region ->
// the specific violation." Isolated: different row, different col, chebyshev
// distance 2 (not adjacent), same region id (the base fixture's override).
func TestValidator_Violations_SameRegion(t *testing.T) {
	v := NewValidator()
	region := baseRegion5x5()
	board := newBoard(5, region, []engine.Cell{{Row: 1, Col: 1}, {Row: 3, Col: 3}})

	got := v.Violations(board)
	if !hasRule(got, RuleSameRegion) {
		t.Errorf("Violations() = %+v, want it to contain rule %q", got, RuleSameRegion)
	}
	if len(got) != 1 {
		t.Errorf("Violations() = %+v, want exactly 1 violation (same-region only)", got)
	}
}

// TestValidator_Violations_CornerAdjacent pins: "Two queens corner-adjacent
// -> adjacency violation (guards the 8-neighbor rule)." Isolated: different
// row/col/region, chebyshev distance exactly 1 via a diagonal step.
func TestValidator_Violations_CornerAdjacent(t *testing.T) {
	v := NewValidator()
	region := baseRegion5x5()
	board := newBoard(5, region, []engine.Cell{{Row: 1, Col: 1}, {Row: 2, Col: 2}})

	got := v.Violations(board)
	if !hasRule(got, RuleAdjacent) {
		t.Errorf("Violations() = %+v, want it to contain rule %q for a corner-adjacent pair", got, RuleAdjacent)
	}
	if len(got) != 1 {
		t.Errorf("Violations() = %+v, want exactly 1 violation (adjacency only)", got)
	}
}

// TestValidator_Violations_EdgeAdjacent pins: "Two queens edge-adjacent ->
// adjacency violation." An edge-adjacent pair necessarily also shares a row
// or column (moving by exactly one cell orthogonally), so both rules are
// expected to fire here.
func TestValidator_Violations_EdgeAdjacent(t *testing.T) {
	v := NewValidator()
	region := baseRegion5x5()
	board := newBoard(5, region, []engine.Cell{{Row: 1, Col: 1}, {Row: 1, Col: 2}})

	got := v.Violations(board)
	if !hasRule(got, RuleAdjacent) {
		t.Errorf("Violations() = %+v, want it to contain rule %q for an edge-adjacent pair", got, RuleAdjacent)
	}
	want := map[string]bool{RuleAdjacent: true, RuleSameRow: true}
	if gotSet := ruleSet(got); !reflect.DeepEqual(gotSet, want) {
		t.Errorf("Violations() rule set = %+v, want exactly %+v", gotSet, want)
	}
}

// TestValidator_NoViolation_SameDiagonalNotAdjacent guards the single most
// common Queens implementation bug per the spec: "the diagonal rule is local
// only." Two queens on the SAME full diagonal (chebyshev distance 3, not
// the classic chess "any distance on a diagonal" rule) must raise NO
// violation at all.
func TestValidator_NoViolation_SameDiagonalNotAdjacent(t *testing.T) {
	v := NewValidator()
	region := baseRegion5x5()
	board := newBoard(5, region, []engine.Cell{{Row: 0, Col: 0}, {Row: 3, Col: 3}})

	got := v.Violations(board)
	if len(got) != 0 {
		t.Errorf("Violations() = %+v, want none: same-diagonal queens 3 apart must NOT be flagged as adjacent (classic full-diagonal chess bug)", got)
	}
}

// TestValidator_NoViolation_EmptyBoard pins the partial-validator contract:
// an empty board (no queens at all) has no violations.
func TestValidator_NoViolation_EmptyBoard(t *testing.T) {
	v := NewValidator()
	region := baseRegion5x5()
	board := newBoard(5, region, nil)

	if got := v.Violations(board); len(got) != 0 {
		t.Errorf("Violations() on an empty board = %+v, want none", got)
	}
}
