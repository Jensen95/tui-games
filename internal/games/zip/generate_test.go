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
