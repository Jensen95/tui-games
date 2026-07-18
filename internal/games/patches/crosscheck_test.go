package patches

import (
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// allDifficulties is the set of tiers the generator supports; the cross-check
// exercises every one so a bug confined to one Free-probability band still
// surfaces.
var allDifficulties = []engine.Difficulty{engine.Easy, engine.Medium, engine.Hard, engine.Expert}

// TestCrossCheck_SecondSolverAgreesOnGenerated is the core independence check.
// Over LIG_SEEDS seeds and every difficulty, the independent cell-first solver
// (crosscheck.go) must agree with the primary solver (solver.go) and the
// generator on: solution count == 1, and the identical unique tiling. If the
// primary solver and generator shared a bug (e.g. both mis-enumerating a
// shape), this second, structurally-different solver would disagree.
func TestCrossCheck_SecondSolverAgreesOnGenerated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cross-check property test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()

	for _, diff := range allDifficulties {
		for seed := int64(1); seed <= int64(seedCount); seed++ {
			r := engine.NewRand(seed)
			p, sol, err := gen.Generate(diff, r)
			if err != nil {
				t.Fatalf("diff %v seed %d: Generate failed: %v", diff, seed, err)
			}

			// Independent solver: exactly one solution.
			brute := bruteSolutions(p, 2)
			if len(brute) != 1 {
				t.Fatalf("diff %v seed %d: second solver found %d solutions, want 1", diff, seed, len(brute))
			}

			// Primary solver agrees on count.
			s := NewSolver(p)
			if got := s.CountSolutions(p, 2); got != 1 {
				t.Fatalf("diff %v seed %d: primary CountSolutions=%d, want 1", diff, seed, got)
			}

			// The generator's recorded solution == the independent solution.
			if !sameTiling(p, sol.Rects, brute[0]) {
				t.Fatalf("diff %v seed %d: recorded solution disagrees with second solver", diff, seed)
			}

			// The primary solver's solution == the independent solution.
			primarySol, ok := s.Solve(p)
			if !ok {
				t.Fatalf("diff %v seed %d: primary Solve found nothing", diff, seed)
			}
			if !sameTiling(p, primarySol.Rects, brute[0]) {
				t.Fatalf("diff %v seed %d: primary solver disagrees with second solver", diff, seed)
			}
		}
	}
}

// TestCrossCheck_LogicSolveMatchesUnique verifies that whenever LogicSolve
// closes the board its answer is exactly the unique solution found by the
// independent complete solver — a no-guess deduction must never land on a
// different tiling than the exhaustive search.
func TestCrossCheck_LogicSolveMatchesUnique(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cross-check property test in short mode")
	}

	seedCount := getSeedCount()
	gen := NewGenerator()

	for _, diff := range allDifficulties {
		for seed := int64(1); seed <= int64(seedCount); seed++ {
			r := engine.NewRand(seed)
			p, _, err := gen.Generate(diff, r)
			if err != nil {
				t.Fatalf("diff %v seed %d: Generate failed: %v", diff, seed, err)
			}

			brute := bruteSolutions(p, 2)
			if len(brute) != 1 {
				t.Fatalf("diff %v seed %d: second solver found %d solutions, want 1", diff, seed, len(brute))
			}

			s := NewSolver(p)
			logicSol, closed, _ := s.LogicSolve(p)
			if !closed {
				t.Fatalf("diff %v seed %d: LogicSolve did not close a generated puzzle", diff, seed)
			}
			if !sameTiling(p, logicSol.Rects, brute[0]) {
				t.Fatalf("diff %v seed %d: LogicSolve tiling differs from unique solution", diff, seed)
			}
		}
	}
}

// TestCrossCheck_AmbiguousFixtureAgrees confirms both solvers agree on a
// deliberately non-unique puzzle: the classic 2×2 Shikaku pair (Free·2 clues
// at opposite corners) tiles two ways (two horizontal dominoes or two vertical
// dominoes). Both solvers must report exactly 2.
func TestCrossCheck_AmbiguousFixtureAgrees(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 2, Shape: Free},
			3: {Number: 2, Shape: Free},
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	if got := bruteCount(p, 3); got != 2 {
		t.Errorf("second solver: ambiguous puzzle count = %d, want 2", got)
	}
	if got := NewSolver(p).CountSolutions(p, 3); got != 2 {
		t.Errorf("primary solver: ambiguous puzzle count = %d, want 2", got)
	}

	// The two solvers must return the same set of two tilings.
	bs := bruteSolutions(p, 3)
	if len(bs) != 2 {
		t.Fatalf("expected 2 brute solutions, got %d", len(bs))
	}
	// The two brute tilings must themselves be distinct partitions.
	if sameTiling(p, bs[0], bs[1]) {
		t.Error("the two ambiguous solutions should be distinct partitions")
	}
}

