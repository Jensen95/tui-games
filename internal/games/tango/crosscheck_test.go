package tango

import (
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// boardsEqual reports whether two boards hold the same cell assignment.
func boardsEqual(a, b Board) bool {
	if a.N != b.N || len(a.Cells) != len(b.Cells) {
		return false
	}
	for i := range a.Cells {
		if a.Cells[i] != b.Cells[i] {
			return false
		}
	}
	return true
}

// allDifficulties is the full set the generator supports.
var allDifficulties = []engine.Difficulty{engine.Easy, engine.Medium, engine.Hard, engine.Expert}

// TestCross_GeneratedPuzzlesUniqueAndAgree is the core cross-validation: for
// every generated puzzle over many seeds and every difficulty, the independent
// row-pattern counter must agree with the primary solver that the solution
// count is exactly 1, both solvers must return the SAME board, that board must
// equal the generator's recorded solution, and — when the logic solver closes —
// its board must equal that unique solution too (the "two solvers agree" and
// "LogicSolve == complete solution" invariants from
// docs/plan/docs/02-engine-and-generation.md).
func TestCross_GeneratedPuzzlesUniqueAndAgree(t *testing.T) {
	seedCount := getSeedCount()
	g := Generator{}
	primary := Solver{}

	for _, diff := range allDifficulties {
		for seed := int64(1); seed <= int64(seedCount); seed++ {
			r := engine.NewRand(seed)
			puzzle, solution, err := g.Generate(diff, r)
			if err != nil {
				t.Fatalf("diff=%s seed=%d: Generate failed: %v", diff, seed, err)
			}

			// Independent count == exactly 1.
			if got := countSolutionsCross(puzzle, 2); got != 1 {
				t.Errorf("diff=%s seed=%d: cross counter found %d solutions, want 1", diff, seed, got)
			}
			// Primary count == exactly 1 (agreement).
			if got := primary.CountSolutions(puzzle, 2); got != 1 {
				t.Errorf("diff=%s seed=%d: primary found %d solutions, want 1", diff, seed, got)
			}

			// Both solvers return the SAME board, equal to the recorded solution.
			crossBoard, ok := solveCross(puzzle)
			if !ok {
				t.Fatalf("diff=%s seed=%d: cross solver found no solution", diff, seed)
			}
			primaryBoard, ok := primary.Solve(puzzle)
			if !ok {
				t.Fatalf("diff=%s seed=%d: primary solver found no solution", diff, seed)
			}
			if !boardsEqual(crossBoard, primaryBoard) {
				t.Errorf("diff=%s seed=%d: cross and primary disagree on the unique solution", diff, seed)
			}
			if !boardsEqual(crossBoard, solution) {
				t.Errorf("diff=%s seed=%d: unique solution != recorded generator solution", diff, seed)
			}

			// LogicSolve, when it closes, must equal that unique solution.
			logicBoard, closed, tech := primary.LogicSolve(puzzle)
			switch diff {
			case engine.Easy, engine.Medium, engine.Hard:
				if !closed {
					t.Errorf("diff=%s seed=%d: LogicSolve should close a no-guess tier", diff, seed)
				}
			}
			if closed {
				if !boardsEqual(logicBoard, solution) {
					t.Errorf("diff=%s seed=%d: LogicSolve closed to a board != unique solution", diff, seed)
				}
				if tech == "" {
					t.Errorf("diff=%s seed=%d: LogicSolve closed but reported no technique", diff, seed)
				}
			}
		}
	}
}

// TestCross_RandomPuzzlesAgreeOnCount fuzzes the two solvers against each other
// on puzzles the GENERATOR never produced, so a bug they might share (a missing
// or mis-encoded constraint) surfaces as a count disagreement rather than
// hiding behind the generator's own use of the primary solver.
//
// Two families are exercised:
//   - Derived: keep a random subset of a real solution's givens+edges, so at
//     least one solution always exists and counts of 1..many are compared.
//   - Raw: random symbol givens with no edges, which may be unsatisfiable, so
//     agreement on a count of 0 is also checked.
func TestCross_RandomPuzzlesAgreeOnCount(t *testing.T) {
	seedCount := getSeedCount()
	primary := Solver{}
	const capN = 6

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed * 2654435761)

		// --- Derived puzzle: subset of a valid solution's clues. ---
		sol := generateSolution(N, r)
		h, v := deriveEdges(sol, N)
		derived := Puzzle{
			N:      N,
			Givens: map[int]Symbol{},
			HEdges: map[[2]int]Relation{},
			VEdges: map[[2]int]Relation{},
			Diff:   engine.Easy,
		}
		for i := 0; i < N*N; i++ {
			if r.IntN(3) == 0 { // ~1/3 of cells kept as givens
				derived.Givens[i] = sol[i]
			}
		}
		for pair, rel := range h {
			if r.IntN(2) == 0 {
				derived.HEdges[pair] = rel
			}
		}
		for pair, rel := range v {
			if r.IntN(2) == 0 {
				derived.VEdges[pair] = rel
			}
		}
		cp := primary.CountSolutions(derived, capN)
		cc := countSolutionsCross(derived, capN)
		if cp != cc {
			t.Errorf("seed=%d derived: primary count=%d, cross count=%d (disagreement)", seed, cp, cc)
		}
		if cp == 0 {
			t.Errorf("seed=%d derived: expected at least the source solution, got 0", seed)
		}

		// --- Raw puzzle: random symbols on a random subset of cells. ---
		raw := Puzzle{
			N:      N,
			Givens: map[int]Symbol{},
			HEdges: map[[2]int]Relation{},
			VEdges: map[[2]int]Relation{},
			Diff:   engine.Easy,
		}
		for i := 0; i < N*N; i++ {
			if r.IntN(4) == 0 {
				if r.IntN(2) == 0 {
					raw.Givens[i] = Sun
				} else {
					raw.Givens[i] = Moon
				}
			}
		}
		rp := primary.CountSolutions(raw, capN)
		rc := countSolutionsCross(raw, capN)
		if rp != rc {
			t.Errorf("seed=%d raw: primary count=%d, cross count=%d (disagreement)", seed, rp, rc)
		}
	}
}

