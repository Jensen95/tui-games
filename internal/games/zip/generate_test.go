package zip

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// noGuessDifficulties are the bands the generation invariant guarantees are
// closable by the logic/forced-move solver with no guessing (see
// docs/plan/docs/02-engine-and-generation.md's solvability tiers table).
var noGuessDifficulties = []engine.Difficulty{engine.Easy, engine.Medium, engine.Hard}

// TestGenerator_Invariant_PerSeed is the property test from
// docs/plan/games/zip.md's "Generator (property-based over many seeds &
// sizes)" section: for every seed 1..N (N from LIG_SEEDS, default 250) and
// every no-guess difficulty band, the generated puzzle must satisfy the
// full generation invariant.
func TestGenerator_Invariant_PerSeed(t *testing.T) {
	n := seedCount()
	gen := Generator{}
	val := Validator{}
	sol := Solver{}

	for _, diff := range noGuessDifficulties {
		diff := diff
		for seed := 1; seed <= n; seed++ {
			seed := seed
			t.Run(fmt.Sprintf("%s/seed=%d", diff, seed), func(t *testing.T) {
				p, s, err := mustGenerate(t, gen, diff, engine.NewRand(int64(seed)))
				if err != nil {
					t.Fatalf("Generate(%s, seed=%d) error: %v", diff, seed, err)
				}

				// Invariant 1: the recorded solution passes Solved().
				b := Board{Puzzle: p, Path: s.Path}
				if !mustSolved(t, val, b) {
					t.Errorf("Validator.Solved(recorded solution) = false, want true")
				}

				// Invariant 2: exactly one solution.
				if got := mustCountSolutions(t, sol, p, 2); got != 1 {
					t.Errorf("CountSolutions(p, cap=2) = %d, want 1 (unique)", got)
				}

				// Invariant 3: no-guess tier closes by pure logic.
				_, closed, _ := mustLogicSolve(t, sol, p)
				if !closed {
					t.Errorf("LogicSolve did not close a %s-tier puzzle (must be no-guess solvable)", diff)
				}

				// Structural invariant: walls lie only on non-solution edges,
				// so the intended path stays legal.
				assertWallsOffSolution(t, p, s)

				// Structural invariant: waypoint numbers form a contiguous
				// 1..K and appear in path order.
				assertWaypointsContiguousInPathOrder(t, p, s)
			})
		}
	}
}

// TestGenerator_SizeLadder_Monotonic is the regression guard against the
// mid-ladder collapse this change fixed. Grid size is the only difficulty lever
// that survives Zip's densify-to-unique loop for the no-guess tiers (see
// sizeFor's doc comment), so if Medium and Hard ever share a size again they
// become near-indistinguishable — precisely the defect that had Medium and Hard
// both at 6x6 with ~93% numbered cells (and Medium even edging Hard on average
// waypoints). This test pins the no-guess ladder to a strictly increasing cell
// count so that collapse cannot silently return.
//
// It is fast and deterministic: it inspects only sizeFor's output (via the
// dimensions on freshly generated puzzles), not any timing or seed sweep.
//
// Expert is treated separately. It relaxes the no-guess guarantee and is
// distinguished from Hard by search difficulty rather than board area, so it is
// deliberately kept at its sparse, contract-correct 6x6 and is therefore SMALLER
// than 7x7 Hard. The ladder is strictly increasing only across Easy < Medium <
// Hard; for Expert we assert only that it retains a fixed, sane board (>= Medium),
// not that it is >= Hard by area.
func TestGenerator_SizeLadder_Monotonic(t *testing.T) {
	gen := Generator{}

	cells := func(diff engine.Difficulty) int {
		// Generate once (seed is irrelevant to dimensions, which sizeFor fixes
		// per tier) and read the concrete R*C off the puzzle, so the guard
		// exercises the real generated size rather than re-reading sizeFor.
		p, _, err := gen.Generate(diff, engine.NewRand(1))
		if err != nil {
			t.Fatalf("Generate(%s) error: %v", diff, err)
		}
		return p.R * p.C
	}

	easy := cells(engine.Easy)
	medium := cells(engine.Medium)
	hard := cells(engine.Hard)
	expert := cells(engine.Expert)

	if !(easy < medium && medium < hard) {
		t.Errorf("no-guess size ladder is not strictly increasing: Easy=%d, Medium=%d, Hard=%d (want Easy < Medium < Hard so the tiers stay distinguishable)", easy, medium, hard)
	}

	// Expert is ranked above Hard by search difficulty, not board area, and is
	// intentionally sparse on a smaller board. Guard only that it keeps a sane,
	// fixed board size (at least Medium's), not that it out-sizes Hard.
	if expert < medium {
		t.Errorf("Expert grid shrank below Medium: Expert=%d, Medium=%d (want Expert >= Medium)", expert, medium)
	}
}