// TestCrossCheck_NearMissUniqueFreeClues is the should-NOT-trigger companion:
// a puzzle full of Free clues that nonetheless has a single solution (spec
// Gotcha: "Free clues are the main source of multiple solutions" — but not
// every Free puzzle is ambiguous). Both solvers must agree it is unique. This
// 1×4 strip with a single Free·4 clue can only be one 4×1 rectangle.
func TestCrossCheck_NearMissUniqueFreeClues(t *testing.T) {
	p := &Puzzle{
		R: 1,
		C: 4,
		Clues: map[int]Clue{
			0: {Number: 4, Shape: Free},
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}
	if got := bruteCount(p, 2); got != 1 {
		t.Errorf("second solver: unique-Free puzzle count = %d, want 1", got)
	}
	if got := NewSolver(p).CountSolutions(p, 2); got != 1 {
		t.Errorf("primary solver: unique-Free puzzle count = %d, want 1", got)
	}
}

// TestCrossCheck_ImpossibleAndOverconstrained checks both solvers agree that
// unsatisfiable puzzles have zero solutions (both the no-gap-possible case and
// the over-area case).
func TestCrossCheck_ImpossibleAndOverconstrained(t *testing.T) {
	cases := []struct {
		name string
		p    *Puzzle
	}{
		{
			name: "gap-forced", // 1x3 with two area-1 clues: middle cell can never be covered
			p: &Puzzle{R: 1, C: 3, Clues: map[int]Clue{
				0: {Number: 1, Shape: Free},
				2: {Number: 1, Shape: Free},
			}},
		},
		{
			name: "over-area", // clue numbers sum to 6 > 4 cells
			p: &Puzzle{R: 2, C: 2, Clues: map[int]Clue{
				0: {Number: 3, Shape: Free},
				2: {Number: 3, Shape: Free},
			}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bruteCount(tc.p, 2); got != 0 {
				t.Errorf("second solver count = %d, want 0", got)
			}
			if got := NewSolver(tc.p).CountSolutions(tc.p, 2); got != 0 {
				t.Errorf("primary solver count = %d, want 0", got)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Validator ↔ independent brute-force checker (cross-validation layer 3).
// -----------------------------------------------------------------------------

// independentValid re-implements the spec's Valid(rects) predicate (spec
// "Solved-state definition") from scratch on a Board's cell labels, sharing no
// code with validator.go. Returns whether the board is a complete valid tiling.
func independentValid(b *Board) bool {
	p := b.P
	if p == nil || len(b.Cells) != p.R*p.C {
		return false
	}
	// Every cell covered.
	for _, l := range b.Cells {
		if l < 0 {
			return false
		}
	}
	// Group cells by label; each label must form a solid rectangle with exactly
	// one clue whose number and shape match.
	type box struct{ minR, maxR, minC, maxC, count int }
	boxes := map[int]*box{}
	for i, l := range b.Cells {
		r, c := i/p.C, i%p.C
		bx, ok := boxes[l]
		if !ok {
			boxes[l] = &box{minR: r, maxR: r, minC: c, maxC: c, count: 1}
			continue
		}
		if r < bx.minR {
			bx.minR = r
		}
		if r > bx.maxR {
			bx.maxR = r
		}
		if c < bx.minC {
			bx.minC = c
		}
		if c > bx.maxC {
			bx.maxC = c
		}
		bx.count++
	}
	for _, bx := range boxes {
		w := bx.maxC - bx.minC + 1
		h := bx.maxR - bx.minR + 1
		if bx.count != w*h { // ragged => overlap/fragmentation
			return false
		}
		clues := 0
		var cl Clue
		for r := bx.minR; r <= bx.maxR; r++ {
			for c := bx.minC; c <= bx.maxC; c++ {
				if got, ok := p.Clues[r*p.C+c]; ok {
					clues++
					cl = got
				}
			}
		}
		if clues != 1 {
			return false
		}
		if w*h != cl.Number {
			return false
		}
		if !crossShapeOK(cl.Shape, w, h) {
			return false
		}
	}
	return true
}

// TestCrossCheck_ValidatorAgreesWithBruteForce exhaustively enumerates every
// possible cell-to-label assignment on tiny grids and asserts Validator.Solved
// agrees, cell for cell, with the independent predicate. This is the layer-3
// fuzz: two independent implementations of "is this board solved" must never
// disagree.
func TestCrossCheck_ValidatorAgreesWithBruteForce(t *testing.T) {
	puzzles := []*Puzzle{
		{R: 2, C: 2, Clues: map[int]Clue{
			0: {Number: 2, Shape: Wide},
			2: {Number: 2, Shape: Wide},
		}},
		{R: 2, C: 2, Clues: map[int]Clue{
			0: {Number: 2, Shape: Free},
			3: {Number: 2, Shape: Free},
		}},
		{R: 2, C: 3, Clues: map[int]Clue{
			0: {Number: 3, Shape: Wide},
			3: {Number: 3, Shape: Wide},
		}},
		{R: 2, C: 2, Clues: map[int]Clue{
			0: {Number: 4, Shape: Square},
		}},
	}

	for pi, p := range puzzles {
		v := NewValidator(p)
		n := p.R * p.C
		// Labels range over 0..n-1 (a distinct label per cell is enough to
		// express any partition into up to n rectangles). Enumerate n^n
		// labelings — tiny for n<=6.
		total := 1
		for i := 0; i < n; i++ {
			total *= n
		}
		for code := 0; code < total; code++ {
			cells := make([]int, n)
			x := code
			for i := 0; i < n; i++ {
				cells[i] = x % n
				x /= n
			}
			b := &Board{P: p, Cells: cells}
			if got, want := v.Solved(b), independentValid(b); got != want {
				t.Fatalf("puzzle %d labeling %v: Validator.Solved=%v, independent=%v", pi, cells, got, want)
			}
		}
	}
}

// -----------------------------------------------------------------------------
// Dedup: geometric dihedral invariance including Wide↔Tall (cross-val item C).
// -----------------------------------------------------------------------------

// geoTransformPuzzle applies a genuine geometric dihedral transform: it moves
// clue anchors AND swaps Wide↔Tall under the four dimension-swapping
// transforms. Under such a rotation the actual solution rectangles swap width
// and height, so a Wide rectangle becomes Tall; a fingerprint that ignored
// this would fail to recognize a puzzle and its own rotation as the same.
func geoTransformPuzzle(p *Puzzle, t engine.Transform) *Puzzle {
	newR, newC := t.Dims(p.R, p.C)
	swap := t.SwapsDims()
	nc := make(map[int]Clue, len(p.Clues))
	for idx, clue := range p.Clues {
		dst := t.Apply(engine.CellAt(idx, p.C), p.R, p.C)
		if swap {
			switch clue.Shape {
			case Wide:
				clue.Shape = Tall
			case Tall:
				clue.Shape = Wide
			}
		}
		nc[engine.Index(dst, newC)] = clue
	}
	return &Puzzle{R: newR, C: newC, Clues: nc, SeedVal: p.SeedVal, Diff: p.Diff}
}

// TestCrossCheck_FingerprintGeometricDihedralInvariance asserts a puzzle and
// each of its eight TRUE geometric transforms share a fingerprint — including
// the dim-swapping rotations that exchange Wide and Tall. This is the dedup
// invariant the cross-validation matrix names for Patches ("shape types
// transform correctly under rotation, Wide↔Tall"). A fingerprint that moved
// clue positions but left shapes untouched would pass the weaker
// position-only test yet fail here, so this guards against exactly that gap.
func TestCrossCheck_FingerprintGeometricDihedralInvariance(t *testing.T) {
	seedCount := getSeedCount()
	gen := NewGenerator()
	fp := NewFingerprinter()

	for _, diff := range allDifficulties {
		for seed := int64(1); seed <= int64(seedCount); seed++ {
			r := engine.NewRand(seed)
			p, _, err := gen.Generate(diff, r)
			if err != nil {
				t.Fatalf("diff %v seed %d: Generate failed: %v", diff, seed, err)
			}
			base := fp.Fingerprint(p)
			for _, tr := range engine.AllTransforms {
				g := geoTransformPuzzle(p, tr)
				if fp.Fingerprint(g) != base {
					t.Fatalf("diff %v seed %d: geometric transform %v changed fingerprint", diff, seed, tr)
				}
			}
		}
	}
}

// TestCrossCheck_FingerprintDistinguishesWideFromTall guards the other
// direction: two puzzles that differ ONLY in a Wide vs Tall clue (and are not
// related by any dihedral symmetry) must get different fingerprints. On a
// non-square grid a lone Wide clue and a lone Tall clue describe genuinely
// different puzzles and must not be collapsed.
func TestCrossCheck_FingerprintDistinguishesWideFromTall(t *testing.T) {
	// 2x3 grid, single clue of area 6 in the corner: Wide (must be 3x2/6x1)
	// vs Tall (must be 2x3/1x6). These are not dihedral images of each other
	// because both live on the same 2x3 frame with the clue in the same corner.
	wide := &Puzzle{R: 2, C: 3, Clues: map[int]Clue{0: {Number: 6, Shape: Wide}}}
	tall := &Puzzle{R: 2, C: 3, Clues: map[int]Clue{0: {Number: 6, Shape: Tall}}}
	fp := NewFingerprinter()
	if fp.Fingerprint(wide) == fp.Fingerprint(tall) {
		t.Error("Wide and Tall single-clue puzzles must have distinct fingerprints")
	}
}

// TestCrossCheck_FingerprintDistinguishesDifferentPuzzles is a basic
// distinctness sanity check independent of the generator: two hand-built
// puzzles with different clue layouts must not collide.
func TestCrossCheck_FingerprintDistinguishesDifferentPuzzles(t *testing.T) {
	// a: the whole 2x2 grid as one Square·4 rectangle (a single clue).
	// b: the same grid split into two Wide·2 dominoes (two clues).
	// Different clue counts make these non-symmetric under any transform.
	a := &Puzzle{R: 2, C: 2, Clues: map[int]Clue{
		0: {Number: 4, Shape: Square},
	}}
	b := &Puzzle{R: 2, C: 2, Clues: map[int]Clue{
		0: {Number: 2, Shape: Wide},
		2: {Number: 2, Shape: Wide},
	}}
	fp := NewFingerprinter()
	if fp.Fingerprint(a) == fp.Fingerprint(b) {
		t.Error("structurally different puzzles must have distinct fingerprints")
	}
}
