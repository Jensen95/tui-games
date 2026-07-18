// Package minisudoku implements Mini Sudoku (LinkedIn's compact 6×6 Sudoku):
// a 6×6 grid filled with digits 1..6 so that every row, column, and 2×3 box
// contains each digit exactly once. See docs/plan/games/mini-sudoku.md for
// the full spec.
//
// This file holds the data model and package-level constants; the
// validator, solvers, generator, fingerprinter, encoding and registry entry
// live in the sibling files of this package.
package minisudoku

import (
	"github.com/Jensen95/tui-games/internal/engine"
)

// GameID is this game's identifier in the engine registry.
const GameID = engine.GameID("minisudoku")

// N, BoxH, BoxW are the fixed grid/box dimensions for Mini Sudoku: a 6×6
// grid of 2-row×3-column boxes. Box geometry is kept parameterized
// throughout the package (rather than hard-coded 2×3 math) so a future
// variant (e.g. 4×4 with 2×2 boxes, or 8×8 with 2×4 boxes) stays trivial —
// see docs/plan/games/mini-sudoku.md "Grid & pieces".
const (
	N    = 6 // Fixed grid size
	BoxH = 2 // Box height
	BoxW = 3 // Box width
)

// Solution is the completed puzzle: N*N cells, row-major, digits 1..N.
type Solution struct {
	Cells []int // len N*N, row-major, values 1..N
}

// Board represents the current state of a puzzle being solved (partial or
// complete): 0 marks an empty cell.
type Board struct {
	Cells []int // len N*N, row-major, values 0..N
}

// Puzzle represents a Mini Sudoku puzzle: its givens plus metadata. It never
// carries the solution — that's kept out-of-band by callers, per
// engine.Generated.
type Puzzle struct {
	N       int         // grid size (6)
	BoxH    int         // box height (2)
	BoxW    int         // box width (3)
	Givens  map[int]int // cell index -> digit 1..N
	SeedVal int64       // seed value
	Diff    engine.Difficulty
}

func (p Puzzle) GameID() engine.GameID         { return GameID }
func (p Puzzle) Difficulty() engine.Difficulty { return p.Diff }
func (p Puzzle) Seed() int64                   { return p.SeedVal }

var _ engine.Puzzle = Puzzle{}

// boxID returns the box index (0..N-1) that cell (row, col) belongs to, for
// an N×N grid divided into boxH×boxW boxes. Boxes are numbered row-major by
// band (row/boxH) then stack (col/boxW).
func boxID(row, col, boxH, boxW, n int) int {
	stacksPerRow := n / boxW
	return (row/boxH)*stacksPerRow + col/boxW
}
