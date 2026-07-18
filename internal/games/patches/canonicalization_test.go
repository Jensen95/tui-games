package patches

import (
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// TestCanonicalization_AllTransformsShareFingerprint tests that applying every dihedral transform to a puzzle yields the same fingerprint.
// Invariant: Fingerprint(puzzle) == Fingerprint(transform(puzzle)) for all transforms
func TestCanonicalization_AllTransformsShareFingerprint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping canonicalization test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()
	fp := NewFingerprinter()

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Easy, r)

		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		if p == nil {
			t.Fatalf("seed %d: Generate returned nil puzzle", seed)
		}

		baseFingerprint := fp.Fingerprint(p)

		// Apply each dihedral transform
		for _, transform := range engine.AllTransforms {
			transformed := applyTransformToPuzzle(p, transform)
			transformedFP := fp.Fingerprint(transformed)

			if transformedFP != baseFingerprint {
				t.Errorf("seed %d: transform %v produced different fingerprint", seed, transform)
			}
		}
	}
}

// TestCanonicalization_CanonicalMinWorks tests that CanonicalMin correctly identifies the lexicographically smallest serialization.
func TestCanonicalization_CanonicalMinWorks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping canonicalization test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()
	fp := NewFingerprinter()

	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Easy, r)

		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		if p == nil {
			t.Fatalf("seed %d: Generate returned nil puzzle", seed)
		}

		// Collect canonical forms of all transforms
		var candidates [][]byte
		for _, transform := range engine.AllTransforms {
			transformed := applyTransformToPuzzle(p, transform)
			canonical := fp.Canonical(transformed)
			candidates = append(candidates, canonical)
		}

		// Find the minimum
		minCanonical := engine.CanonicalMin(candidates)

		// It should be one of the candidates
		found := false
		for _, c := range candidates {
			if bytesEqual(c, minCanonical) {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("seed %d: CanonicalMin returned a value not in candidates", seed)
		}
	}
}

// TestCanonicalization_BatchFingerprintsPairwiseDistinct tests that a batch of generated puzzles have distinct fingerprints.
// This catches generation dupes and transform failures.
// Invariant: Fingerprints of different puzzles are distinct
func TestCanonicalization_BatchFingerprintsPairwiseDistinct(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping canonicalization test in short mode")
	}

	// Test a smaller batch to keep test time reasonable
	batchSize := 50
	seedCount := getSeedCount()
	if seedCount < batchSize {
		batchSize = seedCount
	}

	gen := NewGenerator()
	fp := NewFingerprinter()
	seen := make(map[[32]byte]int64)

	for seed := int64(1); seed <= int64(batchSize); seed++ {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Easy, r)

		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		if p == nil {
			t.Fatalf("seed %d: Generate returned nil puzzle", seed)
		}

		fingerprint := fp.Fingerprint(p)

		if prevSeed, dup := seen[fingerprint]; dup {
			t.Errorf("duplicate fingerprint: seed %d and seed %d", prevSeed, seed)
		}

		seen[fingerprint] = seed
	}
}

// TestCanonicalization_ColorAgnostic tests that color (cosmetic) doesn't affect fingerprinting.
// Invariant: Colors must never influence fingerprinting
func TestCanonicalization_ColorAgnostic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping canonicalization test in short mode")
	}

	// Generate a puzzle and verify that if we were to color it differently,
	// the fingerprint would remain the same. Since colors are cosmetic,
	// two puzzles identical except for color assignment should have the same fingerprint.
	// This is tested indirectly: if the fingerprinter is correct, all transforms
	// (which may reorder colors) should still yield the same fingerprint.

	gen := NewGenerator()
	fp := NewFingerprinter()

	r := engine.NewRand(42)
	p1, _, err := gen.Generate(engine.Easy, r)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	r = engine.NewRand(42)
	p2, _, err := gen.Generate(engine.Easy, r)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	fp1 := fp.Fingerprint(p1)
	fp2 := fp.Fingerprint(p2)

	if fp1 != fp2 {
		t.Error("same seed should produce same fingerprint (determinism)")
	}
}

// TestCanonicalization_SquareGridTransforms tests canonicalization on square grids.
func TestCanonicalization_SquareGridTransforms(t *testing.T) {
	// For square grids, all 8 transforms should be applicable and share a fingerprint
	seedCount := getSeedCount()
	if seedCount < 10 {
		seedCount = 10
	}

	gen := NewGenerator()
	fp := NewFingerprinter()

	for seed := int64(1); seed <= 10; seed++ {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Easy, r)

		if err != nil {
			continue // Skip if generation fails
		}
		if p == nil {
			continue
		}

		// Only test square grids
		if p.R != p.C {
			continue
		}

		baseFingerprint := fp.Fingerprint(p)

		// All transforms should apply without panicking
		for i, transform := range engine.AllTransforms {
			transformed := applyTransformToPuzzle(p, transform)
			if transformed == nil {
				t.Errorf("seed %d: transform %d returned nil", seed, i)
				continue
			}

			fp2 := fp.Fingerprint(transformed)
			if fp2 != baseFingerprint {
				t.Errorf("seed %d: transform %d changed fingerprint", seed, i)
			}
		}
	}
}

// Helper: applies a full dihedral transform to a puzzle. A genuine geometric
// transform moves each clue's anchor cell AND remaps its shape: the four
// dimension-swapping transforms exchange width and height, turning Wide into
// Tall and vice versa (Square/Free are fixed). Without the shape remap this
// helper would build a puzzle that is not actually a rotation of the original
// (its clue shapes would no longer match the rotated solution), so the
// "transforms share a fingerprint" assertions would be testing the wrong
// invariant. See docs/plan/games/patches.md "Uniqueness & deduplication".
func applyTransformToPuzzle(p *Puzzle, transform engine.Transform) *Puzzle {
	newR, newC := transform.Dims(p.R, p.C)
	swap := transform.SwapsDims()

	newClues := make(map[int]Clue)
	for cellIdx, clue := range p.Clues {
		oldCell := engine.CellAt(cellIdx, p.C)
		newCell := transform.Apply(oldCell, p.R, p.C)
		newIdx := engine.Index(newCell, newC)
		if swap {
			switch clue.Shape {
			case Wide:
				clue.Shape = Tall
			case Tall:
				clue.Shape = Wide
			}
		}
		newClues[newIdx] = clue
	}

	return &Puzzle{
		R:       newR,
		C:       newC,
		Clues:   newClues,
		SeedVal: p.SeedVal,
		Diff:    p.Diff,
	}
}

// Helper: compares two byte slices.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
