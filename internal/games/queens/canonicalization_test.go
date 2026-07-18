package queens

import (
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// TestFingerprint_AllTransforms_ShareFingerprint pins: "Region relabeling /
// rotation of a puzzle yields the same fingerprint (canonicalization test)."
// Applying every transform in the 8-way dihedral group (with region labels
// re-normalized by first appearance, since colors are non-semantic) must
// yield puzzles that all fingerprint identically.
func TestFingerprint_AllTransforms_ShareFingerprint(t *testing.T) {
	fp := NewFingerprinter()
	base := goldenUniquePuzzle6()
	want := fp.Fingerprint(base)

	for _, tr := range engine.AllTransforms {
		transformed := transformPuzzle(base, tr)
		got := fp.Fingerprint(transformed)
		if got != want {
			t.Errorf("Fingerprint(transform=%v) = %x, want %x (same as untransformed)", tr, got, want)
		}
	}
}

// TestFingerprint_AllTransforms_CanonicalBytesIdentical is the byte-level
// version of the same invariant: Canonical() output, not just its hash,
// must be identical across every transform.
func TestFingerprint_AllTransforms_CanonicalBytesIdentical(t *testing.T) {
	fp := NewFingerprinter()
	base := goldenUniquePuzzle6()
	want := fp.Canonical(base)

	for _, tr := range engine.AllTransforms {
		transformed := transformPuzzle(base, tr)
		got := fp.Canonical(transformed)
		if string(got) != string(want) {
			t.Errorf("Canonical(transform=%v) = %x, want %x (same as untransformed)", tr, got, want)
		}
	}
}

// TestFingerprint_DistinctPuzzles_DistinctFingerprints pins: "Fingerprints
// pairwise distinct across a large batch." Two genuinely different
// hand-built puzzles (different N, different regions, different solutions)
// must never collide.
func TestFingerprint_DistinctPuzzles_DistinctFingerprints(t *testing.T) {
	fp := NewFingerprinter()
	a := fp.Fingerprint(goldenUniquePuzzle6())
	b := fp.Fingerprint(ambiguousPuzzle5())

	if a == b {
		t.Errorf("Fingerprint(golden N=6) == Fingerprint(ambiguous N=5) = %x, want distinct fingerprints for distinct puzzles", a)
	}
}

// TestFingerprint_GeneratedBatch_PairwiseDistinct exercises the generator +
// fingerprinter together: a batch of freshly generated puzzles (distinct
// seeds) must never collide. Seed count honors LIG_SEEDS (default 250).
func TestFingerprint_GeneratedBatch_PairwiseDistinct(t *testing.T) {
	n := seedCount()
	if n > 50 {
		// Fingerprint collision-checking is O(n); 50 distinct generations is
		// already strong evidence and keeps this test fast even when
		// LIG_SEEDS is cranked up for other (per-seed) property tests.
		n = 50
	}

	gen := NewGenerator()
	fp := NewFingerprinter()
	seen := make(map[[32]byte]int64, n)

	for i := 1; i <= n; i++ {
		seed := int64(i)
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Easy, r)
		if err != nil {
			t.Fatalf("Generate(seed=%d) returned error: %v", seed, err)
		}
		f := fp.Fingerprint(p)
		if prev, dup := seen[f]; dup {
			t.Errorf("Fingerprint collision between seed=%d and seed=%d: %x", prev, seed, f)
		}
		seen[f] = seed
	}
}
