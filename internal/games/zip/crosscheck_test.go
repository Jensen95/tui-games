package zip

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// This file is the cross-validation harness required by
// docs/plan/prompts/cross-validation-agent.md: crosscheck.go's solver is an
// independently-structured implementation (different neighbor order,
// different pruning, no shared helpers with impl.go's walkSolutions /
// feasibleCompletion). These tests assert it agrees with the primary
// Solver on every generated puzzle across LIG_SEEDS seeds, that LogicSolve's
// output (when it closes) matches the independent complete solver, and that
// both solvers agree on hand-built ambiguous / near-ambiguous fixtures.

// TestCrossCheck_AgreesWithPrimary_GeneratedPuzzles is the primary
// cross-validation invariant: for every generated puzzle, the independent
// solver and the primary solver must report the same solution count (both
// capped at 2) and, when unique, the identical solution path. A mismatch
// here means the generator's own uniqueness proof (which uses the primary
// solver) cannot be trusted.
func TestCrossCheck_AgreesWithPrimary_GeneratedPuzzles(t *testing.T) {
	n := seedCount()
	gen := Generator{}
	primary := Solver{}

	for _, diff := range noGuessDifficulties {
		diff := diff
		for seed := 1; seed <= n; seed++ {
			seed := seed
			t.Run(diffSeedName(diff, seed), func(t *testing.T) {
				p, genSol, err := mustGenerate(t, gen, diff, engine.NewRand(int64(seed)))
				if err != nil {
					t.Fatalf("Generate(%s, seed=%d) error: %v", diff, seed, err)
				}

				primaryCount := mustCountSolutions(t, primary, p, 2)
				crossCount, crossPath := crossCountSolutions(p, 2)

				if primaryCount != crossCount {
					t.Fatalf("solution count mismatch: primary=%d cross=%d (seed=%d, diff=%s)", primaryCount, crossCount, seed, diff)
				}
				if primaryCount != 1 {
					t.Fatalf("expected a unique solution, both solvers report count=%d (seed=%d, diff=%s)", primaryCount, seed, diff)
				}

				primarySol, ok := mustSolve(t, primary, p)
				if !ok {
					t.Fatalf("primary Solve returned ok=false despite CountSolutions=1")
				}

				if !reflect.DeepEqual(primarySol.Path, crossPath) {
					t.Errorf("solver disagreement on unique solution:\n  primary: %v\n  cross:   %v", primarySol.Path, crossPath)
				}
				if !reflect.DeepEqual(genSol.Path, crossPath) {
					t.Errorf("cross solver's path disagrees with the generator's recorded solution:\n  generated: %v\n  cross:     %v", genSol.Path, crossPath)
				}
				if !crossIsValidComplete(p, crossPath) {
					t.Errorf("cross solver's own solution fails its independent Solved-state re-check: %v", crossPath)
				}
			})
		}
	}
}

// TestCrossCheck_Expert_AgreesWithPrimary is the Expert-tier companion to
// TestCrossCheck_AgreesWithPrimary_GeneratedPuzzles. Expert puzzles are NOT
// required to close under the logic ladder, but their uniqueness guarantee is
// just as non-negotiable — so the independent complete solver must still agree
// with the primary solver on the count (1) and on the single solution path.
// This keeps the "second solver agrees on the unique solution" cross-validation
// invariant covering Expert even though the LogicSolve-closure crosscheck
// deliberately excludes it.
func TestCrossCheck_Expert_AgreesWithPrimary(t *testing.T) {
	n := seedCount()
	gen := Generator{}
	primary := Solver{}

	for seed := 1; seed <= n; seed++ {
		seed := seed
		t.Run(diffSeedName(engine.Expert, seed), func(t *testing.T) {
			p, genSol, err := mustGenerate(t, gen, engine.Expert, engine.NewRand(int64(seed)))
			if err != nil {
				t.Fatalf("Generate(expert, seed=%d) error: %v", seed, err)
			}

			primaryCount := mustCountSolutions(t, primary, p, 2)
			crossCount, crossPath := crossCountSolutions(p, 2)

			if primaryCount != crossCount {
				t.Fatalf("solution count mismatch: primary=%d cross=%d (seed=%d, expert)", primaryCount, crossCount, seed)
			}
			if primaryCount != 1 {
				t.Fatalf("expected a unique Expert solution, both solvers report count=%d (seed=%d)", primaryCount, seed)
			}

			primarySol, ok := mustSolve(t, primary, p)
			if !ok {
				t.Fatalf("primary Solve returned ok=false despite CountSolutions=1")
			}
			if !reflect.DeepEqual(primarySol.Path, crossPath) {
				t.Errorf("solver disagreement on unique Expert solution:\n  primary: %v\n  cross:   %v", primarySol.Path, crossPath)
			}
			if !reflect.DeepEqual(genSol.Path, crossPath) {
				t.Errorf("cross solver's path disagrees with the generator's recorded Expert solution:\n  generated: %v\n  cross:     %v", genSol.Path, crossPath)
			}
			if !crossIsValidComplete(p, crossPath) {
				t.Errorf("cross solver's own Expert solution fails its independent Solved-state re-check: %v", crossPath)
			}
		})
	}
}

