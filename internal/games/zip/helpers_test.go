package zip

import (
	"math/rand/v2"
	"os"
	"strconv"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// seedCount reads LIG_SEEDS for the number of seeds property tests should
// cover, defaulting to 250 so CI stays fast; nightly fuzz runs can set it
// higher. See docs/plan/docs/04-testing-strategy.md.
func seedCount() int {
	if v := os.Getenv("LIG_SEEDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 250
}

// serpentinePath returns the boustrophedon (row-major, alternating
// direction) Hamiltonian path over a rows×cols grid: row 0 left-to-right,
// row 1 right-to-left, etc. Used to build hand-written fixtures; this is
// fixture construction, not the generator/solver logic under test.
func serpentinePath(rows, cols int) []int {
	path := make([]int, 0, rows*cols)
	for r := 0; r < rows; r++ {
		if r%2 == 0 {
			for c := 0; c < cols; c++ {
				path = append(path, engine.Index(engine.Cell{Row: r, Col: c}, cols))
			}
		} else {
			for c := cols - 1; c >= 0; c-- {
				path = append(path, engine.Index(engine.Cell{Row: r, Col: c}, cols))
			}
		}
	}
	return path
}

// The methods under test all currently panic("todo"). A raw panic in a Go
// test aborts the entire test binary (later tests never run), which would
// hide the very red-ness we need to confirm across ~30 tests in one
// `go test` invocation. These must{Xxx} helpers recover that panic and turn
// it into a normal, isolated t.Fatalf failure for the single calling test,
// so every test in the package reports its own red result. Once the green
// agent removes the todo panics, these helpers become transparent passthroughs.

func mustViolations(t *testing.T, v Validator, b Board) []engine.Violation {
	t.Helper()
	var out []engine.Violation
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Violations panicked (not yet implemented): %v", r)
			}
		}()
		out = v.Violations(b)
	}()
	return out
}

func mustSolved(t *testing.T, v Validator, b Board) bool {
	t.Helper()
	var out bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Solved panicked (not yet implemented): %v", r)
			}
		}()
		out = v.Solved(b)
	}()
	return out
}

func mustSolve(t *testing.T, s Solver, p Puzzle) (Solution, bool) {
	t.Helper()
	var sol Solution
	var ok bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Solve panicked (not yet implemented): %v", r)
			}
		}()
		sol, ok = s.Solve(p)
	}()
	return sol, ok
}

func mustCountSolutions(t *testing.T, s Solver, p Puzzle, cap int) int {
	t.Helper()
	var n int
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("CountSolutions panicked (not yet implemented): %v", r)
			}
		}()
		n = s.CountSolutions(p, cap)
	}()
	return n
}

func mustLogicSolve(t *testing.T, s Solver, p Puzzle) (Solution, bool, engine.Technique) {
	t.Helper()
	var sol Solution
	var closed bool
	var tech engine.Technique
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("LogicSolve panicked (not yet implemented): %v", r)
			}
		}()
		sol, closed, tech = s.LogicSolve(p)
	}()
	return sol, closed, tech
}

func mustGenerate(t *testing.T, g Generator, diff engine.Difficulty, r *rand.Rand) (Puzzle, Solution, error) {
	t.Helper()
	var p Puzzle
	var sol Solution
	var err error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("Generate panicked (not yet implemented): %v", rec)
			}
		}()
		p, sol, err = g.Generate(diff, r)
	}()
	return p, sol, err
}

func mustCanonical(t *testing.T, f Fingerprinter, p Puzzle) []byte {
	t.Helper()
	var out []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Canonical panicked (not yet implemented): %v", r)
			}
		}()
		out = f.Canonical(p)
	}()
	return out
}

func mustFingerprint(t *testing.T, f Fingerprinter, p Puzzle) [32]byte {
	t.Helper()
	var out [32]byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Fingerprint panicked (not yet implemented): %v", r)
			}
		}()
		out = f.Fingerprint(p)
	}()
	return out
}
