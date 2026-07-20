package patches

import (
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// techStats summarizes, over a batch of generated puzzles for one tier, how
// deep a no-guess solve had to reach: the mean deepest-technique rank and the
// fraction of puzzles that required at least the given rank.
type techStats struct {
	n        int
	sumRank  int
	atLeast2 int // required cell-forced or deeper
	atLeast3 int // required contradiction-elimination
}

func (s techStats) meanRank() float64     { return float64(s.sumRank) / float64(s.n) }
func (s techStats) fracAtLeast2() float64 { return float64(s.atLeast2) / float64(s.n) }
func (s techStats) fracAtLeast3() float64 { return float64(s.atLeast3) / float64(s.n) }

// gatherTechStats generates seeds 1..n puzzles at diff and records the deepest
// technique each one's no-guess solve required. Every generated puzzle must be
// logic-solvable (Easy/Medium/Hard are no-guess tiers), so a non-closing solve
// is a hard failure.
func gatherTechStats(t *testing.T, diff engine.Difficulty, n int) techStats {
	t.Helper()
	var st techStats
	gen := NewGenerator()
	for seed := int64(1); seed <= int64(n); seed++ {
		r := engine.NewRand(seed)
		p, _, err := gen.Generate(diff, r)
		if err != nil {
			t.Fatalf("%v seed %d: Generate failed: %v", diff, seed, err)
		}
		_, closed, tech := NewSolver(p).LogicSolve(p)
		if !closed {
			t.Fatalf("%v seed %d: puzzle not logic-solvable", diff, seed)
		}
		rank := techRank(tech)
		st.n++
		st.sumRank += rank
		if rank >= techRank(TechniqueCellForced) {
			st.atLeast2++
		}
		if rank >= techRank(TechniqueContradiction) {
			st.atLeast3++
		}
	}
	return st
}

// TestDifficultyFloor_TierSeparation is the regression guard for the
// deepest-technique floors added to Generate. Before the floors, the audit
// found Easy and Medium statistically identical: ~99% of puzzles at every tier
// solved on the shallowest 'clue-singleton' rung, so the mean deepest-technique
// rank was flat (Easy 1.008 == Medium 1.008 < Hard 1.042). This test locks in
// that the tiers are now genuinely separated and, in particular, that Medium
// requires deeper techniques than Easy on a meaningful fraction of puzzles.
//
// It is deterministic (fixed seeds) and fast (a small, LIG_SEEDS-capped batch,
// independent of the larger property-test sweep).
func TestDifficultyFloor_TierSeparation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping difficulty-floor guard in short mode")
	}

	// Fixed, modest batch so this guard stays fast even under LIG_SEEDS=300;
	// large enough to be statistically stable given the observed fractions.
	n := 120
	if sc := getSeedCount(); sc < n {
		n = sc
	}

	easy := gatherTechStats(t, engine.Easy, n)
	med := gatherTechStats(t, engine.Medium, n)
	hard := gatherTechStats(t, engine.Hard, n)

	t.Logf("mean deepest-technique rank: easy=%.3f medium=%.3f hard=%.3f",
		easy.meanRank(), med.meanRank(), hard.meanRank())
	t.Logf("fraction requiring >= cell-forced: easy=%.2f medium=%.2f hard=%.2f",
		easy.fracAtLeast2(), med.fracAtLeast2(), hard.fracAtLeast2())
	t.Logf("fraction requiring contradiction-elimination: hard=%.2f", hard.fracAtLeast3())

	// Monotonic ladder: Easy <= Medium <= Hard on mean deepest rank.
	if !(easy.meanRank() <= med.meanRank()) {
		t.Errorf("mean rank not monotonic Easy<=Medium: easy=%.3f medium=%.3f",
			easy.meanRank(), med.meanRank())
	}
	if !(med.meanRank() <= hard.meanRank()) {
		t.Errorf("mean rank not monotonic Medium<=Hard: medium=%.3f hard=%.3f",
			med.meanRank(), hard.meanRank())
	}

	// Easy vs Medium must be measurably DIFFERENT — the core regression. Easy
	// should almost never need cell-forced (it accepts the shallowest rung),
	// while Medium's floor forces cell-forced-or-deeper on the large majority.
	if easy.fracAtLeast2() > 0.10 {
		t.Errorf("Easy required cell-forced-or-deeper too often (%.2f > 0.10); "+
			"Easy should sit on the shallowest rung", easy.fracAtLeast2())
	}
	if med.fracAtLeast2() < 0.75 {
		t.Errorf("Medium did not require cell-forced-or-deeper on enough puzzles "+
			"(%.2f < 0.75); Easy and Medium are not separated", med.fracAtLeast2())
	}
	// Guard the gap explicitly so a future regression that flattens the tiers
	// (as the audit originally found) fails loudly here.
	if med.fracAtLeast2()-easy.fracAtLeast2() < 0.50 {
		t.Errorf("Easy vs Medium gap too small: easy=%.2f medium=%.2f (want gap >= 0.50)",
			easy.fracAtLeast2(), med.fracAtLeast2())
	}

	// Hard must have a distinct signature: it requires the deepest
	// (contradiction-elimination) rung on a meaningful fraction of puzzles.
	if hard.fracAtLeast3() < 0.50 {
		t.Errorf("Hard did not require contradiction-elimination on enough puzzles "+
			"(%.2f < 0.50); Hard lacks a distinct signature", hard.fracAtLeast3())
	}
}
