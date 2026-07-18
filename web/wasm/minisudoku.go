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
