package zip

import (
	"fmt"
	"maps"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// squareFixturePuzzle is a 3x3 grid (square, so all 8 dihedral transforms
// preserve its R,C shape -- see engine.Transform's doc comment on
// SwapsDims) with an asymmetric waypoint layout and one wall, used to
// exercise the full symmetry group without the non-square dimension-swap
// subtlety.
func squareFixturePuzzle() Puzzle {
	return Puzzle{
		R: 3, C: 3,
		Waypoint: map[int]int{0: 1, 4: 2, 8: 3},
		Walls:    map[[2]int]bool{WallKey(1, 2): true},
	}
}

// transformPuzzle applies dihedral transform tr to p's waypoint cells and
// wall edges, producing the geometrically-rotated/reflected puzzle. This is
// test fixture-construction (mirroring, at the byte level, what a real
// canonicalizer must do internally) -- not the Canonical/Fingerprint logic
// under test, which stays a todo-panic in zip.go.
func transformPuzzle(p Puzzle, tr engine.Transform) Puzzle {
	newR, newC := tr.Dims(p.R, p.C)
	remap := func(idx int) int {
		c := engine.CellAt(idx, p.C)
		nc := tr.Apply(c, p.R, p.C)
		return engine.Index(nc, newC)
	}

	newWaypoint := make(map[int]int, len(p.Waypoint))
	for idx, num := range p.Waypoint {
		newWaypoint[remap(idx)] = num
	}
	newWalls := make(map[[2]int]bool, len(p.Walls))
	for edge := range p.Walls {
		newWalls[WallKey(remap(edge[0]), remap(edge[1]))] = true
	}

	return Puzzle{
		R: newR, C: newC,
		Waypoint: newWaypoint,
		Walls:    newWalls,
		SeedVal:  p.SeedVal,
		Diff:     p.Diff,
	}
}

// TestFingerprint_DihedralTransformsShareFingerprint pins: "dihedral
// transforms of one puzzle share a fingerprint" (docs/plan/games/zip.md TDD
// matrix; symmetry group per docs/02-engine-and-generation.md is the 8
// dihedral transforms -- numbering fixes path direction, so reversal is
// deliberately excluded, see TestFingerprint_ReversalNotCollapsed).
func TestFingerprint_DihedralTransformsShareFingerprint(t *testing.T) {
	base := squareFixturePuzzle()
	fp := Fingerprinter{}
	want := mustFingerprint(t, fp, base)

	for _, tr := range engine.AllTransforms {
		tr := tr
		t.Run(fmt.Sprintf("transform=%d", tr), func(t *testing.T) {
			got := mustFingerprint(t, fp, transformPuzzle(base, tr))
			if got != want {
				t.Errorf("Fingerprint(transform %d of base) = %x, want %x (identity's fingerprint)", tr, got, want)
			}
		})
	}
}

// puzzleGeometryEqual reports whether a and b carry identical fingerprinted
// geometry: dimensions, waypoint numbering, and walls. SeedVal and Diff are
// cosmetic (never enter the fingerprint), so they are ignored.
func puzzleGeometryEqual(a, b Puzzle) bool {
	return a.R == b.R && a.C == b.C &&
		maps.Equal(a.Waypoint, b.Waypoint) &&
		maps.Equal(a.Walls, b.Walls)
}

// dihedralEquivalent reports whether some dihedral transform of a reproduces b
// exactly -- i.e. a and b are "the same puzzle up to symmetry". It is an
// *independent* oracle: it remaps geometry directly (via the test's
// transformPuzzle), not through the serialization the Fingerprinter uses, so
// it can be trusted to referee whether a shared fingerprint is a genuine
// duplicate or a canonicalization defect.
func dihedralEquivalent(a, b Puzzle) bool {
	for _, tr := range engine.AllTransforms {
		if puzzleGeometryEqual(transformPuzzle(a, tr), b) {
			return true
		}
	}
	return false
}

// TestFingerprint_BatchPairwiseDistinct pins the spec property "fingerprints
// pairwise distinct across a batch" (docs/plan/games/zip.md) in the form that
// actually holds: equal fingerprints must imply equal puzzles.
//
// Two seeds may legitimately produce the same puzzle up to dihedral symmetry.
// The Easy tier is deliberately low-entropy (5x5, ~half the cells numbered),
// so a birthday-paradox collision among raw sequential seeds is expected, not
// a defect -- "never a repeat" is enforced by the retry/corpus dedup layer
// (cmd/lig generate + internal/corpus), never promised per raw seed (see
// Generator.Generate, which takes no seen-set). Asserting "no two seeds ever
// collide" therefore tests a guarantee that does not exist: it passes today
// only by luck of which seeds fall in range, and would fail as a false alarm
// the moment a genuine duplicate lands in the tested window.
//
// The real defect worth guarding against is a lossy or over-collapsing
// canonicalization that maps two DISTINCT puzzles onto one fingerprint. So we
// assert exactly that: on any fingerprint collision the two puzzles must be
// dihedral-equivalent (checked by an independent oracle). This never
// false-alarms on entropy, so it runs the full seed count -- more seeds is
// strictly more evidence that canonicalization stays injective on distinct
// puzzles.
func TestFingerprint_BatchPairwiseDistinct(t *testing.T) {
	n := seedCount()
	gen := Generator{}
	fp := Fingerprinter{}

	// One representative puzzle per fingerprint. Dihedral equivalence is
	// transitive, so comparing a new collider against any prior member of its
	// fingerprint class is sufficient.
	seen := make(map[[32]byte]Puzzle, n)
	for seed := 1; seed <= n; seed++ {
		p, _, err := mustGenerate(t, gen, engine.Easy, engine.NewRand(int64(seed)))
		if err != nil {
			t.Fatalf("Generate(seed=%d) error: %v", seed, err)
		}
		f := mustFingerprint(t, fp, p)
		if prior, dup := seen[f]; dup && !dihedralEquivalent(prior, p) {
			t.Errorf("fingerprint collision between non-equivalent puzzles at seed %d "+
				"(fingerprint %x); canonicalization is collapsing distinct puzzles", seed, f)
		}
		seen[f] = p
	}
}

// TestFingerprint_ReversalNotCollapsed pins the spec Gotcha: "a path and its
// reversal are the same shape but numbering makes them distinct puzzles
// (reversing swaps 1<->K); treat numbered puzzles as directed, so reversal
// is a different puzzle unless you also renumber." Canonicalization must
// NOT collapse a puzzle and its direction-reversed twin onto one
// fingerprint.
//
// forward and reversed are hand-verified to not be dihedral transforms of
// each other on this 2x3 grid: the numbered cells sit at the four corners of
// a top-left 2x2 sub-square, and neither Rot180, FlipH, nor FlipV of forward
// (the only shape-preserving non-identity transforms on a non-square 2x3
// grid) reproduces reversed's cell->number assignment.
func TestFingerprint_ReversalNotCollapsed(t *testing.T) {
	forward := Puzzle{
		R: 2, C: 3,
		Waypoint: map[int]int{0: 1, 1: 2, 3: 3, 4: 4},
		Walls:    map[[2]int]bool{},
	}
	reversed := Puzzle{
		R: 2, C: 3,
		Waypoint: map[int]int{0: 4, 1: 3, 3: 2, 4: 1}, // same cells, numbering reversed (K+1-num)
		Walls:    map[[2]int]bool{},
	}

	fp := Fingerprinter{}
	a := mustFingerprint(t, fp, forward)
	b := mustFingerprint(t, fp, reversed)
	if a == b {
		t.Errorf("Fingerprint(forward) == Fingerprint(reversed) (%x); reversal must not be collapsed by canonicalization", a)
	}
}