// assertWallsOffSolution checks that no wall blocks a step the recorded
// solution actually takes.
func assertWallsOffSolution(t *testing.T, p Puzzle, s Solution) {
	t.Helper()
	for i := 0; i+1 < len(s.Path); i++ {
		key := WallKey(s.Path[i], s.Path[i+1])
		if p.Walls[key] {
			t.Errorf("wall on edge %v lies on the solution path (step %d: %d -> %d)", key, i, s.Path[i], s.Path[i+1])
		}
	}
}

// assertWaypointsContiguousInPathOrder checks Waypoint values are exactly
// {1..K} and that walking the solution path encounters them in that order.
func assertWaypointsContiguousInPathOrder(t *testing.T, p Puzzle, s Solution) {
	t.Helper()
	k := len(p.Waypoint)
	if k < 2 {
		t.Errorf("len(Waypoint) = %d, want >= 2 (start cell 1 and end cell K)", k)
		return
	}
	seen := make([]bool, k+1)
	for cell, num := range p.Waypoint {
		if num < 1 || num > k {
			t.Errorf("waypoint at cell %d has number %d, want in contiguous range 1..%d", cell, num, k)
			continue
		}
		seen[num] = true
	}
	for i := 1; i <= k; i++ {
		if !seen[i] {
			t.Errorf("waypoint numbers not contiguous 1..%d: missing %d", k, i)
		}
	}

	var order []int
	for _, cell := range s.Path {
		if num, ok := p.Waypoint[cell]; ok {
			order = append(order, num)
		}
	}
	want := make([]int, k)
	for i := range want {
		want[i] = i + 1
	}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("waypoint numbers along the solution path = %v, want %v (ascending 1..K)", order, want)
	}
}

// expertStallFloor is the minimum fraction of Expert puzzles that must stall
// the forced-move (logic) ladder over the seed sweep. Expert's whole reason to
// exist is that it "may require limited search" (docs/plan/docs/02-engine-and-
// generation.md tiers table); if a clear majority did NOT stall, Expert would
// be statistically indistinguishable from Hard — the exact defect this tier was
// built to fix. The generator rejects logic-closable candidates, so the true
// stall rate is ~100%; the threshold is set well below that so the test asserts
// the property robustly without ever flaking on a loaded box.
const expertStallFloor = 0.60

