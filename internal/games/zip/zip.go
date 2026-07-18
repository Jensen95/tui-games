// Package zip implements the "Zip" path-drawing puzzle: draw one continuous
// Hamiltonian path through every cell of an R×C grid, passing through
// numbered waypoints in ascending order, moving only orthogonally, and never
// crossing a wall edge.
//
// See docs/plan/games/zip.md for the full specification (rules, data model,
// generation approach, solver approach, and the TDD test matrix this package
// is built against).
//
// This file holds only the data model and the declarations the accompanying
// _test.go files compile against. Every method body that represents real
// game logic is left as panic("todo") — a later implementation pass fills
// them in. Only trivial, non-logic scaffolding (struct field accessors, the
// wall-edge key helper) is implemented for real, since there is no behavior
// there for a later agent to get right or wrong.
package zip

import (
	"math/rand/v2"

	"github.com/Jensen95/tui-games/internal/engine"
)

// ID is this game's registry identifier.
const ID engine.GameID = "zip"

// Rule identifiers used in engine.Violation.Rule. Stable and
// machine-checkable; tests assert on these exact strings.
const (
	// RuleRevisit fires when a cell appears more than once in the path.
	RuleRevisit = "revisit"
	// RuleNonAdjacentStep fires when two consecutive path cells are not
	// orthogonally adjacent — covers both diagonal steps and larger jumps
	// (e.g. two cells in the same row two columns apart). Per the spec,
	// diagonal moves are never legal, only up/down/left/right.
	RuleNonAdjacentStep = "non-adjacent-step"
	// RuleWallCrossing fires when a step crosses a walled edge. Walls sit on
	// edges, not cells, and block movement in either direction across that
	// edge (the map key is direction-agnostic — see WallKey).
	RuleWallCrossing = "wall-crossing"
	// RuleWaypointOrder fires when numbered cells are encountered out of
	// ascending order (e.g. reaching 3 before 2).
	RuleWaypointOrder = "waypoint-order"
	// RuleWrongStart fires when the path's first cell is not the cell
	// numbered 1.
	RuleWrongStart = "wrong-start"
)

// Techniques named by the logic/forced-move solver, for difficulty labeling
// and hints.
const (
	TechniqueForcedMove engine.Technique = "forced-move"
)

// Puzzle is one Zip puzzle: an R×C grid with some cells carrying waypoint
// numbers 1..K (1 = start, K = end) and a set of blocked edges (walls).
//
// Cell indices throughout are row-major: engine.Index(engine.Cell{Row, Col}, C).
type Puzzle struct {
	R, C int

	// Waypoint maps a cell index to its waypoint number (1..K). Most cells
	// are absent from this map (unnumbered).
	Waypoint map[int]int

	// Walls is the set of blocked edges between orthogonally adjacent cells,
	// keyed by WallKey(a, b) for the two cell indices the edge sits between.
	Walls map[[2]int]bool

	// SeedVal is the seed this puzzle was generated from (0 for hand-built
	// fixtures not produced by Generator).
	SeedVal int64

	// Diff is the puzzle's labeled difficulty.
	Diff engine.Difficulty
}

// GameID implements engine.Puzzle.
func (p Puzzle) GameID() engine.GameID { return ID }

// Difficulty implements engine.Puzzle.
func (p Puzzle) Difficulty() engine.Difficulty { return p.Diff }

// Seed implements engine.Puzzle.
func (p Puzzle) Seed() int64 { return p.SeedVal }

// Solution is a complete Hamiltonian path: len(Path) == R*C, ordered cell
// indices from the cell numbered 1 to the cell numbered K.
type Solution struct {
	Path []int
}

// Board is a (possibly partial) drawn path against a Puzzle. The Validator
// uses it both for live/partial checking (TUI, in-progress drag) and for
// full Solved-state checking (Path complete, len(Path) == Puzzle.R*Puzzle.C).
type Board struct {
	Puzzle Puzzle
	Path   []int
}

// WallKey normalizes an adjacent cell-index pair into the unordered map key
// used by Puzzle.Walls, per the data model in docs/plan/games/zip.md. Walls
// live on edges, not cells or directed steps, so WallKey(a, b) == WallKey(b, a).
func WallKey(a, b int) [2]int {
	if a > b {
		a, b = b, a
	}
	return [2]int{a, b}
}

