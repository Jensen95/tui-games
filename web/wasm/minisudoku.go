//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	"github.com/Jensen95/tui-games/internal/games/minisudoku"
)

func init() { registerAdapter(minisudokuAdapter{}) }

type minisudokuAdapter struct{}

func (minisudokuAdapter) id() string   { return string(minisudoku.GameID) }
func (minisudokuAdapter) name() string { return "Mini Sudoku" }

// minisudokuBoardWire is the UI-facing board contract for Mini Sudoku. See
// web/js/api.md "Mini Sudoku" for the full documentation of every field.
type minisudokuBoardWire struct {
	Rows    int      `json:"rows"`
	Cols    int      `json:"cols"`
	BoxRows int      `json:"boxRows"`
	BoxCols int      `json:"boxCols"`
	Cells   [][]int  `json:"cells"`
	Givens  [][]bool `json:"givens"`
}

// minisudokuSolutionWire is the shape of the "solution" JSON returned by
// generate(): the fully solved grid, same cell encoding as the board.
type minisudokuSolutionWire struct {
	Cells [][]int `json:"cells"`
}

// minisudokuBoardIn is the minimal shape read back from the UI on
// violations()/solved() calls: only Cells is consulted.
type minisudokuBoardIn struct {
	Cells [][]int `json:"cells"`
}

func minisudokuGivensGrid(givens map[int]int, n int) [][]bool {
	out := make([][]bool, n)
	for r := range out {
		out[r] = make([]bool, n)
	}
	for idx := range givens {
		c := engine.CellAt(idx, n)
		out[c.Row][c.Col] = true
	}
	return out
}

func (minisudokuAdapter) generate(diff engine.Difficulty, r *rand.Rand) (gameResult, error) {
	gen := minisudoku.Generator{}
	p, sol, err := gen.Generate(diff, r)
	if err != nil {
		return gameResult{}, fmt.Errorf("minisudoku: generate: %w", err)
	}
	encoded, err := minisudoku.Encode(p)
	if err != nil {
		return gameResult{}, fmt.Errorf("minisudoku: encode: %w", err)
	}

	n := p.N
	initCells := make([]int, n*n)
	for idx, val := range p.Givens {
		initCells[idx] = val
	}
	board := minisudokuBoardWire{
		Rows:    n,
		Cols:    n,
		BoxRows: p.BoxH,
		BoxCols: p.BoxW,
		Cells:   intGrid2D(initCells, n, n),
		Givens:  minisudokuGivensGrid(p.Givens, n),
	}
	boardJSON, err := json.Marshal(board)
	if err != nil {
		return gameResult{}, fmt.Errorf("minisudoku: marshal board: %w", err)
	}

	solWire := minisudokuSolutionWire{Cells: intGrid2D(sol.Cells, n, n)}
	solJSON, err := json.Marshal(solWire)
	if err != nil {
		return gameResult{}, fmt.Errorf("minisudoku: marshal solution: %w", err)
	}
	return gameResult{Puzzle: json.RawMessage(encoded), Solution: solJSON, Board: boardJSON}, nil
}

func (minisudokuAdapter) decode(puzzleJSON, boardJSON []byte) (minisudoku.Puzzle, minisudoku.Board, error) {
	p, err := minisudoku.Decode(puzzleJSON)
	if err != nil {
		return minisudoku.Puzzle{}, minisudoku.Board{}, fmt.Errorf("minisudoku: decode puzzle: %w", err)
	}
	var in minisudokuBoardIn
	if err := json.Unmarshal(boardJSON, &in); err != nil {
		return minisudoku.Puzzle{}, minisudoku.Board{}, fmt.Errorf("minisudoku: decode board: %w", err)
	}
	flat, ferr := flattenIntGrid(in.Cells, p.N, p.N)
	if ferr != nil {
		return minisudoku.Puzzle{}, minisudoku.Board{}, fmt.Errorf("minisudoku: decode board: %w", ferr)
	}
	return p, minisudoku.Board{Cells: flat}, nil
}

func (a minisudokuAdapter) violations(puzzleJSON, boardJSON []byte) ([]violationJSON, error) {
	_, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return nil, err
	}
	return violationsToJSON(minisudoku.Validator{}.Violations(b)), nil
}

func (a minisudokuAdapter) solved(puzzleJSON, boardJSON []byte) (bool, error) {
	_, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return false, err
	}
	return minisudoku.Validator{}.Solved(b), nil
}

// minisudokuHintFallbackTechnique labels a hint reveal that no pure-logic
// technique (naked/hidden singles, naked/hidden pairs, pointing pairs) could
// derive on its own — i.e. the ladder solver stalled and the cell is filled
// straight from the recorded solution instead. Mirrors
// internal/tui/boards/minisudoku.go's constant of the same name/value.
const minisudokuHintFallbackTechnique engine.Technique = "solution"

// hint mirrors internal/tui/boards/minisudoku.go's Hint()/minisudokuNextHint:
// run the no-guessing ladder solver (Solver.LogicSolve) seeded with the
// player's current board as givens, and reveal the first empty cell it
// manages to derive, naming the deepest technique the ladder needed. If the
// ladder makes no further progress at all (every remaining cell needs a
// guess), fall back to the first empty cell, filled from the recorded
// solution with technique "solution". The revealed value always comes from
// the recorded solution (authoritative regardless of which path found the
// cell), exactly like the TUI.
func (a minisudokuAdapter) hint(puzzleJSON, boardJSON, solutionJSON []byte) (hintResultJSON, error) {
	p, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return hintResultJSON{}, err
	}
	var sol minisudokuSolutionWire
	if err := json.Unmarshal(solutionJSON, &sol); err != nil {
		return hintResultJSON{}, fmt.Errorf("minisudoku: decode solution: %w", err)
	}
	solFlat, ferr := flattenIntGrid(sol.Cells, p.N, p.N)
	if ferr != nil {
		return hintResultJSON{}, fmt.Errorf("minisudoku: decode solution: %w", ferr)
	}

	givens := make(map[int]int, len(b.Cells))
	for i, v := range b.Cells {
		if v != 0 {
			givens[i] = v
		}
	}
	temp := minisudoku.Puzzle{N: p.N, BoxH: p.BoxH, BoxW: p.BoxW, Givens: givens}
	ladderSol, _, tech := minisudoku.Solver{}.LogicSolve(temp)

	idx, technique := -1, engine.Technique("")
	for i, v := range b.Cells {
		if v == 0 && i < len(ladderSol.Cells) && ladderSol.Cells[i] != 0 {
			idx, technique = i, tech
			break
		}
	}
	if idx < 0 {
		for i, v := range b.Cells {
			if v == 0 {
				idx, technique = i, minisudokuHintFallbackTechnique
				break
			}
		}
	}
	if idx < 0 {
		return hintResultJSON{Done: true, Message: "board is already full"}, nil
	}

	cell := engine.CellAt(idx, p.N)
	val := solFlat[idx]
	msg := fmt.Sprintf("hint: r%dc%d = %d (%s)", cell.Row+1, cell.Col+1, val, technique)
	return hintResultJSON{
		Message:   msg,
		Technique: string(technique),
		Cells:     []cellJSON{{Row: cell.Row, Col: cell.Col}},
		Apply:     marshalApply(cellsApply{Cells: []cellWrite{{Row: cell.Row, Col: cell.Col, Value: val}}}),
	}, nil
}