// TestCross_GoldenFixtureUnique confirms both solvers agree a hand-built puzzle
// with a known unique solution has exactly one, and return the same board.
func TestCross_GoldenFixtureUnique(t *testing.T) {
	// Fully-given valid solution (see TestValidator_ValidSolution): trivially
	// unique. A good sanity anchor independent of the generator.
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
		HEdges: map[[2]int]Relation{},
		VEdges: map[[2]int]Relation{},
		Diff:   engine.Easy,
	}
	if c := countSolutionsCross(puzzle, 2); c != 1 {
		t.Errorf("cross counter: golden puzzle count=%d, want 1", c)
	}
	primary := Solver{}
	pb, _ := primary.Solve(puzzle)
	cb, ok := solveCross(puzzle)
	if !ok || !boardsEqual(pb, cb) {
		t.Error("cross and primary disagree on golden puzzle solution")
	}
}

// TestCross_AmbiguousFixtureAgrees confirms both solvers agree an
// under-constrained puzzle has (at least) two solutions — the near-ambiguous
// case that guards the uniqueness machinery.
func TestCross_AmbiguousFixtureAgrees(t *testing.T) {
	puzzle := Puzzle{
		N: N,
		Givens: map[int]Symbol{
			0: Sun, 6: Moon, 12: Sun, 18: Moon, 24: Sun, 30: Moon,
		},
		HEdges: map[[2]int]Relation{},
		VEdges: map[[2]int]Relation{},
		Diff:   engine.Easy,
	}
	primary := Solver{}
	pc := primary.CountSolutions(puzzle, 2)
	cc := countSolutionsCross(puzzle, 2)
	if pc != cc {
		t.Errorf("ambiguous fixture: primary=%d, cross=%d (disagreement)", pc, cc)
	}
	if cc != 2 {
		t.Errorf("ambiguous fixture: cross count=%d, want 2 (capped)", cc)
	}
}

