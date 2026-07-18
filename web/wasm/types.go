//go:build js && wasm

// Package main is the WASM engine bridge: it exposes the pure Go engine
// (internal/engine + internal/games/*) to the browser as one global object,
// globalThis.ligEngine, via syscall/js. See web/js/api.md for the full JSON
// contract this file (and its siblings) implement.
//
// Design: this file holds the shared JSON types and the gameAdapter
// interface every per-game file (tango.go, queens.go, zip.go, patches.go,
// minisudoku.go) implements. Each adapter converts between the UI-facing
// board JSON and the game's concrete Board type, then delegates ALL rule
// logic to that game's own engine.Validator — no game rule is ever
// re-implemented here.
package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"

	"github.com/Jensen95/tui-games/internal/engine"
)

// cellJSON is the wire shape of one grid cell reference: {"row":r,"col":c},
// zero-indexed exactly like engine.Cell. Used both inside violation output
// and inside board JSON (e.g. Zip's path).
type cellJSON struct {
	Row int `json:"row"`
	Col int `json:"col"`
}

// violationJSON is the wire shape of one engine.Violation.
type violationJSON struct {
	Rule    string     `json:"rule"`
	Message string     `json:"message"`
	Cells   []cellJSON `json:"cells"`
}

func cellsToJSON(cells []engine.Cell) []cellJSON {
	out := make([]cellJSON, len(cells))
	for i, c := range cells {
		out[i] = cellJSON{Row: c.Row, Col: c.Col}
	}
	return out
}

func violationsToJSON(viols []engine.Violation) []violationJSON {
	// Always allocate (never nil) so it marshals to `[]`, not `null`.
	out := make([]violationJSON, 0, len(viols))
	for _, v := range viols {
		out = append(out, violationJSON{Rule: v.Rule, Message: v.Message, Cells: cellsToJSON(v.Cells)})
	}
	return out
}

// gameResult is the generate() success payload: the opaque puzzle JSON (the
// game's own Encode output, clues only), a game-specific solution JSON, and
// the initial board JSON (givens/clues applied, everything else empty).
type gameResult struct {
	Puzzle   json.RawMessage `json:"puzzle"`
	Solution json.RawMessage `json:"solution"`
	Board    json.RawMessage `json:"board"`
}

// gameAdapter is the per-game bridge between the engine's concrete types and
// the JS-facing JSON contract documented in web/js/api.md.
type gameAdapter interface {
	id() string
	name() string
	// generate builds a fresh puzzle at diff using r.
	generate(diff engine.Difficulty, r *rand.Rand) (gameResult, error)
	// violations decodes puzzleJSON/boardJSON into the game's concrete
	// types and returns every currently-violated rule (partial-board
	// aware — never flags an unfilled cell).
	violations(puzzleJSON, boardJSON []byte) ([]violationJSON, error)
	// solved decodes puzzleJSON/boardJSON and reports whether the board is
	// a complete, fully valid solution.
	solved(puzzleJSON, boardJSON []byte) (bool, error)
	// hint decodes puzzleJSON/boardJSON/solutionJSON and returns exactly one
	// forced move toward the recorded solution, mirroring that game's
	// internal/tui/boards adapter's Hint() method. See hintResultJSON's doc
	// comment and web/js/api.md's "hint()" section for the wire contract.
	hint(puzzleJSON, boardJSON, solutionJSON []byte) (hintResultJSON, error)
}

// hintResultJSON is the wire shape returned by globalThis.ligEngine.hint().
// Every game's hint() returns this same envelope; only the shape of Apply
// varies per game (documented per-game in web/js/api.md, exactly like every
// other board JSON shape in this bridge).
//
//   - Done is true when there is no move left to give (the board is already
//     solved, or no solution was recorded) — Message still describes why,
//     but Cells/Apply are empty/nil and the UI should not try to mutate
//     anything.
//   - Message is always a short, human-readable line describing the move
//     (and, where a technique is known, naming it) — safe to show verbatim
//     in a status line.
//   - Technique is the deepest logic technique used to derive the move,
//     when the game can name one (currently only Mini Sudoku; "" otherwise
//     — never omit the key, an empty string is the documented "unknown"
//     value so callers don't need an existence check).
//   - Cells lists every cell the move touches, for highlighting.
//   - Apply is the game-specific board mutation the UI should perform; see
//     cellsApply (Tango/Queens/Mini Sudoku), pathApply (Zip), and rectApply
//     (Patches) below.
type hintResultJSON struct {
	Done      bool            `json:"done"`
	Message   string          `json:"message"`
	Technique string          `json:"technique"`
	Cells     []cellJSON      `json:"cells"`
	Apply     json.RawMessage `json:"apply,omitempty"`
}

// cellWrite is one absolute-value cell mutation: set board.cells[row][col]
// to value. Tango/Queens/Mini Sudoku hints all reduce to a short list of
// these (Queens' "move the queen" is expressed as a clear (value 0) of the
// old cell followed by a set (value 1) of the new one).
type cellWrite struct {
	Row   int `json:"row"`
	Col   int `json:"col"`
	Value int `json:"value"`
}

// cellsApply is the Apply payload shape for Tango/Queens/Mini Sudoku hints:
// {"cells": [{"row":r,"col":c,"value":v}, ...]}.
type cellsApply struct {
	Cells []cellWrite `json:"cells"`
}

// marshalApply marshals a hint's Apply payload, falling back to a literal
// JSON null on the (unreachable in practice) event that v — always one of
// this file's own plain structs — somehow fails to marshal, so a hint
// response is never itself malformed.
func marshalApply(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

// adapters is populated by each per-game file's init().
var adapters = map[string]gameAdapter{}

func registerAdapter(a gameAdapter) {
	if _, dup := adapters[a.id()]; dup {
		panic(fmt.Sprintf("wasm: duplicate adapter id %q", a.id()))
	}
	adapters[a.id()] = a
}

func lookupAdapter(gameID string) (gameAdapter, error) {
	a, ok := adapters[gameID]
	if !ok {
		return nil, fmt.Errorf("unknown game %q", gameID)
	}
	return a, nil
}

// intGrid2D reshapes a row-major flat slice of length rows*cols into a
// rows x cols 2D slice, per the board JSON convention used throughout this
// bridge (see web/js/api.md).
func intGrid2D(flat []int, rows, cols int) [][]int {
	out := make([][]int, rows)
	for r := 0; r < rows; r++ {
		row := make([]int, cols)
		copy(row, flat[r*cols:(r+1)*cols])
		out[r] = row
	}
	return out
}

// flattenIntGrid is the inverse of intGrid2D: it reads a rows x cols 2D
// slice (as decoded from incoming board JSON) into a row-major flat slice,
// validating the declared dimensions.
func flattenIntGrid(grid [][]int, rows, cols int) ([]int, error) {
	if len(grid) != rows {
		return nil, fmt.Errorf("board has %d rows, want %d", len(grid), rows)
	}
	flat := make([]int, rows*cols)
	for r, row := range grid {
		if len(row) != cols {
			return nil, fmt.Errorf("board row %d has %d cols, want %d", r, len(row), cols)
		}
		copy(flat[r*cols:(r+1)*cols], row)
	}
	return flat, nil
}
