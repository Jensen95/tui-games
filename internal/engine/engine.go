// Package engine defines the shared, game-agnostic contracts every puzzle
// implements, plus generic grid/canonicalization helpers.
//
// Hard rule (enforced by scripts/depguard.sh in CI): this package and every
// internal/games/* package must stay pure Go — no TUI, no charm.land, no os,
// no I/O, no wall clock, no global randomness. All randomness flows through an
// explicitly seeded *rand.Rand so the same seed always yields the same puzzle.
//
// These interfaces are FROZEN (Phase 0 of docs/plan). Changes go through the
// orchestrator, never through a game track.
package engine

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// GameID identifies a game in the registry (e.g. "tango", "zip").
type GameID string

// Difficulty is shared across games. Easy/Medium/Hard are guaranteed
// logic-solvable without guessing; Expert only guarantees a unique solution.
type Difficulty uint8

const (
	Easy Difficulty = iota
	Medium
	Hard
	Expert
)

var difficultyNames = [...]string{"easy", "medium", "hard", "expert"}

func (d Difficulty) String() string {
	if int(d) < len(difficultyNames) {
		return difficultyNames[d]
	}
	return fmt.Sprintf("difficulty(%d)", uint8(d))
}

// ParseDifficulty parses a difficulty name (case-insensitive).
func ParseDifficulty(s string) (Difficulty, error) {
	for i, name := range difficultyNames {
		if strings.EqualFold(s, name) {
			return Difficulty(i), nil
		}
	}
	return Easy, fmt.Errorf("unknown difficulty %q (want one of %s)", s, strings.Join(difficultyNames[:], ", "))
}

// Technique names the deepest deduction technique a logic solver needed
// (e.g. "forced-move", "row-balance"). Used for difficulty labeling and hints.
type Technique string

// Violation is one currently-broken rule on a (possibly partial) board.
// The TUI styles Cells in the error style; tests assert on Rule.
type Violation struct {
	Rule    string // stable, machine-checkable identifier, e.g. "three-in-a-row"
	Message string // human-readable explanation
	Cells   []Cell // offending cells (may be empty for global rules)
}

// Puzzle is the minimal surface every concrete puzzle type exposes.
// Concrete puzzle/solution/board types are per-game.
type Puzzle interface {
	GameID() GameID
	Difficulty() Difficulty
	Seed() int64
}

// Validator referees a board state (partial or complete).
type Validator[B any] interface {
	// Violations returns all currently-violated rules for board b.
	// For an empty/partial board it must return only already-broken rules.
	Violations(b B) []Violation
	// Solved reports whether b is a complete, valid solution.
	Solved(b B) bool
}

// Solver finds solutions and — crucially — counts them (up to a cap).
type Solver[P Puzzle, S any] interface {
	// Solve returns one solution if any exists.
	Solve(p P) (S, bool)
	// CountSolutions returns min(#solutions, cap).
	// Uniqueness check == (cap >= 2 && result == 1).
	CountSolutions(p P, cap int) int
	// LogicSolve attempts a no-guess solve; returns the solution, whether it
	// fully closed the board, and the deepest technique used.
	LogicSolve(p P) (S, bool, Technique)
}

// Generator produces fresh puzzles.
type Generator[P Puzzle, S any] interface {
	// Generate returns a puzzle guaranteed valid + uniquely solvable at ~diff,
	// together with its solution (kept private by callers; used for hints and
	// the win check). All randomness comes from r.
	Generate(diff Difficulty, r *rand.Rand) (P, S, error)
}

// Fingerprinter canonicalizes a puzzle for deduplication.
type Fingerprinter[P Puzzle] interface {
	// Canonical returns a stable, symmetry-normalized byte serialization:
	// the lexicographically smallest serialization across the game's full
	// symmetry group (see docs/plan/docs/02-engine-and-generation.md).
	Canonical(p P) []byte
	// Fingerprint is a hash of Canonical.
	Fingerprint(p P) [32]byte
}

// NewRand returns a deterministic *rand.Rand for a seed. Every generator call
// site derives its RNG through this so seed → puzzle is reproducible.
func NewRand(seed int64) *rand.Rand {
	return rand.New(rand.NewPCG(uint64(seed), 0x9e3779b97f4a7c15))
}
