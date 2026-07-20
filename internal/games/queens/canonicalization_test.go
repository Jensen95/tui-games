package queens

import (
	"slices"
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

// puzzleGeometryEqual reports whether a and b carry identical fingerprinted
// geometry: board size, region grid, and given cells. SeedV and DiffV are
// cosmetic (they never enter serialize/Canonical — see fingerprint.go), so
// they are ignored. Callers are responsible for having normalized both region
// grids into the same color-agnostic frame before comparing (region ids are
// non-semantic).
func puzzleGeometryEqual(a, b Puzzle) bool {
	return a.N == b.N &&
		slices.Equal(a.Region, b.Region) &&
		slices.Equal(a.Givens, b.Givens)
}

// queensEquivalent reports whether a and b are "the same puzzle up to
// symmetry" under Queens' full symmetry group: the 8 dihedral transforms of
// the board AND the region relabeling that Canonical normalizes away (region
// colors are non-semantic, so any bijective renaming of region ids yields the
// same puzzle).
//
// It is an *independent* oracle. It remaps geometry directly via the test's
// transformPuzzle — which applies a dihedral transform to the region grid and
// givens and then re-labels regions by first appearance
// (engine.RelabelFirstAppearance) — and never calls Canonical/Fingerprint. That
// independence is the whole point: a shared fingerprint may only be trusted as
// a "genuine duplicate" if an oracle that does NOT use the fingerprinter agrees
// the two puzzles really are symmetry-equivalent. First-appearance relabeling
// is a complete normalizer for region renaming (it depends only on the region
// *partition* and row-major scan order, not on the id values), so it captures
// exactly the relabeling half of the group; the transform loop captures the
// dihedral half.
//
// b is put in the same frame with transformPuzzle(b, engine.Identity), which
// applies first-appearance relabeling without moving any cell. Because the
// dihedral transforms form a group, "some transform of a normalizes to
// normalized-b" is equivalent to "the orbits of a and b intersect", which is
// exactly the condition under which their canonical (orbit-lexmin)
// fingerprints coincide.
func queensEquivalent(a, b Puzzle) bool {
	bNorm := transformPuzzle(b, engine.Identity)
	for _, tr := range engine.AllTransforms {
		if puzzleGeometryEqual(transformPuzzle(a, tr), bNorm) {
			return true
		}
	}
	return false
}

// TestFingerprint_GeneratedBatch_PairwiseDistinct pins the spec property
// "fingerprints pairwise distinct across a large batch" in the form that
// actually holds: equal fingerprints must imply symmetry-equivalent puzzles.
//
// Two different seeds may legitimately produce the same puzzle up to Queens'
// symmetry group (dihedral transforms + region relabeling). "Never a repeat"
// is enforced only at the retry/corpus-dedup layer (cmd/lig generate +
// internal/corpus) — Generator.Generate takes no seen-set and makes no such
// promise per raw seed. So asserting "no two seeds ever collide" tests a
// guarantee that does not exist: it passed before only because the seed window
// was artificially capped at 50, and would false-alarm the moment a genuine
// symmetry-duplicate landed in the tested range.
//
// The real defect worth guarding against is a lossy or over-collapsing
// canonicalization that maps two DISTINCT puzzles onto one fingerprint. So on
// any fingerprint collision we fail ONLY if the colliding puzzles are not
// queensEquivalent (checked by the independent oracle above). This never
// false-alarms on entropy, so we drop the 50-seed cap and run the full
// seedCount()/LIG_SEEDS batch — more seeds is strictly more evidence that
// canonicalization stays injective on genuinely distinct puzzles (and more
// chances to exercise the equivalence branch).
func TestFingerprint_GeneratedBatch_PairwiseDistinct(t *testing.T) {
	n := seedCount()

	gen := NewGenerator()
	fp := NewFingerprinter()

	// One representative puzzle per fingerprint. Symmetry-equivalence is an
	// equivalence relation (so, within a fingerprint class, transitive), hence
	// comparing a new collider against any prior member of its class suffices.
	seen := make(map[[32]byte]Puzzle, n)
	collisions := 0

	for i := 1; i <= n; i++ {
		seed := int64(i)
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(engine.Easy, r)
		if err != nil {
			t.Fatalf("Generate(seed=%d) returned error: %v", seed, err)
		}
		f := fp.Fingerprint(p)
		if prior, dup := seen[f]; dup {
			if !queensEquivalent(prior, p) {
				t.Errorf("fingerprint collision between non-equivalent puzzles at seed=%d "+
					"(fingerprint %x); canonicalization is collapsing distinct puzzles", seed, f)
			} else {
				collisions++
			}
		}
		seen[f] = p
	}

	t.Logf("observed %d confirmed symmetry-equivalent fingerprint collision(s) across %d seeds", collisions, n)
}