// Validator referees Board state per docs/plan/games/zip.md's Solved-state
// definition and partial-validator rules.
type Validator struct{}

// Violations returns all currently-violated rules for board b. For an
// empty/partial board it must return only already-broken rules: an
// incomplete-but-not-yet-invalid path (including one that has stranded an
// unreachable cell — "trapping") yields no violations. Trapping is a UX
// hint, not a hard-validator rule (see spec Gotchas).
func (Validator) Violations(b Board) []engine.Violation {
	return violationsOf(b)
}

// Solved reports whether b.Path is a complete, valid Zip solution: every
// cell visited exactly once, only orthogonal wall-free steps, starting at
// the cell numbered 1, and numbered cells encountered in ascending order.
func (Validator) Solved(b Board) bool {
	return solvedCheck(b.Puzzle, b.Path)
}

// Solver finds and counts Hamiltonian-path solutions consistent with a
// Puzzle's fixed endpoints, waypoints, and walls.
type Solver struct{}

// Solve returns one valid solution if any exists.
func (Solver) Solve(p Puzzle) (Solution, bool) {
	count, first := enumerateSolutions(p, 1, true)
	if count == 0 {
		return Solution{}, false
	}
	return Solution{Path: first}, true
}

// CountSolutions returns min(#solutions, cap).
func (Solver) CountSolutions(p Puzzle, cap int) int {
	count, _ := enumerateSolutions(p, cap, false)
	return count
}

// LogicSolve attempts a no-guess solve via repeated forced-edge deduction;
// returns the solution, whether it fully closed the board, and the deepest
// technique used.
func (Solver) LogicSolve(p Puzzle) (Solution, bool, engine.Technique) {
	path, closed := logicSolvePath(p)
	if !closed {
		return Solution{Path: path}, false, ""
	}
	return Solution{Path: path}, true, TechniqueForcedMove
}

// Generator produces fresh Zip puzzles: build a random Hamiltonian path
// first, place waypoints/walls along it, then verify uniqueness before
// returning (see spec "Generation approach").
type Generator struct{}

// Generate returns a puzzle guaranteed valid + uniquely solvable at ~diff,
// together with its solution. All randomness comes from r.
func (Generator) Generate(diff engine.Difficulty, r *rand.Rand) (Puzzle, Solution, error) {
	return generateZip(diff, r)
}

// Fingerprinter canonicalizes a Puzzle for dedup over the 8 dihedral
// transforms only — numbering fixes the path's direction, so a path's
// reversal is a *different* puzzle (1 and K swap) and is deliberately not
// collapsed by canonicalization (see spec Gotchas: "reversal/direction
// subtlety in dedup").
type Fingerprinter struct{}

// Canonical returns the lexicographically smallest serialization of p's
// (waypoints, walls) layout across the 8 dihedral transforms.
func (Fingerprinter) Canonical(p Puzzle) []byte {
	return canonicalBytes(p)
}

// Fingerprint hashes Canonical(p).
func (Fingerprinter) Fingerprint(p Puzzle) [32]byte {
	return engine.FingerprintBytes(canonicalBytes(p))
}

// Entry returns the engine registry entry for zip. The orchestrator wires it
// into internal/games/all; this package does not self-register.
func Entry() engine.Entry {
	return engine.Entry{
		ID:   ID,
		Name: "Zip",
		Generate: func(diff engine.Difficulty, r *rand.Rand) (engine.Generated, error) {
			p, sol, err := Generator{}.Generate(diff, r)
			if err != nil {
				return engine.Generated{}, err
			}
			return engine.Generated{
				Puzzle:      p,
				Solution:    sol,
				Encoded:     Encode(p),
				Fingerprint: engine.FingerprintBytes(canonicalBytes(p)),
			}, nil
		},
		Verify: func(encoded []byte) error {
			return verifyEncoded(encoded)
		},
	}
}

// Compile-time checks that the concrete types satisfy the frozen engine
// contracts (internal/engine/engine.go). These are declarations only, not
// behavior: they cost nothing to leave real.
var (
	_ engine.Puzzle                      = Puzzle{}
	_ engine.Validator[Board]            = Validator{}
	_ engine.Solver[Puzzle, Solution]    = Solver{}
	_ engine.Generator[Puzzle, Solution] = Generator{}
	_ engine.Fingerprinter[Puzzle]       = Fingerprinter{}
)
