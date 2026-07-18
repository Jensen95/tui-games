//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	"github.com/Jensen95/tui-games/internal/games/tango"
)

func init() { registerAdapter(tangoAdapter{}) }

type tangoAdapter struct{}

func (tangoAdapter) id() string   { return string(tango.GameID) }
func (tangoAdapter) name() string { return "Tango" }

// tangoBoardWire is the UI-facing board contract for Tango. See
// web/js/api.md "Tango" for the full documentation of every field.
type tangoBoardWire struct {
	Rows   int      `json:"rows"`
	Cols   int      `json:"cols"`
	Cells  [][]int  `json:"cells"`
	Givens [][]bool `json:"givens"`
	HEdges [][]int  `json:"hEdges"`
	VEdges [][]int  `json:"vEdges"`
}

// tangoSolutionWire is the shape of the "solution" JSON returned by
// generate(): the fully solved grid, same cell encoding as the board.
type tangoSolutionWire struct {
	Cells [][]int `json:"cells"`
}

// tangoBoardIn is the minimal shape read back from the UI on
// violations()/solved() calls: only Cells is consulted. Edge constraints
// and dimensions are sourced from the decoded puzzle, never trusted from
// board JSON, because they are immutable puzzle data — see api.md.
type tangoBoardIn struct {
	Cells [][]int `json:"cells"`
}

func tangoCellsGrid(cells []tango.Symbol, n int) [][]int {
	out := make([][]int, n)
	for r := 0; r < n; r++ {
		row := make([]int, n)
		for c := 0; c < n; c++ {
			row[c] = int(cells[r*n+c])
		}
		out[r] = row
	}
	return out
}

func tangoGivensGrid(givens map[int]tango.Symbol, n int) [][]bool {
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

// tangoHEdgesGrid reshapes the puzzle's horizontal-edge map into a rows x
// (cols-1) grid: entry [r][c] is the relation between cells (r,c)-(r,c+1).
func tangoHEdgesGrid(m map[[2]int]tango.Relation, n int) [][]int {
	out := make([][]int, n)
	for r := range out {
		out[r] = make([]int, n-1)
	}
	for pair, rel := range m {
		ca, cb := engine.CellAt(pair[0], n), engine.CellAt(pair[1], n)
		col := ca.Col
		if cb.Col < col {
			col = cb.Col
		}
		out[ca.Row][col] = int(rel)
	}
	return out
}

// tangoVEdgesGrid reshapes the puzzle's vertical-edge map into a (rows-1) x
// cols grid: entry [r][c] is the relation between cells (r,c)-(r+1,c).
func tangoVEdgesGrid(m map[[2]int]tango.Relation, n int) [][]int {
	out := make([][]int, n-1)
	for r := range out {
		out[r] = make([]int, n)
	}
	for pair, rel := range m {
		ca, cb := engine.CellAt(pair[0], n), engine.CellAt(pair[1], n)
		row := ca.Row
		if cb.Row < row {
			row = cb.Row
		}
		out[row][ca.Col] = int(rel)
	}
	return out
}

func (tangoAdapter) generate(diff engine.Difficulty, r *rand.Rand) (gameResult, error) {
	gen := tango.Generator{}
	p, solBoard, err := gen.Generate(diff, r)
	if err != nil {
		return gameResult{}, fmt.Errorf("tango: generate: %w", err)
	}
	encoded, err := tango.Encode(p)
	if err != nil {
		return gameResult{}, fmt.Errorf("tango: encode: %w", err)
	}

	n := p.N
	initCells := make([]tango.Symbol, n*n)
	for idx, sym := range p.Givens {
		initCells[idx] = sym
	}
	board := tangoBoardWire{
		Rows:   n,
		Cols:   n,
		Cells:  tangoCellsGrid(initCells, n),
		Givens: tangoGivensGrid(p.Givens, n),
		HEdges: tangoHEdgesGrid(p.HEdges, n),
		VEdges: tangoVEdgesGrid(p.VEdges, n),
	}
	boardJSON, err := json.Marshal(board)
	if err != nil {
		return gameResult{}, fmt.Errorf("tango: marshal board: %w", err)
	}
	sol := tangoSolutionWire{Cells: tangoCellsGrid(solBoard.Cells, n)}
	solJSON, err := json.Marshal(sol)
	if err != nil {
		return gameResult{}, fmt.Errorf("tango: marshal solution: %w", err)
	}
	return gameResult{Puzzle: json.RawMessage(encoded), Solution: solJSON, Board: boardJSON}, nil
}

func (tangoAdapter) decode(puzzleJSON, boardJSON []byte) (tango.Puzzle, tango.Board, error) {
	p, err := tango.Decode(puzzleJSON)
	if err != nil {
		return tango.Puzzle{}, tango.Board{}, fmt.Errorf("tango: decode puzzle: %w", err)
	}
	var in tangoBoardIn
	if err := json.Unmarshal(boardJSON, &in); err != nil {
		return tango.Puzzle{}, tango.Board{}, fmt.Errorf("tango: decode board: %w", err)
	}
	flat := make([]tango.Symbol, p.N*p.N)
	if len(in.Cells) != p.N {
		return tango.Puzzle{}, tango.Board{}, fmt.Errorf("tango: board has %d rows, want %d", len(in.Cells), p.N)
	}
	for r, row := range in.Cells {
		if len(row) != p.N {
			return tango.Puzzle{}, tango.Board{}, fmt.Errorf("tango: board row %d has %d cols, want %d", r, len(row), p.N)
		}
		for c, v := range row {
			flat[r*p.N+c] = tango.Symbol(v)
		}
	}
	b := tango.Board{N: p.N, Cells: flat, HEdges: p.HEdges, VEdges: p.VEdges}
	return p, b, nil
}

func (a tangoAdapter) violations(puzzleJSON, boardJSON []byte) ([]violationJSON, error) {
	_, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return nil, err
	}
	return violationsToJSON(tango.Validator{}.Violations(b)), nil
}

func (a tangoAdapter) solved(puzzleJSON, boardJSON []byte) (bool, error) {
	_, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return false, err
	}
	return tango.Validator{}.Solved(b), nil
}
