package queens

import (
	"reflect"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

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
