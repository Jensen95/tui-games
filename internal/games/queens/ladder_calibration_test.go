package queens

import (
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// ladderGuardSeeds is a small, fixed seed count: the guard must stay fast and
// fully deterministic (same seeds every run), so it samples a modest batch
// rather than honoring LIG_SEEDS like the heavier property tests.
const ladderGuardSeeds = 12

// TestGenerate_DifficultyLadder_StaysCalibrated is the calibration regression
// guard for the Queens difficulty ladder. It pins the properties the empirical
// audit fixed, so they cannot silently regress:
//
//  1. Grid size climbs monotonically across the tiers and Expert's band is
//     cleanly separated from — and larger than — Hard's. The audit found Hard
//     and Expert overlapping at N=10; here we assert both the structural bands
//     (difficultyBand) and the realized average N over a batch of seeds.
//
//  2. Easy/Medium/Hard stay fully no-guess: every generated board closes under
//     Solver.LogicSolve, the engine contract's no-guess oracle. Their
//     no-guess-close rate is exactly 1.0.
//
//  3. Expert spends the guessing allowance the contract grants it: NO Expert
//     board is solvable by pure forward deduction — every one requires the
//     ladder's proof-by-contradiction step (trial-and-error "guess and check").
//     So Expert's pure-deduction close rate is 0, measurably below the 1.0
//     no-guess rate the easier tiers hold.
//
// Why this framing. This package's Solver.LogicSolve includes a
// proof-by-contradiction technique strong enough that essentially every
// uniquely-solvable Queens board closes under it (verified empirically over
// hundreds of generated boards and thousands of neighbourhood-search nodes).
// So the raw "closes under LogicSolve" rate is ~1.0 for all four tiers and
// cannot by itself distinguish the guessing allowance. The generator therefore
// makes the distinction one level down and enforceable: Expert boards must fail
// pure forward deduction (closesByForwardDeduction == false), guaranteeing they
// exercise proof-by-contradiction, while the easier tiers remain no-guess under
// the full oracle.
func TestGenerate_DifficultyLadder_StaysCalibrated(t *testing.T) {
	if raceEnabled {
		// This guard generates Expert (N=11) boards, whose complete-solver
		// generation is ~an order of magnitude slower under the race detector on
		// a 2-core CI runner — enough to blow the package test timeout. The
		// calibration properties it pins are deterministic and need no race
		// instrumentation, so the uninstrumented LIG_SEEDS job carries this
		// coverage.
		t.Skip("skipped under -race: Expert N=11 generation is too slow; covered by the non-race run")
	}
	gen := NewGenerator()
	solver := NewSolver()

	// --- Structural bands: disjoint and ordered, Expert strictly above Hard. ---
	order := []engine.Difficulty{engine.Easy, engine.Medium, engine.Hard, engine.Expert}
	type band struct {
		diff   engine.Difficulty
		lo, hi int
	}
	bands := make([]band, 0, len(order))
	for _, d := range order {
		lo, hi := difficultyBand(d)
		if lo > hi {
			t.Fatalf("difficultyBand(%s) = [%d,%d]: lo must not exceed hi", d, lo, hi)
		}
		bands = append(bands, band{d, lo, hi})
	}
	for i := 1; i < len(bands); i++ {
		if bands[i].lo <= bands[i-1].hi {
			t.Errorf("bands overlap or touch: %s=[%d,%d] then %s=[%d,%d]; each tier's low bound must exceed the previous tier's high bound",
				bands[i-1].diff, bands[i-1].lo, bands[i-1].hi, bands[i].diff, bands[i].lo, bands[i].hi)
		}
	}
	// The audit's headline defect: Hard and Expert sharing N=10.
	hardLo, hardHi := difficultyBand(engine.Hard)
	expLo, expHi := difficultyBand(engine.Expert)
	if expLo <= hardHi {
		t.Errorf("Expert band [%d,%d] not separated from Hard band [%d,%d]: Expert.lo must exceed Hard.hi", expLo, expHi, hardLo, hardHi)
	}

	// --- Realized metrics over a fixed batch of seeds. ---
	type stats struct {
		sumN       int
		fullClosed int // closes under Solver.LogicSolve (contract no-guess oracle)
		pureClosed int // closes under pure forward deduction (no proof-by-contradiction)
	}
	measure := func(diff engine.Difficulty) stats {
		var s stats
		lo, hi := difficultyBand(diff)
		for seed := 1; seed <= ladderGuardSeeds; seed++ {
			p, _, err := gen.Generate(diff, engine.NewRand(int64(seed)))
			if err != nil {
				t.Fatalf("Generate(%s, seed=%d) error: %v", diff, seed, err)
			}
			if p.N < lo || p.N > hi {
				t.Errorf("Generate(%s, seed=%d): N=%d outside tier band [%d,%d]", diff, seed, p.N, lo, hi)
			}
			if got := solver.CountSolutions(p, 2); got != 1 {
				t.Errorf("Generate(%s, seed=%d): CountSolutions=%d, want 1 (unique)", diff, seed, got)
			}
			s.sumN += p.N
			if _, closed, _ := solver.LogicSolve(p); closed {
				s.fullClosed++
			}
			if closesByForwardDeduction(p) {
				s.pureClosed++
			}
		}
		return s
	}

	easy := measure(engine.Easy)
	medium := measure(engine.Medium)
	hard := measure(engine.Hard)
	expert := measure(engine.Expert)

	avg := func(s stats) float64 { return float64(s.sumN) / float64(ladderGuardSeeds) }
	avgE, avgM, avgH, avgX := avg(easy), avg(medium), avg(hard), avg(expert)

	// avgN non-decreasing across the ladder, with Expert strictly larger than
	// both Easy (a real ladder) and Hard (the specific defect: Expert must now
	// exceed Hard rather than re-sampling its sizes).
	if !(avgE <= avgM && avgM <= avgH && avgH <= avgX) {
		t.Errorf("avgN not non-decreasing across ladder: Easy=%.2f Medium=%.2f Hard=%.2f Expert=%.2f", avgE, avgM, avgH, avgX)
	}
	if !(avgX > avgE) {
		t.Errorf("Expert avgN (%.2f) not strictly greater than Easy avgN (%.2f)", avgX, avgE)
	}
	if !(avgX > avgH) {
		t.Errorf("Expert avgN (%.2f) not strictly greater than Hard avgN (%.2f): Expert no longer exceeds Hard", avgX, avgH)
	}

	// Easy/Medium/Hard are fully no-guess under the contract oracle (rate 1.0).
	for _, tc := range []struct {
		diff engine.Difficulty
		s    stats
	}{{engine.Easy, easy}, {engine.Medium, medium}, {engine.Hard, hard}} {
		if tc.s.fullClosed != ladderGuardSeeds {
			t.Errorf("%s is a no-guess tier but only %d/%d boards closed under LogicSolve (rate must be 1.0)", tc.diff, tc.s.fullClosed, ladderGuardSeeds)
		}
	}

	// Expert genuinely spends the guessing allowance: not one Expert board is
	// solvable by pure forward deduction, so every one requires the solver's
	// proof-by-contradiction step. Its pure-deduction close rate is 0 —
	// measurably below the easier tiers' 1.0 no-guess rate.
	if expert.pureClosed != 0 {
		t.Errorf("Expert pure-deduction close count = %d/%d, want 0 (every Expert board must require proof-by-contradiction)", expert.pureClosed, ladderGuardSeeds)
	}
	// Sanity that the two oracles are not accidentally equivalent: the easier
	// tiers must NOT be all-guess. Easy clears a real share by pure deduction,
	// confirming pure deduction is a meaningfully weaker solver than the full
	// ladder and that the Expert==0 result above is a genuine escalation.
	if easy.pureClosed == 0 {
		t.Errorf("Easy pure-deduction close count = 0/%d; expected several Easy boards to close by pure deduction (guard oracle looks broken)", ladderGuardSeeds)
	}
}
