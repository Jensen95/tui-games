package queens

import (
	"reflect"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// TestGenerate_Expert_DeterministicAndUnique pins the audit's two Expert
// "must hold" guarantees: every (Expert, seed) pair is byte-identical across
// two generations, and every Expert board has exactly one solution. Expert
// relaxes the no-guess closure gate (see generator.go), so it is not exercised
// by the Easy/Medium/Hard property/crosscheck batches; this test is its
// dedicated determinism + uniqueness coverage.
//
// The seed count is a small FIXED sample, deliberately NOT scaled by LIG_SEEDS.
// Expert forces the largest boards (N=11) through the full complete-solver
// uniqueness search twice per seed, which is ~an order of magnitude slower than
// the other tiers and dominates further under the race detector — scaling it to
// the 100/250-seed CI batches (let alone the 5000-seed nightly) blows the test
// timeout. Determinism and uniqueness are structural properties a modest sample
// demonstrates as well as a large one; broad per-seed coverage is carried by
// the no-guess property/crosscheck batches, which run the cheaper tiers.
func TestGenerate_Expert_DeterministicAndUnique(t *testing.T) {
	gen := NewGenerator()
	solver := NewSolver()
	const n = 16
	for i := 1; i <= n; i++ {
		seed := int64(i)
		p1, s1, err1 := gen.Generate(engine.Expert, engine.NewRand(seed))
		if err1 != nil {
			t.Fatalf("Generate(Expert, seed=%d) run1 error: %v", seed, err1)
		}
		p2, s2, err2 := gen.Generate(engine.Expert, engine.NewRand(seed))
		if err2 != nil {
			t.Fatalf("Generate(Expert, seed=%d) run2 error: %v", seed, err2)
		}
		if !reflect.DeepEqual(p1, p2) {
			t.Errorf("Generate(Expert, seed=%d): puzzle not deterministic:\n run1=%+v\n run2=%+v", seed, p1, p2)
		}
		if !reflect.DeepEqual(s1, s2) {
			t.Errorf("Generate(Expert, seed=%d): solution not deterministic", seed)
		}
		if got := solver.CountSolutions(p1, 2); got != 1 {
			t.Errorf("Generate(Expert, seed=%d): CountSolutions(p,2)=%d, want 1 (unique)", seed, got)
		}
	}
}

// TestGenerate_Determinism_SameSeedSamePuzzle pins: "Determinism: same seed
// => identical board." Generating twice from engine.NewRand(seed) with the
// same seed and difficulty must produce byte-for-byte identical Puzzle and
// Solution values.
func TestGenerate_Determinism_SameSeedSamePuzzle(t *testing.T) {
	seeds := []int64{1, 2, 42, 12345}
	diffs := []engine.Difficulty{engine.Easy, engine.Medium, engine.Hard}

	gen := NewGenerator()
	for _, seed := range seeds {
		for _, diff := range diffs {
			p1, s1, err1 := gen.Generate(diff, engine.NewRand(seed))
			if err1 != nil {
				t.Fatalf("Generate(diff=%v, seed=%d) run1 returned error: %v", diff, seed, err1)
			}
			p2, s2, err2 := gen.Generate(diff, engine.NewRand(seed))
			if err2 != nil {
				t.Fatalf("Generate(diff=%v, seed=%d) run2 returned error: %v", diff, seed, err2)
			}

			if !reflect.DeepEqual(p1, p2) {
				t.Errorf("Generate(diff=%v, seed=%d): puzzle mismatch across identical seeds:\n  run1=%+v\n  run2=%+v", diff, seed, p1, p2)
			}
			if !reflect.DeepEqual(s1, s2) {
				t.Errorf("Generate(diff=%v, seed=%d): solution mismatch across identical seeds:\n  run1=%+v\n  run2=%+v", diff, seed, s1, s2)
			}
		}
	}
}

// TestGenerate_Determinism_DifferentSeedsUsuallyDiffer is a sanity check on
// the fixture above: it would be a vacuous pass if Generate ignored its rand
// source entirely and always returned the same puzzle. At least one pair
// among several distinct seeds must differ.
func TestGenerate_Determinism_DifferentSeedsUsuallyDiffer(t *testing.T) {
	gen := NewGenerator()
	var puzzles []Puzzle
	for _, seed := range []int64{1, 2, 3, 4, 5} {
		p, _, err := gen.Generate(engine.Easy, engine.NewRand(seed))
		if err != nil {
			t.Fatalf("Generate(seed=%d) returned error: %v", seed, err)
		}
		puzzles = append(puzzles, p)
	}

	allSame := true
	for _, p := range puzzles[1:] {
		if !reflect.DeepEqual(p, puzzles[0]) {
			allSame = false
			break
		}
	}
	if allSame {
		t.Errorf("Generate produced an identical puzzle for 5 distinct seeds; rand source is not being used")
	}
}
