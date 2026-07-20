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

// pathApply is the Apply payload shape for Zip hints: the full replacement
// path (board.path round-trips as a whole array per api.md, never
// incrementally), extended by exactly one cell beyond the player's current
// path's longest prefix shared with the recorded solution.
type pathApply struct {
	Path []cellJSON `json:"path"`
}

// zipLegalUnvisitedNeighbors returns the cell indices reachable in one
// orthogonal step from head that the path has not visited yet and that no wall
// blocks — the moves the path could legally make next. Pure adjacency + wall
// geometry; not a rule the bridge re-implements.
func zipLegalUnvisitedNeighbors(p zip.Puzzle, head int, visited map[int]bool) []int {
	r, c := head/p.C, head%p.C
	cands := make([]int, 0, 4)
	if c > 0 {
		cands = append(cands, head-1)
	}
	if c < p.C-1 {
		cands = append(cands, head+1)
	}
	if r > 0 {
		cands = append(cands, head-p.C)
	}
	if r < p.R-1 {
		cands = append(cands, head+p.C)
	}
	out := make([]int, 0, len(cands))
	for _, nb := range cands {
		if visited[nb] {
			continue
		}
		if p.Walls[zip.WallKey(head, nb)] {
			continue
		}
		out = append(out, nb)
	}
	return out
}

// zipHintReason derives a truthful explanation for extending the path to
// next, given the prefix already agreed with the solution (solPath[:i]). It
// asserts only what it can verify from the puzzle geometry: the mandatory
// start on waypoint 1, an only-legal-move (the head has exactly one unvisited,
// unwalled neighbor), and/or the next waypoint in ascending order. When none
// of those pin the move (the choice follows from global reachability the
// bridge does not re-derive), it says so honestly.
func zipHintReason(p zip.Puzzle, solPath []int, i, next int) string {
	if i == 0 {
		return "every path must start on the cell numbered 1"
	}
	head := solPath[i-1]
	visited := make(map[int]bool, i)
	for _, idx := range solPath[:i] {
		visited[idx] = true
	}
	onlyMove := false
	if legal := zipLegalUnvisitedNeighbors(p, head, visited); len(legal) == 1 && legal[0] == next {
		onlyMove = true
	}
	wp, isWaypoint := p.Waypoint[next]

	switch {
	case onlyMove && isWaypoint:
		return fmt.Sprintf("the only unvisited cell the path can reach from its head without crossing a wall — and it is waypoint %d (visited in ascending order)", wp)
	case onlyMove:
		return "the only unvisited cell the path can reach from its head without crossing a wall"
	case isWaypoint:
		return fmt.Sprintf("it is waypoint %d, and waypoints must be reached in ascending order", wp)
	default:
		return "the next cell on the puzzle's unique path"
	}
}

// hint mirrors internal/tui/boards/zip.go's Hint(): find the longest prefix
// the player's path shares with the recorded solution path, then extend it
// by exactly the next solution cell. The move follows the recorded solution;
// on top of it, zipHintReason explains why that cell is the one to take next.
func (a zipAdapter) hint(puzzleJSON, boardJSON, solutionJSON []byte) (hintResultJSON, error) {
	p, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return hintResultJSON{}, err
	}
	var sol zipSolutionWire
	if err := json.Unmarshal(solutionJSON, &sol); err != nil {
		return hintResultJSON{}, fmt.Errorf("zip: decode solution: %w", err)
	}
	solPath := zipCellsToPath(sol.Path, p.C)
	if len(solPath) == 0 {
		return hintResultJSON{Done: true, Message: "no solution recorded"}, nil
	}

	i := 0
	for i < len(b.Path) && i < len(solPath) && b.Path[i] == solPath[i] {
		i++
	}
	if i >= len(solPath) {
		return hintResultJSON{Done: true, Message: "path is already complete"}, nil
	}

	next := solPath[i]
	newPath := append(append([]int(nil), solPath[:i]...), next)
	cell := engine.CellAt(next, p.C)
	return hintResultJSON{
		Message: fmt.Sprintf("hint: extend path to r%dc%d — %s", cell.Row+1, cell.Col+1, zipHintReason(p, solPath, i, next)),
		Cells:   []cellJSON{{Row: cell.Row, Col: cell.Col}},
		Apply:   marshalApply(pathApply{Path: zipPathToCells(newPath, p.C)}),
	}, nil
}
