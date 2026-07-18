package zip

import (
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// BenchmarkGenerate measures medium-difficulty generation against the docs/02
// perf budget (p99 < 150ms for grids up to 8x8). Each iteration uses a fresh
// seed so the benchmark reflects real generation cost, not a cached result.
func BenchmarkGenerate(b *testing.B) {
	g := Generator{}
	for i := 0; i < b.N; i++ {
		if _, _, err := g.Generate(engine.Medium, engine.NewRand(int64(i+1))); err != nil {
			b.Fatalf("Generate error: %v", err)
		}
	}
}
