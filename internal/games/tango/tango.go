// Package tango implements LinkedIn-style Tango (a Takuzu/Binairo variant):
// a 6×6 grid where every cell holds a Sun or a Moon, subject to row/column
// balance, no-three-in-a-row (horizontal/vertical only, never diagonal), and
// optional "=" (equal) / "×" (cross) edge constraints between orthogonally
// adjacent cells. See docs/plan/games/tango.md for the full spec.
//
// This file holds the data model and package-level constants; the
// validator, solvers, generator, fingerprinter, encoding and registry entry
// live in the sibling files of this package.
package tango

import (
	"github.com/Jensen95/tui-games/internal/engine"
)

// GameID is this game's identifier in the engine registry.
const GameID = engine.GameID("tango")

// N is the fixed grid size (6×6). The engine is parameterized on N
// internally so a future even-sized variant stays easy, but every Generate
// call currently produces exactly N×N puzzles.
const N = 6

// Symbol represents a cell value: Empty, Sun, or Moon.
type Symbol uint8

const (
	Empty Symbol = iota
	Sun
	Moon
)

// flip swaps Sun<->Moon, leaving Empty untouched. Used both by the logic
// solver (an edge/pair/gap deduction forces "the other symbol") and by the
// fingerprinter (the sun<->moon symmetry of the game).
func flip(s Symbol) Symbol {
	switch s {
	case Sun:
		return Moon
	case Moon:
		return Sun
	default:
		return s
	}
}

// Relation represents an edge constraint between adjacent cells.
type Relation uint8

const (
	None Relation = iota
	Equal
	Cross
)

// Board represents the current state of the grid being solved: the fixed
// edge constraints plus the current (partial or complete) cell assignment.
type Board struct {
	N      int
	Cells  []Symbol            // len N*N, row-major
	HEdges map[[2]int]Relation // horizontal-neighbor relations
	VEdges map[[2]int]Relation // vertical-neighbor relations
}

// Puzzle represents a tango puzzle: givens and edge constraints, never the
// solution (that is kept out-of-band by callers, per engine.Generated).
type Puzzle struct {
	N       int
	Givens  map[int]Symbol      // cell index -> locked symbol
	HEdges  map[[2]int]Relation // horizontal-neighbor relations
	VEdges  map[[2]int]Relation // vertical-neighbor relations
	SeedVal int64               // seed value
	Diff    engine.Difficulty
}

func (p Puzzle) GameID() engine.GameID         { return GameID }
func (p Puzzle) Difficulty() engine.Difficulty { return p.Diff }
func (p Puzzle) Seed() int64                   { return p.SeedVal }

var _ engine.Puzzle = Puzzle{}
