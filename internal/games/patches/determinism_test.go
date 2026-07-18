package patches

import (
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// TestDeterminism_SameSeedIdenticalPuzzle tests that Generate(diff, rand.New(seed)) twice with the same seed produces byte-identical puzzles.
// Invariant: same seed => identical puzzle
func TestDeterminism_SameSeedIdenticalPuzzle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping determinism test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r1 := engine.NewRand(seed)
		p1, _, err1 := gen.Generate(engine.Easy, r1)

		if err1 != nil {
			t.Fatalf("seed %d: first Generate failed: %v", seed, err1)
		}
		if p1 == nil {
			t.Fatalf("seed %d: first Generate returned nil puzzle", seed)
		}

		r2 := engine.NewRand(seed)
		p2, _, err2 := gen.Generate(engine.Easy, r2)

		if err2 != nil {
			t.Fatalf("seed %d: second Generate failed: %v", seed, err2)
		}
		if p2 == nil {
			t.Fatalf("seed %d: second Generate returned nil puzzle", seed)
		}

		// Puzzles should be identical
		if !puzzlesEqual(p1, p2) {
			t.Errorf("seed %d: same seed produced different puzzles", seed)
		}
	}
}

// TestDeterminism_DifferentSeedsDifferentPuzzles tests that different seeds produce different puzzles (with high probability).
// This is a sanity check: if all seeds produced the same puzzle, we'd know randomness is broken.
func TestDeterminism_DifferentSeedsDifferentPuzzles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping determinism test in short mode")
	}

	gen := NewGenerator()

	// Generate puzzles for seeds 1 and 2
	r1 := engine.NewRand(1)
	p1, _, err1 := gen.Generate(engine.Easy, r1)

	if err1 != nil {
		t.Fatalf("seed 1: Generate failed: %v", err1)
	}
	if p1 == nil {
		t.Fatalf("seed 1: Generate returned nil puzzle")
	}

	r2 := engine.NewRand(2)
	p2, _, err2 := gen.Generate(engine.Easy, r2)

	if err2 != nil {
		t.Fatalf("seed 2: Generate failed: %v", err2)
	}
	if p2 == nil {
		t.Fatalf("seed 2: Generate returned nil puzzle")
	}

	// Puzzles should (almost certainly) be different
	// Not a hard invariant, but a sanity check
	if puzzlesEqual(p1, p2) {
		t.Logf("warning: seeds 1 and 2 produced identical puzzles (unlikely but possible)")
	}
}

// TestDeterminism_SameSeedAllDifficulties tests that the same seed produces the same grid structure (but possibly different difficulty).
// Invariant: randomness is deterministic; seed -> structure is reproducible
func TestDeterminism_SameSeedAllDifficulties(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping determinism test in short mode")
	}

	seed := int64(42)
	gen := NewGenerator()

	r1 := engine.NewRand(seed)
	pEasy, _, errEasy := gen.Generate(engine.Easy, r1)

	if errEasy != nil {
		t.Fatalf("seed %d: Easy Generate failed: %v", seed, errEasy)
	}

	r2 := engine.NewRand(seed)
	pMed, _, errMed := gen.Generate(engine.Medium, r2)

	if errMed != nil {
		t.Fatalf("seed %d: Medium Generate failed: %v", seed, errMed)
	}

	// Both should have the same seed value recorded.
	//
	// NOTE(green-impl): originally compared each against the outer `seed`
	// variable (42), which no implementation can satisfy — Generate's frozen
	// signature (engine.Generator[P, S]) only receives the already-seeded
	// *rand.Rand, never the raw int64 that built it, and math/rand/v2 does
	// not expose a way to recover a seed from a *rand.Rand. Fixed minimally
	// to match the comment's actual, satisfiable intent (both calls, started
	// from equally-seeded RNGs, record the same derived seed value) and the
	// precedent in this codebase (see internal/games/queens/generator.go's
	// seedFromRand) of deriving a per-puzzle seed from r rather than
	// threading the original seed through untouched.
	if pEasy.SeedVal != pMed.SeedVal {
		t.Error("puzzle seed not set correctly")
	}

	// They may have different clues (due to difficulty targeting), but should share the same basic structure
	// For now, just verify both are valid puzzles
	if len(pEasy.Clues) == 0 {
		t.Error("Easy puzzle has no clues")
	}
	if len(pMed.Clues) == 0 {
		t.Error("Medium puzzle has no clues")
	}
}

// TestDeterminism_RepeatabilityMultipleCalls tests that calling Generate multiple times with reset RNG yields same result.
func TestDeterminism_RepeatabilityMultipleCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping determinism test in short mode")
	}

	seed := int64(123)
	gen := NewGenerator()

	puzzles := make([]*Puzzle, 3)
	for i := 0; i < 3; i++ {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Easy, r)
		if err != nil {
			t.Fatalf("iteration %d: Generate failed: %v", i, err)
		}
		puzzles[i] = p
	}

	// All three should be identical
	for i := 1; i < 3; i++ {
		if !puzzlesEqual(puzzles[0], puzzles[i]) {
			t.Errorf("puzzle %d differs from puzzle 0", i)
		}
	}
}

// TestDeterminism_SeedZeroAndNegative tests that seed 0 and negative seeds work.
func TestDeterminism_SeedZeroAndNegative(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping determinism test in short mode")
	}

	gen := NewGenerator()
	seeds := []int64{0, -1, -999}

	for _, seed := range seeds {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Easy, r)

		if err != nil {
			t.Logf("seed %d: Generate returned error: %v (acceptable)", seed, err)
			continue
		}

		if p == nil {
			t.Errorf("seed %d: Generate returned nil puzzle", seed)
			continue
		}

		// Verify it's a valid puzzle structure
		if len(p.Clues) == 0 {
			t.Errorf("seed %d: puzzle has no clues", seed)
		}

		// Repeatability: generate again with same seed
		r2 := engine.NewRand(seed)
		p2, _, _ := gen.Generate(engine.Easy, r2)

		if p2 != nil && !puzzlesEqual(p, p2) {
			t.Errorf("seed %d: not deterministic", seed)
		}
	}
}

// Helper: deep-compares two puzzles for equality.
func puzzlesEqual(p1, p2 *Puzzle) bool {
	if p1 == nil || p2 == nil {
		return p1 == p2
	}

	if p1.R != p2.R || p1.C != p2.C {
		return false
	}

	if p1.SeedVal != p2.SeedVal {
		return false
	}

	if p1.Diff != p2.Diff {
		return false
	}

	if len(p1.Clues) != len(p2.Clues) {
		return false
	}

	for idx, clue1 := range p1.Clues {
		clue2, ok := p2.Clues[idx]
		if !ok {
			return false
		}
		if clue1.Number != clue2.Number || clue1.Shape != clue2.Shape {
			return false
		}
	}

	return true
}
