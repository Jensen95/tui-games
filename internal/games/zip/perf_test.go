package zip

import (
	"sort"
	"testing"
	"time"

	"github.com/Jensen95/tui-games/internal/engine"
)

// TestGenerator_LatencyBudget pins the spec's Gotchas entry: "Generation
// performance is the project's main algorithmic risk" and the TDD matrix
// item "Generation p99 latency under target" (docs/plan/docs/02-engine-and-
// generation.md: Zip's budget is < 150ms for grids up to 8x8). Unlike
// BenchmarkGenerate (bench_test.go), which only runs under `go test -bench`
// and asserts nothing, this is a real `go test` case with a hard assertion,
// so a generator regression (e.g. someone swapping the fast backbite path
// for a naive unbounded DFS) fails CI instead of only showing up in an
// unread benchmark log.
func TestGenerator_LatencyBudget(t *testing.T) {
	if raceEnabled {
		t.Skip("latency budget is not meaningful under the race detector (10-50x slowdown); the non-race CI job asserts it")
	}
	n := seedCount()
	// Latency measurement doesn't need the full property-test seed count to
	// be meaningful and 250+ timed generations would slow this test down for
	// little benefit; cap it independently of LIG_SEEDS.
	if n > 60 {
		n = 60
	}
	gen := Generator{}

	const budget = 150 * time.Millisecond
	var samples []time.Duration
	var max time.Duration

	for _, diff := range []engine.Difficulty{engine.Easy, engine.Medium, engine.Hard, engine.Expert} {
		for seed := 1; seed <= n; seed++ {
			start := time.Now()
			if _, _, err := gen.Generate(diff, engine.NewRand(int64(seed))); err != nil {
				t.Fatalf("Generate(%s, seed=%d) error: %v", diff, seed, err)
			}
			d := time.Since(start)
			samples = append(samples, d)
			if d > max {
				max = d
			}
		}
	}

	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	p99 := samples[(len(samples)*99)/100]

	if p99 > budget {
		t.Errorf("Generate p99 latency = %v, want <= %v (docs/02 budget, grids up to 8x8; ours are %v)", p99, budget, "5x5 Easy / 6x6 Medium+Expert / 6x7 Hard")
	}
	// The max is allowed some headroom over the p99 budget (a single slow
	// outlier on a loaded CI box shouldn't flake this test), but a max many
	// multiples over budget is itself a signal something regressed badly.
	if hardMax := 4 * budget; max > hardMax {
		t.Errorf("Generate max latency = %v, want <= %v (4x budget headroom)", max, hardMax)
	}
}