// TestCrossCheck_LogicSolve_MatchesIndependentComplete pins the spec's
// cross-validation invariant (docs/plan/docs/02-engine-and-generation.md):
// "LogicSolve output, when it closes, must equal the complete solver's
// unique solution" — checked here against the *independent* complete
// solver, not the primary's own Solve, so a shared bug between the primary
// solver and LogicSolve can't hide.
func TestCrossCheck_LogicSolve_MatchesIndependentComplete(t *testing.T) {
	n := seedCount()
	gen := Generator{}
	primary := Solver{}

	for _, diff := range noGuessDifficulties {
		diff := diff
		for seed := 1; seed <= n; seed++ {
			seed := seed
			t.Run(diffSeedName(diff, seed), func(t *testing.T) {
				p, _, err := mustGenerate(t, gen, diff, engine.NewRand(int64(seed)))
				if err != nil {
					t.Fatalf("Generate(%s, seed=%d) error: %v", diff, seed, err)
				}
				logicSol, closed, _ := mustLogicSolve(t, primary, p)
				if !closed {
					t.Fatalf("LogicSolve did not close a %s-tier puzzle (seed=%d); no-guess tiers must close", diff, seed)
				}
				_, crossPath := crossCountSolutions(p, 2)
				if !reflect.DeepEqual(logicSol.Path, crossPath) {
					t.Errorf("LogicSolve.Path = %v, independent complete solver = %v, want equal", logicSol.Path, crossPath)
				}
			})
		}
	}
}

// TestCrossCheck_AmbiguousFixture_AgreesCountTwo checks both solvers agree
// the hand-built two-solution fixture (solver_test.go's ambiguousPuzzle) is
// non-unique.
func TestCrossCheck_AmbiguousFixture_AgreesCountTwo(t *testing.T) {
	p := ambiguousPuzzle()

	primaryCount := mustCountSolutions(t, Solver{}, p, 2)
	crossCount, _ := crossCountSolutions(p, 2)

	if primaryCount != 2 {
		t.Fatalf("primary CountSolutions(ambiguous, cap=2) = %d, want 2", primaryCount)
	}
	if crossCount != 2 {
		t.Fatalf("cross CountSolutions(ambiguous, cap=2) = %d, want 2", crossCount)
	}
}

// TestCrossCheck_NearAmbiguousFixture_WallKillsAlternative is the
// "near-miss" companion to the ambiguous fixture: it starts from the exact
// same two-solution puzzle and adds a single wall that lies on path B's
// first step (0->3) but not on path A's route at all (path A never visits
// edge 0-3 — see ambiguousPuzzle's doc comment for both paths). That one
// wall should collapse the puzzle from ambiguous (2) to unique (1), with
// both solvers agreeing on the surviving path (A). This directly exercises
// the generator's "wall kills alternative" carving strategy on a fixture
// neither solver's author built the puzzle to fit.
func TestCrossCheck_NearAmbiguousFixture_WallKillsAlternative(t *testing.T) {
	p := ambiguousPuzzle()
	p.Walls = map[[2]int]bool{WallKey(0, 3): true}

	pathA := []int{0, 1, 2, 5, 4, 3, 6, 7, 8}

	// Sanity: the wall must not sit on path A's own route (else we'd have
	// broken the fixture, not merely disambiguated it).
	assertWallsOffSolution(t, p, Solution{Path: pathA})

	primaryCount := mustCountSolutions(t, Solver{}, p, 2)
	crossCount, crossPath := crossCountSolutions(p, 2)

	if primaryCount != 1 {
		t.Fatalf("primary CountSolutions(near-ambiguous, cap=2) = %d, want 1 (wall should kill path B)", primaryCount)
	}
	if crossCount != 1 {
		t.Fatalf("cross CountSolutions(near-ambiguous, cap=2) = %d, want 1", crossCount)
	}
	if !reflect.DeepEqual(crossPath, pathA) {
		t.Errorf("cross solver's surviving path = %v, want %v (path A)", crossPath, pathA)
	}

	primarySol, ok := mustSolve(t, Solver{}, p)
	if !ok {
		t.Fatalf("primary Solve(near-ambiguous) returned ok=false")
	}
	if !reflect.DeepEqual(primarySol.Path, crossPath) {
		t.Errorf("primary/cross disagree on near-ambiguous puzzle's unique path: %v vs %v", primarySol.Path, crossPath)
	}
}

// TestCrossCheck_GoldenAndFullyNumbered_AgreeWithPrimary re-runs the
// hand-built fixtures from solver_test.go through the independent solver as
// an extra sanity net beyond the property-based generated-puzzle sweep.
func TestCrossCheck_GoldenAndFullyNumbered_AgreeWithPrimary(t *testing.T) {
	for name, p := range map[string]Puzzle{
		"minimalWaypoint": minimalWaypointPuzzle(),
		"fullyNumbered":   fullyNumberedPuzzle(),
	} {
		p := p
		t.Run(name, func(t *testing.T) {
			primaryCount := mustCountSolutions(t, Solver{}, p, 2)
			crossCount, crossPath := crossCountSolutions(p, 2)
			if primaryCount != crossCount {
				t.Fatalf("count mismatch: primary=%d cross=%d", primaryCount, crossCount)
			}
			if primaryCount != 1 {
				t.Fatalf("expected unique solution, got count=%d", primaryCount)
			}
			primarySol, ok := mustSolve(t, Solver{}, p)
			if !ok {
				t.Fatalf("primary Solve returned ok=false")
			}
			if !reflect.DeepEqual(primarySol.Path, crossPath) {
				t.Errorf("path mismatch: primary=%v cross=%v", primarySol.Path, crossPath)
			}
		})
	}
}

func diffSeedName(diff engine.Difficulty, seed int) string {
	return fmt.Sprintf("%s/seed=%d", diff, seed)
}
