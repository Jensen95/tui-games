//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	"github.com/Jensen95/tui-games/internal/games/zip"
)

func init() { registerAdapter(zipAdapter{}) }

type zipAdapter struct{}

func (zipAdapter) id() string   { return string(zip.ID) }
func (zipAdapter) name() string { return "Zip" }

// zipBoardWire is the UI-facing board contract for Zip. See web/js/api.md
// "Zip" for the full documentation of every field. Walls are split into
// hWalls/vWalls (analogous to Tango's hEdges/vEdges) rather than the single
// generic "walls" list suggested in the task brief, to keep every game's
// edge data in the same row/col-indexed grid shape.
type zipBoardWire struct {
	Rows      int        `json:"rows"`
	Cols      int        `json:"cols"`
	Waypoints [][]int    `json:"waypoints"`
	HWalls    [][]bool   `json:"hWalls"`
	VWalls    [][]bool   `json:"vWalls"`
	Path      []cellJSON `json:"path"`
}

// zipSolutionWire is the shape of the "solution" JSON returned by
// generate(): the full Hamiltonian path in visiting order, same encoding as
// the board's Path.
type zipSolutionWire struct {
	Path []cellJSON `json:"path"`
}

// zipBoardIn is the minimal shape read back from the UI on
// violations()/solved() calls: only Path is consulted. Waypoints and walls
// are sourced from the decoded puzzle, never trusted from board JSON,
// because they are immutable puzzle data — see api.md.
type zipBoardIn struct {
	Path []cellJSON `json:"path"`
}

// zipWaypointsGrid reshapes the puzzle's waypoint map into a rows x cols
// grid; 0 means "no waypoint on this cell".
func zipWaypointsGrid(p zip.Puzzle) [][]int {
	out := make([][]int, p.R)
	for r := range out {
		out[r] = make([]int, p.C)
	}
	for idx, num := range p.Waypoint {
		c := engine.CellAt(idx, p.C)
		out[c.Row][c.Col] = num
	}
	return out
}

// zipHWallsGrid reshapes the puzzle's wall set into a rows x (cols-1) grid:
// entry [r][c] reports whether the edge between cells (r,c)-(r,c+1) is
// walled.
func zipHWallsGrid(p zip.Puzzle) [][]bool {
	out := make([][]bool, p.R)
	for r := 0; r < p.R; r++ {
		out[r] = make([]bool, p.C-1)
		for c := 0; c < p.C-1; c++ {
			a := r*p.C + c
			out[r][c] = p.Walls[zip.WallKey(a, a+1)]
		}
	}
	return out
}

// zipVWallsGrid reshapes the puzzle's wall set into a (rows-1) x cols grid:
// entry [r][c] reports whether the edge between cells (r,c)-(r+1,c) is
// walled.
func zipVWallsGrid(p zip.Puzzle) [][]bool {
	out := make([][]bool, 0)
	if p.R == 0 {
		return out
	}
	out = make([][]bool, p.R-1)
	for r := 0; r < p.R-1; r++ {
		out[r] = make([]bool, p.C)
		for c := 0; c < p.C; c++ {
			a := r*p.C + c
			out[r][c] = p.Walls[zip.WallKey(a, a+p.C)]
		}
	}
	return out
}

func zipPathToCells(path []int, cols int) []cellJSON {
	out := make([]cellJSON, len(path))
	for i, idx := range path {
		c := engine.CellAt(idx, cols)
		out[i] = cellJSON{Row: c.Row, Col: c.Col}
	}
	return out
}

func zipCellsToPath(cells []cellJSON, cols int) []int {
	out := make([]int, len(cells))
	for i, c := range cells {
		out[i] = c.Row*cols + c.Col
	}
	return out
}

func (zipAdapter) generate(diff engine.Difficulty, r *rand.Rand) (gameResult, error) {
	gen := zip.Generator{}
	p, sol, err := gen.Generate(diff, r)
	if err != nil {
		return gameResult{}, fmt.Errorf("zip: generate: %w", err)
	}
	encoded := zip.Encode(p)

	board := zipBoardWire{
		Rows:      p.R,
		Cols:      p.C,
		Waypoints: zipWaypointsGrid(p),
		HWalls:    zipHWallsGrid(p),
		VWalls:    zipVWallsGrid(p),
		Path:      []cellJSON{},
	}
	boardJSON, err := json.Marshal(board)
	if err != nil {
		return gameResult{}, fmt.Errorf("zip: marshal board: %w", err)
	}

	solWire := zipSolutionWire{Path: zipPathToCells(sol.Path, p.C)}
	solJSON, err := json.Marshal(solWire)
	if err != nil {
		return gameResult{}, fmt.Errorf("zip: marshal solution: %w", err)
	}
	return gameResult{Puzzle: json.RawMessage(encoded), Solution: solJSON, Board: boardJSON}, nil
}

func (zipAdapter) decode(puzzleJSON, boardJSON []byte) (zip.Puzzle, zip.Board, error) {
	p, err := zip.Decode(puzzleJSON)
	if err != nil {
		return zip.Puzzle{}, zip.Board{}, fmt.Errorf("zip: decode puzzle: %w", err)
	}
	var in zipBoardIn
	if err := json.Unmarshal(boardJSON, &in); err != nil {
		return zip.Puzzle{}, zip.Board{}, fmt.Errorf("zip: decode board: %w", err)
	}
	path := zipCellsToPath(in.Path, p.C)
	n := p.R * p.C
	for _, idx := range path {
		if idx < 0 || idx >= n {
			return zip.Puzzle{}, zip.Board{}, fmt.Errorf("zip: path cell index %d out of range 0..%d", idx, n-1)
		}
	}
	return p, zip.Board{Puzzle: p, Path: path}, nil
}

func (a zipAdapter) violations(puzzleJSON, boardJSON []byte) ([]violationJSON, error) {
	_, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return nil, err
	}
	return violationsToJSON(zip.Validator{}.Violations(b)), nil
}

func (a zipAdapter) solved(puzzleJSON, boardJSON []byte) (bool, error) {
	_, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return false, err
	}
	return zip.Validator{}.Solved(b), nil
}
