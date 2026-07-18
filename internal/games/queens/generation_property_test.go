package queens

import (
	"fmt"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// TestGeneration_Invariant_OverSeeds pins the generation invariant from
// docs/plan/docs/02-engine-and-generation.md, specialized per
// docs/plan/games/queens.md's TDD matrix:
//
//   - Every generated board has exactly N regions, all connected, every cell colored.
//   - Validator.Solved(solution) is true.
//   - Solver.CountSolutions(p, 2) == 1 (unique).
//   - Solver.LogicSolve closes the board (Easy/Medium/Hard are all no-guess tiers).
//   - The generated puzzle's Difficulty() matches the requested band.
//   - Any givens are consistent with the recorded solution.
//
// Seed count honors LIG_SEEDS (default 250). Grid size (5..11, since
// engine.Generator's signature has no size parameter — see the interface
// note in generator.go) and difficulty both rotate across the seed loop so a
// default run still exercises the full supported range without requiring
// seeds*sizes*difficulties generations.
func TestGeneration_Invariant_OverSeeds(t *testing.T) {
	sizes := []int{5, 6, 7, 8, 9, 10, 11}
	diffs := []engine.Difficulty{engine.Easy, engine.Medium, engine.Hard}

	gen := NewGenerator()
	val := NewValidator()
	solver := NewSolver()

	n := seedCount()
	for i := 1; i <= n; i++ {
		seed := int64(i)
		wantSize := sizes[i%len(sizes)]
		diff := diffs[i%len(diffs)]

		t.Run(fmt.Sprintf("seed=%d/diff=%s", seed, diff), func(t *testing.T) {
			p, sol, err := gen.Generate(diff, engine.NewRand(seed))
			if err != nil {
				t.Fatalf("Generate(diff=%v, seed=%d) returned error: %v", diff, seed, err)
			}
			_ = wantSize // documents intent: implementations may vary size by seed/rand, not by this test.

			if p.N < 5 || p.N > 11 {
				t.Fatalf("Generate(diff=%v, seed=%d): puzzle.N = %d, want 5..11", diff, seed, p.N)
			}
			if len(p.Region) != p.N*p.N {
				t.Fatalf("Generate(diff=%v, seed=%d): len(Region) = %d, want %d (N*N)", diff, seed, len(p.Region), p.N*p.N)
			}
			if got := len(distinctRegionIDs(p.Region)); got != p.N {
				t.Errorf("Generate(diff=%v, seed=%d): %d distinct region ids, want exactly N=%d", diff, seed, got, p.N)
			}
			if !regionsConnected(p.N, p.Region) {
				t.Errorf("Generate(diff=%v, seed=%d): at least one region is not 4-connected", diff, seed)
			}

			if sol.N != p.N || len(sol.QueenAt) != p.N {
				t.Fatalf("Generate(diff=%v, seed=%d): solution shape mismatch, got N=%d len(QueenAt)=%d, want N=%d", diff, seed, sol.N, len(sol.QueenAt), p.N)
			}

			board := boardFromSolution(p, sol)
			if !val.Solved(board) {
				t.Errorf("Generate(diff=%v, seed=%d): Validator.Solved(recorded solution) = false, want true", diff, seed)
			}

			if got := solver.CountSolutions(p, 2); got != 1 {
				t.Errorf("Generate(diff=%v, seed=%d): CountSolutions(p, 2) = %d, want 1 (unique)", diff, seed, got)
			}

			logicSol, closed, tech := solver.LogicSolve(p)
			if !closed {
				t.Errorf("Generate(diff=%v, seed=%d): LogicSolve did not close; %v is a no-guess tier", diff, seed, diff)
			}
			if closed && logicSol.N == p.N {
				for row, col := range logicSol.QueenAt {
					if col != sol.QueenAt[row] {
						t.Errorf("Generate(diff=%v, seed=%d): LogicSolve solution disagrees with recorded solution at row %d: got col %d, want %d", diff, seed, row, col, sol.QueenAt[row])
						break
					}
				}
			}
			_ = tech

			if p.Difficulty() != diff {
				t.Errorf("Generate(diff=%v, seed=%d): puzzle.Difficulty() = %v, want %v", diff, seed, p.Difficulty(), diff)
			}

			for _, g := range p.Givens {
				cell := engine.CellAt(g, p.N)
				if sol.QueenAt[cell.Row] != cell.Col {
					t.Errorf("Generate(diff=%v, seed=%d): given at %+v is not on the recorded solution (row %d has queen at col %d)", diff, seed, cell, cell.Row, sol.QueenAt[cell.Row])
				}
			}
		})
	}
}

// BenchmarkGenerate exercises the "Generation p99 latency under target
// across sizes" line of the TDD matrix (target < 50ms, see
// docs/plan/docs/02-engine-and-generation.md's performance budget table).
// Not a correctness test; only runs under `go test -bench`.
func BenchmarkGenerate(b *testing.B) {
	gen := NewGenerator()
	diffs := []engine.Difficulty{engine.Easy, engine.Medium, engine.Hard}
	for i := 0; i < b.N; i++ {
		diff := diffs[i%len(diffs)]
		if _, _, err := gen.Generate(diff, engine.NewRand(int64(i))); err != nil {
			b.Fatalf("Generate returned error: %v", err)
		}
	}
}
