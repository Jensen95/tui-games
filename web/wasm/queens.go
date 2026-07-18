//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	"github.com/Jensen95/tui-games/internal/games/queens"
)

func init() { registerAdapter(queensAdapter{}) }

type queensAdapter struct{}

func (queensAdapter) id() string   { return string(queens.GameID) }
func (queensAdapter) name() string { return "Queens" }

// queensBoardWire is the UI-facing board contract for Queens. See
// web/js/api.md "Queens" for the full documentation of every field. Note
// that the engine's board model only ever tracks Empty/Queen — the player's
// "X" mark is a TUI/UI-only annotation with no engine representation (see
// internal/games/queens/queens.go's Cell doc comment) and is never sent
// here.
type queensBoardWire struct {
	N       int      `json:"n"`
	Regions [][]int  `json:"regions"`
	Cells   [][]int  `json:"cells"`
	Givens  [][]bool `json:"givens"`
}

// queensSolutionWire is the shape of the "solution" JSON returned by
// generate(): the fully solved grid, same cell encoding as the board.
type queensSolutionWire struct {
	Cells [][]int `json:"cells"`
}

// queensBoardIn is the minimal shape read back from the UI on
// violations()/solved() calls: only Cells is consulted. The region coloring
// is sourced from the decoded puzzle, never trusted from board JSON,
// because it is immutable puzzle data — see api.md.
type queensBoardIn struct {
	Cells [][]int `json:"cells"`
}

func queensCellsGrid(cells []queens.Cell, n int) [][]int {
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

func queensGivensGrid(givens []int, n int) [][]bool {
	out := make([][]bool, n)
	for r := range out {
		out[r] = make([]bool, n)
	}
	for _, idx := range givens {
		c := engine.CellAt(idx, n)
		out[c.Row][c.Col] = true
	}
	return out
}

func (queensAdapter) generate(diff engine.Difficulty, r *rand.Rand) (gameResult, error) {
	gen := queens.NewGenerator()
	p, sol, err := gen.Generate(diff, r)
	if err != nil {
		return gameResult{}, fmt.Errorf("queens: generate: %w", err)
	}
	encoded, err := queens.Encode(p)
	if err != nil {
		return gameResult{}, fmt.Errorf("queens: encode: %w", err)
	}

	n := p.N
	initCells := make([]queens.Cell, n*n)
	for _, idx := range p.Givens {
		initCells[idx] = queens.Queen
	}
	board := queensBoardWire{
		N:       n,
		Regions: intGrid2D(p.Region, n, n),
		Cells:   queensCellsGrid(initCells, n),
		Givens:  queensGivensGrid(p.Givens, n),
	}
	boardJSON, err := json.Marshal(board)
	if err != nil {
		return gameResult{}, fmt.Errorf("queens: marshal board: %w", err)
	}

	solCells := make([]queens.Cell, n*n)
	for row, col := range sol.QueenAt {
		solCells[row*n+col] = queens.Queen
	}
	solWire := queensSolutionWire{Cells: queensCellsGrid(solCells, n)}
	solJSON, err := json.Marshal(solWire)
	if err != nil {
		return gameResult{}, fmt.Errorf("queens: marshal solution: %w", err)
	}
	return gameResult{Puzzle: json.RawMessage(encoded), Solution: solJSON, Board: boardJSON}, nil
}

func (queensAdapter) decode(puzzleJSON, boardJSON []byte) (queens.Puzzle, queens.Board, error) {
	p, err := queens.Decode(puzzleJSON)
	if err != nil {
		return queens.Puzzle{}, queens.Board{}, fmt.Errorf("queens: decode puzzle: %w", err)
	}
	var in queensBoardIn
	if err := json.Unmarshal(boardJSON, &in); err != nil {
		return queens.Puzzle{}, queens.Board{}, fmt.Errorf("queens: decode board: %w", err)
	}
	if len(in.Cells) != p.N {
		return queens.Puzzle{}, queens.Board{}, fmt.Errorf("queens: board has %d rows, want %d", len(in.Cells), p.N)
	}
	flat := make([]queens.Cell, p.N*p.N)
	for r, row := range in.Cells {
		if len(row) != p.N {
			return queens.Puzzle{}, queens.Board{}, fmt.Errorf("queens: board row %d has %d cols, want %d", r, len(row), p.N)
		}
		for c, v := range row {
			flat[r*p.N+c] = queens.Cell(v)
		}
	}
	b := queens.Board{N: p.N, Region: p.Region, Cells: flat}
	return p, b, nil
}

func (a queensAdapter) violations(puzzleJSON, boardJSON []byte) ([]violationJSON, error) {
	_, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return nil, err
	}
	return violationsToJSON(queens.NewValidator().Violations(b)), nil
}

func (a queensAdapter) solved(puzzleJSON, boardJSON []byte) (bool, error) {
	_, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return false, err
	}
	return queens.NewValidator().Solved(b), nil
}

// hint mirrors internal/tui/boards/queens.go's Hint(): scanning rows in
// order, the first row whose queen doesn't match the recorded solution gets
// fixed — clearing any (non-given) queen already in that row, then placing
// the queen (skipping either half that would touch a given cell).
func (a queensAdapter) hint(puzzleJSON, boardJSON, solutionJSON []byte) (hintResultJSON, error) {
	p, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return hintResultJSON{}, err
	}
	var sol queensSolutionWire
	if err := json.Unmarshal(solutionJSON, &sol); err != nil {
		return hintResultJSON{}, fmt.Errorf("queens: decode solution: %w", err)
	}
	n := p.N
	if len(sol.Cells) != n {
		return hintResultJSON{}, fmt.Errorf("queens: solution has %d rows, want %d", len(sol.Cells), n)
	}

	given := make(map[int]bool, len(p.Givens))
	for _, idx := range p.Givens {
		given[idx] = true
	}

	for row := 0; row < n; row++ {
		if len(sol.Cells[row]) != n {
			return hintResultJSON{}, fmt.Errorf("queens: solution row %d has %d cols, want %d", row, len(sol.Cells[row]), n)
		}
		wantCol := -1
		for c := 0; c < n; c++ {
			if sol.Cells[row][c] == int(queens.Queen) {
				wantCol = c
				break
			}
		}
		if wantCol < 0 {
			continue // malformed solution row -- skip rather than fail the whole hint
		}

		curCol := -1
		for c := 0; c < n; c++ {
			if b.Cells[row*n+c] == queens.Queen {
				curCol = c
				break
			}
		}
		if curCol == wantCol {
			continue
		}

		var writes []cellWrite
		cellsHi := make([]cellJSON, 0, 2)
		if curCol != -1 && !given[row*n+curCol] {
			writes = append(writes, cellWrite{Row: row, Col: curCol, Value: int(queens.Empty)})
			cellsHi = append(cellsHi, cellJSON{Row: row, Col: curCol})
		}
		if !given[row*n+wantCol] {
			writes = append(writes, cellWrite{Row: row, Col: wantCol, Value: int(queens.Queen)})
		}
		cellsHi = append(cellsHi, cellJSON{Row: row, Col: wantCol})

		return hintResultJSON{
			Message: fmt.Sprintf("hint: queen at r%dc%d", row+1, wantCol+1),
			Cells:   cellsHi,
			Apply:   marshalApply(cellsApply{Cells: writes}),
		}, nil
	}
	return hintResultJSON{Done: true, Message: "already solved"}, nil
}