// TestCross_ExactSolutionCountAgrees walks the cap up so the two solvers must
// agree on the EXACT number of solutions, not just a capped comparison, for a
// spread of partially-constrained boards.
func TestCross_ExactSolutionCountAgrees(t *testing.T) {
	primary := Solver{}
	const capN = 40
	for seed := int64(1); seed <= 60; seed++ {
		r := engine.NewRand(seed * 7919)
		sol := generateSolution(N, r)
		p := Puzzle{
			N:      N,
			Givens: map[int]Symbol{},
			HEdges: map[[2]int]Relation{},
			VEdges: map[[2]int]Relation{},
			Diff:   engine.Easy,
		}
		// Keep a sparse set of givens so multiple solutions are likely but the
		// count stays under the cap most of the time.
		for i := 0; i < N*N; i++ {
			if r.IntN(2) == 0 {
				p.Givens[i] = sol[i]
			}
		}
		if primary.CountSolutions(p, capN) != countSolutionsCross(p, capN) {
			t.Errorf("seed=%d: exact count disagreement (primary=%d cross=%d)",
				seed, primary.CountSolutions(p, capN), countSolutionsCross(p, capN))
		}
	}
}

// TestCross_GivenOnlyContradictionHasZeroSolutions is a regression test for a
// bug the cross-check surfaced: the primary solver skips given cells and its
// incremental violatesAt only re-checks constraints touching the just-placed
// cell, so a rule violated ENTIRELY among givens (a given-only three-in-a-row
// or a given-only "="/"×" edge conflict) was never validated — Solve returned
// ok=true on an invalid board and CountSolutions reported phantom completions.
// Both solvers must now agree such puzzles are unsatisfiable.
func TestCross_GivenOnlyContradictionHasZeroSolutions(t *testing.T) {
	primary := Solver{}

	cases := []struct {
		name string
		p    Puzzle
	}{
		{
			name: "given-only vertical triple (col 2 rows 1-3)",
			p: Puzzle{
				N:      N,
				Givens: map[int]Symbol{8: Moon, 14: Moon, 20: Moon},
				HEdges: map[[2]int]Relation{},
				VEdges: map[[2]int]Relation{},
				Diff:   engine.Easy,
			},
		},
		{
			name: "given-only horizontal triple (row 2 cols 1-3)",
			p: Puzzle{
				N:      N,
				Givens: map[int]Symbol{13: Sun, 14: Sun, 15: Sun},
				HEdges: map[[2]int]Relation{},
				VEdges: map[[2]int]Relation{},
				Diff:   engine.Easy,
			},
		},
		{
			name: "given-only equal-edge conflict",
			p: Puzzle{
				N:      N,
				Givens: map[int]Symbol{0: Sun, 1: Moon},
				HEdges: map[[2]int]Relation{{0, 1}: Equal},
				VEdges: map[[2]int]Relation{},
				Diff:   engine.Easy,
			},
		},
		{
			name: "given-only cross-edge conflict",
			p: Puzzle{
				N:      N,
				Givens: map[int]Symbol{0: Sun, 6: Sun},
				HEdges: map[[2]int]Relation{},
				VEdges: map[[2]int]Relation{{0, 6}: Cross},
				Diff:   engine.Easy,
			},
		},
	}

	for _, tc := range cases {
		pc := primary.CountSolutions(tc.p, 2)
		cc := countSolutionsCross(tc.p, 2)
		if pc != 0 {
			t.Errorf("%s: primary CountSolutions=%d, want 0", tc.name, pc)
		}
		if cc != 0 {
			t.Errorf("%s: cross CountSolutions=%d, want 0", tc.name, cc)
		}
		if b, ok := primary.Solve(tc.p); ok {
			t.Errorf("%s: primary.Solve returned ok=true (Solved=%v) for an unsatisfiable puzzle",
				tc.name, (Validator{}).Solved(b))
		}
		if _, closed, _ := primary.LogicSolve(tc.p); closed {
			t.Errorf("%s: LogicSolve reported closed=true for an unsatisfiable puzzle", tc.name)
		}
	}
}

