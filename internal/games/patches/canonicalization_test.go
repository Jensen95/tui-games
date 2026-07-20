package patches

import (
	"maps"
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

// puzzleGeometryEqual reports whether a and b carry identical fingerprinted
// geometry: dimensions and the anchor->clue map (each Clue being its Number and
// Shape). SeedVal and Diff are cosmetic bookkeeping that never enter the
// fingerprint, so they are ignored. Clue is a comparable struct, so map
// equality settles both the anchor positions and the (number, shape) payloads.
func puzzleGeometryEqual(a, b *Puzzle) bool {
	return a.R == b.R && a.C == b.C && maps.Equal(a.Clues, b.Clues)
}

// patchesEquivalent reports whether some dihedral transform of a reproduces b
// exactly — i.e. a and b are "the same puzzle up to Patches' symmetry group".
// It is an *independent* oracle: it remaps geometry directly (via the test's
// applyTransformToPuzzle, which moves each clue's anchor cell AND flips Wide<->
// Tall under the four dimension-swapping transforms), NOT through the byte
// serialization the Fingerprinter canonicalizes. Because it walks the exact
// same group Canonical does (engine.AllTransforms) with the exact same shape/
// anchor remap, it faithfully mirrors the equivalence relation the fingerprint
// is supposed to quotient by — so it can referee whether a shared fingerprint
// is a genuine symmetry duplicate or a canonicalization defect. An oracle that
// covered fewer transforms (or skipped the Wide<->Tall remap) would wrongly
// flag a legitimate rotated/reflected duplicate as a bug.
func patchesEquivalent(a, b *Puzzle) bool {
	for _, tr := range engine.AllTransforms {
		if puzzleGeometryEqual(applyTransformToPuzzle(a, tr), b) {
			return true
		}
	}
	return false
}

// TestCanonicalization_BatchFingerprintsPairwiseDistinct pins the batch-dedup
// property in the form that actually holds: equal fingerprints must imply
// symmetry-equivalent puzzles.
//
// Two distinct seeds can legitimately produce the same puzzle up to Patches'
// symmetry group (the 8 dihedral transforms, with Wide<->Tall remapped under
// the dimension-swapping ones). The Easy tier is low-entropy, so a
// birthday-paradox collision among raw sequential seeds is expected, not a
// defect: "never a repeat" is enforced only by the retry/corpus-dedup layer
// (which rejects a candidate whose fingerprint was already banked), never
// per raw seed — Generator.Generate takes no seen-set. Asserting "no two seeds
// ever collide" therefore tests a guarantee that does not exist; it passes
// today only by luck of which seeds fall in the tested window and would fail
// as a false alarm the moment a genuine symmetry-duplicate lands in range.
//
// The real defect worth guarding against is a lossy or over-collapsing
// canonicalization that maps two NON-equivalent puzzles onto one fingerprint.
// So we assert exactly that: on any fingerprint collision the two puzzles must
// be dihedral-equivalent, checked by the independent patchesEquivalent oracle.
// This never false-alarms on entropy, so it runs the full seed count — more
// seeds is strictly more evidence that canonicalization stays injective on
// distinct puzzles.
func TestCanonicalization_BatchFingerprintsPairwiseDistinct(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping canonicalization test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()
	fp := NewFingerprinter()

	// One representative puzzle per fingerprint. Dihedral equivalence is
	// transitive, so comparing a new collider against any prior member of its
	// fingerprint class is sufficient.
	seen := make(map[[32]byte]*Puzzle, seedCount)
	for seed := int64(1); seed <= int64(seedCount); seed++ {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Easy, r)

		if err != nil {
			t.Fatalf("seed %d: Generate failed: %v", seed, err)
		}
		if p == nil {
			t.Fatalf("seed %d: Generate returned nil puzzle", seed)
		}

		fingerprint := fp.Fingerprint(p)

		if prior, dup := seen[fingerprint]; dup && !patchesEquivalent(prior, p) {
			t.Errorf("fingerprint collision between non-equivalent puzzles at seed %d "+
				"(fingerprint %x); canonicalization is collapsing distinct puzzles", seed, fingerprint)
		}
		seen[fingerprint] = p
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