// TestGenerator_Expert_UniqueValidAndMostlyStalls is the Expert-tier property
// test. Unlike the no-guess tiers (TestGenerator_Invariant_PerSeed), Expert is
// NOT required to close under the logic ladder — it only guarantees exactly one
// solution. This test pins the Expert contract across the LIG_SEEDS seed sweep:
//
//   - every Expert puzzle's recorded solution is Valid (Solved == true);
//   - every Expert puzzle is uniquely solvable (CountSolutions(cap=2) == 1) —
//     the non-negotiable guarantee shared with every other tier;
//   - walls lie only on non-solution edges and waypoints are a contiguous 1..K
//     in path order (same structural invariants as the other tiers);
//   - a clear majority of generated Expert puzzles STALL the logic ladder
//     (LogicSolve closes == false), i.e. they genuinely require search.
func TestGenerator_Expert_UniqueValidAndMostlyStalls(t *testing.T) {
	n := seedCount()
	gen := Generator{}
	val := Validator{}
	sol := Solver{}

	stalled := 0
	for seed := 1; seed <= n; seed++ {
		p, s, err := mustGenerate(t, gen, engine.Expert, engine.NewRand(int64(seed)))
		if err != nil {
			t.Fatalf("Generate(expert, seed=%d) error: %v", seed, err)
		}

		// The recorded solution must be Valid.
		b := Board{Puzzle: p, Path: s.Path}
		if !mustSolved(t, val, b) {
			t.Errorf("seed=%d: Validator.Solved(recorded solution) = false, want true", seed)
		}

		// Exactly one solution — non-negotiable even for Expert.
		if got := mustCountSolutions(t, sol, p, 2); got != 1 {
			t.Errorf("seed=%d: CountSolutions(p, cap=2) = %d, want 1 (unique)", seed, got)
		}

		// Same structural invariants as the no-guess tiers.
		assertWallsOffSolution(t, p, s)
		assertWaypointsContiguousInPathOrder(t, p, s)

		// Expert must NOT be required to close under the logic ladder; count how
		// many genuinely stall (need search).
		if _, closed, _ := mustLogicSolve(t, sol, p); !closed {
			stalled++
		}
	}

	rate := float64(stalled) / float64(n)
	if rate < expertStallFloor {
		t.Errorf("Expert stall rate = %.1f%% (%d/%d), want >= %.0f%%: Expert puzzles must mostly require search, else they are indistinguishable from Hard",
			100*rate, stalled, n, 100*expertStallFloor)
	}
}

// TestVerify_AcceptsExpert_WithoutLogicClosure pins the Verify/label coherence
// required for Expert: Entry().Verify re-checks the generation invariant from
// the encoded clues alone, and for Expert that invariant is uniqueness only —
// it must NOT demand logic closure. A generated Expert puzzle that stalls the
// ladder must still verify.
func TestVerify_AcceptsExpert_WithoutLogicClosure(t *testing.T) {
	n := seedCount()
	if n > 50 {
		n = 50 // Verify re-runs the complete solver; a slice of the sweep suffices.
	}
	entry := Entry()
	gen := Generator{}
	sol := Solver{}

	sawStall := false
	for seed := 1; seed <= n; seed++ {
		p, _, err := mustGenerate(t, gen, engine.Expert, engine.NewRand(int64(seed)))
		if err != nil {
			t.Fatalf("Generate(expert, seed=%d) error: %v", seed, err)
		}
		if _, closed, _ := mustLogicSolve(t, sol, p); !closed {
			sawStall = true
		}
		if err := entry.Verify(Encode(p)); err != nil {
			t.Errorf("seed=%d: Entry().Verify rejected a valid Expert puzzle: %v", seed, err)
		}
	}
	if !sawStall {
		t.Errorf("expected at least one Expert puzzle in the first %d seeds to stall the logic ladder; the test would not be exercising the no-logic-closure path otherwise", n)
	}
}

// TestGenerator_Determinism_SameSeedSamePuzzle pins docs/plan/games/zip.md's
// "Determinism: same seed => identical puzzle" requirement. Bounded to a
// single difficulty band (still honoring LIG_SEEDS for the seed count) so
// the determinism check -- which calls Generate twice per seed -- doesn't
// multiply the cost of the full per-difficulty invariant sweep above.
func TestGenerator_Determinism_SameSeedSamePuzzle(t *testing.T) {
	n := seedCount()
	gen := Generator{}

	for seed := 1; seed <= n; seed++ {
		seed := seed
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			p1, s1, err1 := mustGenerate(t, gen, engine.Easy, engine.NewRand(int64(seed)))
			if err1 != nil {
				t.Fatalf("Generate(seed=%d) call 1 error: %v", seed, err1)
			}
			p2, s2, err2 := mustGenerate(t, gen, engine.Easy, engine.NewRand(int64(seed)))
			if err2 != nil {
				t.Fatalf("Generate(seed=%d) call 2 error: %v", seed, err2)
			}

			if !reflect.DeepEqual(p1, p2) {
				t.Errorf("Generate(seed=%d) produced different puzzles across two calls:\n  1: %+v\n  2: %+v", seed, p1, p2)
			}
			if !reflect.DeepEqual(s1, s2) {
				t.Errorf("Generate(seed=%d) produced different solutions across two calls:\n  1: %+v\n  2: %+v", seed, s1, s2)
			}
		})
	}
}