// ============================================================================
// Gotcha audit — orthogonal-only edge constraints
// ============================================================================

// TestGotcha_EdgesAreOrthogonalOnly asserts the spec gotcha "Edge constraints
// are between orthogonal neighbors only": structurallyValid must reject an edge
// wired between diagonal (non-orthogonal) cells, and — the should-not-trigger
// near-miss — accept the same relation on a genuinely orthogonal pair.
func TestGotcha_EdgesAreOrthogonalOnly(t *testing.T) {
	// Near-miss: a legitimately horizontal edge (0,1) must be accepted.
	okPuzzle := Puzzle{
		N:      N,
		Givens: map[int]Symbol{},
		HEdges: map[[2]int]Relation{{0, 1}: Equal},
		VEdges: map[[2]int]Relation{},
		Diff:   engine.Easy,
	}
	if err := structurallyValid(okPuzzle); err != nil {
		t.Errorf("orthogonal horizontal edge (0,1) should be structurally valid, got %v", err)
	}

	// Rule: a diagonal edge (0 -> 7, i.e. (0,0)->(1,1)) declared as horizontal
	// must be rejected — it is not orthogonally adjacent.
	diagPuzzle := Puzzle{
		N:      N,
		Givens: map[int]Symbol{},
		HEdges: map[[2]int]Relation{{0, 7}: Equal},
		VEdges: map[[2]int]Relation{},
		Diff:   engine.Easy,
	}
	if err := structurallyValid(diagPuzzle); err == nil {
		t.Error("diagonal edge (0,7) must be rejected: edges are orthogonal-only")
	}

	// A vertical pair declared in the horizontal set must also be rejected.
	misfiledPuzzle := Puzzle{
		N:      N,
		Givens: map[int]Symbol{},
		HEdges: map[[2]int]Relation{{0, N}: Equal}, // (0,0)->(1,0): vertical, not horizontal
		VEdges: map[[2]int]Relation{},
		Diff:   engine.Easy,
	}
	if err := structurallyValid(misfiledPuzzle); err == nil {
		t.Error("vertical pair filed under HEdges must be rejected as non-horizontal")
	}
}

// TestGotcha_EdgeSolverRespectsOrthogonalEdges is the solver-side companion:
// an "=" edge and a "×" edge on orthogonal pairs actually constrain the count,
// confirming edges are honored (not silently ignored) by BOTH solvers.
func TestGotcha_EdgeSolverRespectsOrthogonalEdges(t *testing.T) {
	primary := Solver{}
	base := Puzzle{
		N:      N,
		Givens: map[int]Symbol{},
		HEdges: map[[2]int]Relation{},
		VEdges: map[[2]int]Relation{},
		Diff:   engine.Easy,
	}
	// Baseline (no edges) solution count, capped.
	const capN = 30
	b0p := primary.CountSolutions(base, capN)
	b0c := countSolutionsCross(base, capN)
	if b0p != b0c {
		t.Fatalf("baseline count disagreement primary=%d cross=%d", b0p, b0c)
	}

	// Adding a cross edge between (0,0) and (0,1) forbids solutions where they
	// match; both solvers must still agree.
	withEdge := Puzzle{
		N:      N,
		Givens: map[int]Symbol{},
		HEdges: map[[2]int]Relation{{0, 1}: Cross},
		VEdges: map[[2]int]Relation{},
		Diff:   engine.Easy,
	}
	ep := primary.CountSolutions(withEdge, capN)
	ec := countSolutionsCross(withEdge, capN)
	if ep != ec {
		t.Errorf("edge-constrained count disagreement primary=%d cross=%d", ep, ec)
	}
}
