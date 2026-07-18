// Package queens implements LinkedIn-style Queens: an N×N grid divided into
// N connected colored regions, where a valid placement puts exactly one
// queen per row, per column, per region, with no two queens touching —
// including diagonally (local 8-neighbor adjacency only, never the classic
// full chess diagonal). See docs/plan/games/queens.md for the full spec.
//
// This file holds the data model and package-level constants; the validator,
// solvers, generator, fingerprinter, and registry entry live in the sibling
// files of this package.
package queens

import (
	"github.com/Jensen95/tui-games/internal/engine"
)

// GameID is this game's identifier in the engine registry.
const GameID engine.GameID = "queens"

// Cell is a board cell's state. Marked (the player's "X" note) is a TUI-only
// concept and intentionally has no representation here — the engine board
// only ever models Empty/Queen, per docs/plan/games/queens.md's data model.
type Cell uint8

const (
	Empty Cell = iota
	Queen
)

// Violation rule identifiers. These are the stable, machine-checkable Rule
// values engine.Violation carries; tests assert on them directly.
const (
	RuleSameRow    = "same-row"
	RuleSameCol    = "same-col"
	RuleSameRegion = "same-region"
	// RuleAdjacent fires for any 8-neighbor (edge- or corner-) touch between
	// two queens. It must NOT fire for two queens that merely share a full
	// diagonal but sit farther apart than one cell — that's the classic
	// chess-diagonal bug this game must not reproduce.
	RuleAdjacent = "adjacent"
)

// Puzzle is one Queens board: an N×N region coloring plus optional givens.
// Region has length N*N, row-major (see engine.Index/engine.CellAt), and
// holds a region id (0..N-1) per cell. Givens holds row-major cell indices
// that start with a queen pre-placed (optional; may be empty/nil).
type Puzzle struct {
	N      int
	Region []int
	Givens []int
	SeedV  int64
	DiffV  engine.Difficulty
}

// GameID implements engine.Puzzle.
func (p Puzzle) GameID() engine.GameID { return GameID }

// Difficulty implements engine.Puzzle.
func (p Puzzle) Difficulty() engine.Difficulty { return p.DiffV }

// Seed implements engine.Puzzle.
func (p Puzzle) Seed() int64 { return p.SeedV }

var _ engine.Puzzle = Puzzle{}

// Solution is one row-per-queen placement: QueenAt[row] is the column of
// that row's queen. Length N. Baking in "one per row" shrinks the search
// space, per docs/plan/games/queens.md's data model sketch.
type Solution struct {
	N       int
	QueenAt []int
}

// Board is the mutable board state a Validator referees: the puzzle's fixed
// region coloring plus the current queen placement (partial or complete).
// Cells has length N*N, row-major.
type Board struct {
	N      int
	Region []int
	Cells  []Cell
}

// Technique name constants used by LogicSolve's deduction ladder
// (docs/plan/games/queens.md "Deduction ladder"). Exact naming is an
// implementation detail; tests only assert non-emptiness when a puzzle
// closes, never a specific technique string.
const (
	TechniqueNone               engine.Technique = ""
	TechniqueSingleton          engine.Technique = "singleton"
	TechniqueElimination        engine.Technique = "elimination-cascade"
	TechniqueRegionLineLock     engine.Technique = "region-line-lock"
	TechniqueSetLocking         engine.Technique = "set-locking"
	TechniqueAdjacencyExclusion engine.Technique = "adjacency-exclusion"
)
