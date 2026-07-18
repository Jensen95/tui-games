package tango

import (
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// BenchmarkGenerate measures Generate at Medium difficulty against the
// docs/plan/docs/02-engine-and-generation.md perf budget for Tango
// (p99 < 20ms, single core).
func BenchmarkGenerate(b *testing.B) {
	gen := Generator{}
	for i := 0; i < b.N; i++ {
		r := engine.NewRand(int64(i + 1))
		if _, _, err := gen.Generate(engine.Medium, r); err != nil {
			b.Fatalf("Generate failed: %v", err)
		}
	}
}
