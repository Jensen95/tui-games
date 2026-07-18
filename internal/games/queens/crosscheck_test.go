package queens

import (
	"reflect"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// --- fixture-level agreement: golden + ambiguous boards ---

// TestCrossSolver_Agrees_GoldenUniqueBoard pins the cross-validation
// invariant on the shipped golden fixture: the independent column-major
// solver must find the same unique solution as the primary solver.
func TestCrossSolver_Agrees_GoldenUniqueBoard(t *testing.T) {
	primary := NewSolver()
	cross := NewCrossSolver()
	p := goldenUniquePuzzle6()
	want := goldenUniqueSolution6()

	pSol, pOk := primary.Solve(p)
	if !pOk {
		t.Fatalf("primary Solve() returned ok=false, want true")
	}
	if !reflect.DeepEqual(pSol, want) {
		t.Fatalf("primary Solve() = %+v, want %+v", pSol, want)
	}

	cSol, cOk := cross.Solve(p)
	if !cOk {
		t.Fatalf("CrossSolver.Solve() returned ok=false, want true")
	}
	if !reflect.DeepEqual(cSol, want) {
		t.Errorf("CrossSolver.Solve() = %+v, want %+v (golden solution)", cSol, want)
	}

	if got := cross.CountSolutions(p, 2); got != 1 {
		t.Errorf("CrossSolver.CountSolutions(golden N=6, cap=2) = %d, want 1", got)
	}
	if got, want := cross.CountSolutions(p, 2), primary.CountSolutions(p, 2); got != want {
		t.Errorf("CrossSolver.CountSolutions = %d, primary Solver.CountSolutions = %d, want agreement", got, want)
	}
}

// TestCrossSolver_Agrees_AmbiguousBoard pins agreement on the hand-built
// ambiguous fixture: both solvers must report exactly 2 solutions, and the
// solution CrossSolver finds first must be one of the two known ones (an
// independent structural search may enumerate in a different order than the
// primary solver, so we don't require the SAME first solution — only that
// each solver's result set is the known pair).
func TestCrossSolver_Agrees_AmbiguousBoard(t *testing.T) {
	primary := NewSolver()
	cross := NewCrossSolver()
	p := ambiguousPuzzle5()
	known := ambiguousSolutions5()

	pCount := primary.CountSolutions(p, 3)
	cCount := cross.CountSolutions(p, 3)
	if pCount != 2 {
		t.Fatalf("primary Solver.CountSolutions(ambiguous N=5, cap=3) = %d, want 2", pCount)
	}
	if cCount != 2 {
		t.Errorf("CrossSolver.CountSolutions(ambiguous N=5, cap=3) = %d, want 2 (agree with primary)", cCount)
	}

	cSol, ok := cross.Solve(p)
	if !ok {
		t.Fatalf("CrossSolver.Solve(ambiguous N=5) returned ok=false, want true")
	}
	matched := false
	for _, want := range known {
		if reflect.DeepEqual(cSol, want) {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("CrossSolver.Solve(ambiguous N=5) = %+v, want one of the known solutions %+v", cSol, known)
	}
}

// TestCrossSolver_Agrees_LogicSolve_GoldenBoard pins: "LogicSolve's solution
// (when it closes) equals the complete solver's unique solution" — checked
// against the INDEPENDENT solver, not just the primary complete solver (that
// self-agreement is already pinned in solver_test.go).
func TestCrossSolver_Agrees_LogicSolve_GoldenBoard(t *testing.T) {
	primary := NewSolver()
	cross := NewCrossSolver()
	p := goldenUniquePuzzle6()

	logicSol, closed, _ := primary.LogicSolve(p)
	if !closed {
		t.Fatalf("LogicSolve(golden N=6) closed=false, want true")
	}
	crossSol, ok := cross.Solve(p)
	if !ok {
		t.Fatalf("CrossSolver.Solve(golden N=6) returned ok=false, want true")
	}
	if !reflect.DeepEqual(logicSol, crossSol) {
		t.Errorf("LogicSolve() = %+v, CrossSolver.Solve() = %+v, want agreement", logicSol, crossSol)
	}
}

// --- generated-batch agreement over LIG_SEEDS-controlled seeds ---

// TestCrossSolver_Agrees_GeneratedBatch_OverSeeds is the core cross-check:
// for every generated puzzle over many seeds and sizes, the independent
// solver must (a) agree with the primary complete solver on solution count,
// (b) agree with the primary on the unique solution, and (c) agree with
// LogicSolve's solution whenever LogicSolve closes. A generator or solver
// bug shared between the two implementations would have to be identical
// down to the bit to survive this — the whole point of an independently
// structured second solver.
func TestCrossSolver_Agrees_GeneratedBatch_OverSeeds(t *testing.T) {
	diffs := []engine.Difficulty{engine.Easy, engine.Medium, engine.Hard}

	gen := NewGenerator()
	primary := NewSolver()
	cross := NewCrossSolver()

	n := seedCount()
	for i := 1; i <= n; i++ {
		seed := int64(i)
		diff := diffs[i%len(diffs)]

		p, recordedSol, err := gen.Generate(diff, engine.NewRand(seed))
		if err != nil {
			t.Fatalf("Generate(diff=%v, seed=%d) returned error: %v", diff, seed, err)
		}

		pCount := primary.CountSolutions(p, 2)
		cCount := cross.CountSolutions(p, 2)
		if pCount != 1 {
			// Not this test's concern (generation_property_test.go pins this),
			// but bail out clearly rather than compare garbage below.
			t.Fatalf("seed=%d diff=%v: primary Solver.CountSolutions(p,2) = %d, want 1", seed, diff, pCount)
		}
		if cCount != pCount {
			t.Fatalf("seed=%d diff=%v: CrossSolver.CountSolutions(p,2) = %d, primary = %d, want agreement (generator uniqueness bug?)", seed, diff, cCount, pCount)
		}

		pSol, pOk := primary.Solve(p)
		cSol, cOk := cross.Solve(p)
		if !pOk || !cOk {
			t.Fatalf("seed=%d diff=%v: Solve ok = (primary=%v, cross=%v), want both true", seed, diff, pOk, cOk)
		}
		if !reflect.DeepEqual(pSol, cSol) {
			t.Fatalf("seed=%d diff=%v: primary Solve() = %+v, CrossSolver.Solve() = %+v, want agreement on the unique solution", seed, diff, pSol, cSol)
		}
		if !reflect.DeepEqual(pSol, recordedSol) {
			t.Fatalf("seed=%d diff=%v: primary Solve() = %+v, Generate's recorded solution = %+v, want agreement", seed, diff, pSol, recordedSol)
		}

		logicSol, closed, _ := primary.LogicSolve(p)
		if !closed {
			t.Fatalf("seed=%d diff=%v: LogicSolve did not close; %v is a no-guess tier", seed, diff, diff)
		}
		if !reflect.DeepEqual(logicSol, cSol) {
			t.Errorf("seed=%d diff=%v: LogicSolve() = %+v, CrossSolver.Solve() = %+v, want agreement", seed, diff, logicSol, cSol)
		}
	}
}

// --- Gotcha audit addition: color labels are non-semantic ---

// permuteRegionLabels relabels region (a pure color permutation — no spatial
// transform at all) via perm: cell with original id r gets perm[r]. Region
// shape/connectivity/solution are unchanged; only the numeric labels move.
// This is the gotcha "Color labels are non-semantic — never let them leak
// into fingerprints or solver logic" exercised WITHOUT any dihedral
// transform, which is what canonicalization_test.go covers instead — that
// test happens to also relabel by first-appearance after each spatial
// transform, but never proves invariance under an arbitrary label
// permutation applied in isolation.
func permuteRegionLabels(region []int, perm []int) []int {
	out := make([]int, len(region))
	for i, id := range region {
		out[i] = perm[id]
	}
	return out
}

// TestFingerprint_ArbitraryColorPermutation_SameFingerprint pins the
// "color labels are non-semantic" gotcha directly: relabeling a puzzle's
// regions under an arbitrary permutation (no rotation/reflection at all)
// must not change its fingerprint.
func TestFingerprint_ArbitraryColorPermutation_SameFingerprint(t *testing.T) {
	fp := NewFingerprinter()
	base := goldenUniquePuzzle6()
	want := fp.Fingerprint(base)

	// A non-trivial permutation of the 6 region ids (no fixed points beyond
	// what's unavoidable, deliberately not sorted/identity).
	perm := []int{3, 5, 0, 4, 1, 2}
	relabeled := Puzzle{
		N:      base.N,
		Region: permuteRegionLabels(base.Region, perm),
		Givens: append([]int(nil), base.Givens...),
		SeedV:  base.SeedV,
		DiffV:  base.DiffV,
	}

	got := fp.Fingerprint(relabeled)
	if got != want {
		t.Errorf("Fingerprint(arbitrary color permutation) = %x, want %x (unchanged; colors are non-semantic)", got, want)
	}

	// Sanity: the permuted region grid must still solve to the same unique
	// solution and remain connected — the permutation didn't accidentally
	// change the puzzle's actual shape, only its labels.
	if !regionsConnected(relabeled.N, relabeled.Region) {
		t.Fatalf("permuteRegionLabels produced a disconnected region — test fixture bug, not a product bug")
	}
	solver := NewSolver()
	sol, ok := solver.Solve(relabeled)
	if !ok || !reflect.DeepEqual(sol, goldenUniqueSolution6()) {
		t.Fatalf("relabeled puzzle solves to %+v (ok=%v), want the same golden solution %+v — test fixture bug, not a product bug", sol, ok, goldenUniqueSolution6())
	}
}

// --- Seam check: encoding must never leak the solution ---

// TestEncode_DoesNotLeakSolution pins the seam requirement that a puzzle's
// on-disk encoding carries clues only. It checks structurally (the wire
// format has no field capable of holding a QueenAt-shaped solution) by
// round-tripping a puzzle and confirming Decode recovers exactly the clue
// data Encode was given — nothing more, nothing solution-shaped smuggled in
// or out.
func TestEncode_DoesNotLeakSolution(t *testing.T) {
	p := goldenUniquePuzzle6()
	// Givens intentionally left empty on this fixture; also check a variant
	// with a given so both branches of the wire struct are exercised.
	withGiven := p
	withGiven.Givens = []int{engine.Index(engine.Cell{Row: 0, Col: 2}, p.N)} // matches the golden solution's row-0 queen

	for _, puzzle := range []Puzzle{p, withGiven} {
		data, err := Encode(puzzle)
		if err != nil {
			t.Fatalf("Encode(%+v) returned error: %v", puzzle, err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode() returned error: %v", err)
		}
		if decoded.N != puzzle.N {
			t.Errorf("Decode().N = %d, want %d", decoded.N, puzzle.N)
		}
		if !reflect.DeepEqual(decoded.Region, puzzle.Region) {
			t.Errorf("Decode().Region mismatch")
		}
		if !reflect.DeepEqual(decoded.Givens, puzzle.Givens) {
			t.Errorf("Decode().Givens = %+v, want %+v", decoded.Givens, puzzle.Givens)
		}
		if decoded.DiffV != puzzle.DiffV {
			t.Errorf("Decode().DiffV = %v, want %v", decoded.DiffV, puzzle.DiffV)
		}

		// The encoded bytes must still let a solver derive the answer (that's
		// the point of an unambiguous puzzle) but must not contain a
		// ready-made QueenAt array anywhere: the wire struct (see entry.go)
		// has exactly Game/N/Region/Givens/Seed/Difficulty fields, none of
		// which is a length-N per-row column array distinct from Givens. We
		// assert that decoding never yields more placed cells than the
		// puzzle's own Givens declared — i.e. nothing beyond stated givens
		// arrived "for free".
		if len(decoded.Givens) != len(puzzle.Givens) {
			t.Errorf("Decode() produced %d givens, want exactly the %d encoded (no extra solution leakage)", len(decoded.Givens), len(puzzle.Givens))
		}
	}
}

// TestEntry_Verify_AcceptsGenerated_RejectsTamperedRegion exercises Entry()'s
// Verify hook end-to-end: a freshly generated, encoded puzzle must verify,
// and corrupting its region grid (breaking the N-connected-regions
// invariant) must be rejected.
func TestEntry_Verify_AcceptsGenerated_RejectsTamperedRegion(t *testing.T) {
	e := Entry()
	r := engine.NewRand(7)
	gen, err := e.Generate(engine.Easy, r)
	if err != nil {
		t.Fatalf("Entry().Generate returned error: %v", err)
	}
	if err := e.Verify(gen.Encoded); err != nil {
		t.Fatalf("Entry().Verify(freshly generated) returned error: %v", err)
	}

	p, err := Decode(gen.Encoded)
	if err != nil {
		t.Fatalf("Decode() returned error: %v", err)
	}
	// Collapse every cell into region 0: still in-bounds, but no longer N
	// connected regions labeled 0..N-1 (structurallyValid must reject it).
	tampered := make([]int, len(p.Region))
	p.Region = tampered
	badData, err := Encode(p)
	if err != nil {
		t.Fatalf("Encode(tampered) returned error: %v", err)
	}
	if err := e.Verify(badData); err == nil {
		t.Errorf("Entry().Verify(tampered region, all cells region 0) = nil error, want rejection")
	}
}
