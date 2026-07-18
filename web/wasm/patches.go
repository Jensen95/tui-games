//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sort"

	"github.com/Jensen95/tui-games/internal/engine"
	"github.com/Jensen95/tui-games/internal/games/patches"
)

func init() { registerAdapter(patchesAdapter{}) }

type patchesAdapter struct{}

func (patchesAdapter) id() string   { return string(patches.GameID) }
func (patchesAdapter) name() string { return "Patches" }

// patchesClueWire is the wire shape of one clue cell: its position, target
// area, and shape constraint name ("square"|"wide"|"tall"|"free").
type patchesClueWire struct {
	Row   int    `json:"row"`
	Col   int    `json:"col"`
	Area  int    `json:"area"`
	Shape string `json:"shape"`
}

// patchesBoardWire is the UI-facing board contract for Patches. See
// web/js/api.md "Patches" for the full documentation of every field.
type patchesBoardWire struct {
	Rows   int               `json:"rows"`
	Cols   int               `json:"cols"`
	Clues  []patchesClueWire `json:"clues"`
	Labels [][]int           `json:"labels"`
}

// patchesSolutionWire is the shape of the "solution" JSON returned by
// generate(): the fully tiled grid, same label encoding as the board
// (labels are an internal, solver-order index — see api.md — not the
// board's arbitrary UI-assigned labels).
type patchesSolutionWire struct {
	Labels [][]int `json:"labels"`
}

// patchesBoardIn is the minimal shape read back from the UI on
// violations()/solved() calls: only Labels is consulted. Clues are sourced
// from the decoded puzzle, never trusted from board JSON, because they are
// immutable puzzle data — see api.md.
type patchesBoardIn struct {
	Labels [][]int `json:"labels"`
}

func patchesCluesWire(p *patches.Puzzle) []patchesClueWire {
	keys := make([]int, 0, len(p.Clues))
	for k := range p.Clues {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	out := make([]patchesClueWire, 0, len(keys))
	for _, idx := range keys {
		c := p.Clues[idx]
		cell := engine.CellAt(idx, p.C)
		out = append(out, patchesClueWire{Row: cell.Row, Col: cell.Col, Area: c.Number, Shape: c.Shape.String()})
	}
	return out
}

// patchesSolutionLabels rebuilds a full rows x cols label grid from the
// solution's rectangle list: every cell inside Rects[i] gets label i.
func patchesSolutionLabels(p *patches.Puzzle, sol *patches.Solution) [][]int {
	cells := make([]int, p.R*p.C)
	for i := range cells {
		cells[i] = -1
	}
	for i, rect := range sol.Rects {
		for row := rect.R0; row < rect.R0+rect.H; row++ {
			for col := rect.C0; col < rect.C0+rect.W; col++ {
				cells[row*p.C+col] = i
			}
		}
	}
	return intGrid2D(cells, p.R, p.C)
}

func (patchesAdapter) generate(diff engine.Difficulty, r *rand.Rand) (gameResult, error) {
	gen := patches.NewGenerator()
	p, sol, err := gen.Generate(diff, r)
	if err != nil {
		return gameResult{}, fmt.Errorf("patches: generate: %w", err)
	}
	encoded := patches.Encode(p)

	initial := patches.NewBoard(p)
	board := patchesBoardWire{
		Rows:   p.R,
		Cols:   p.C,
		Clues:  patchesCluesWire(p),
		Labels: intGrid2D(initial.Cells, p.R, p.C),
	}
	boardJSON, err := json.Marshal(board)
	if err != nil {
		return gameResult{}, fmt.Errorf("patches: marshal board: %w", err)
	}

	solWire := patchesSolutionWire{Labels: patchesSolutionLabels(p, sol)}
	solJSON, err := json.Marshal(solWire)
	if err != nil {
		return gameResult{}, fmt.Errorf("patches: marshal solution: %w", err)
	}
	return gameResult{Puzzle: json.RawMessage(encoded), Solution: solJSON, Board: boardJSON}, nil
}

func (patchesAdapter) decode(puzzleJSON, boardJSON []byte) (*patches.Puzzle, *patches.Board, error) {
	p, err := patches.Decode(puzzleJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("patches: decode puzzle: %w", err)
	}
	var in patchesBoardIn
	if err := json.Unmarshal(boardJSON, &in); err != nil {
		return nil, nil, fmt.Errorf("patches: decode board: %w", err)
	}
	flat, ferr := flattenIntGrid(in.Labels, p.R, p.C)
	if ferr != nil {
		return nil, nil, fmt.Errorf("patches: decode board: %w", ferr)
	}
	return p, &patches.Board{P: p, Cells: flat}, nil
}

func (a patchesAdapter) violations(puzzleJSON, boardJSON []byte) ([]violationJSON, error) {
	p, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return nil, err
	}
	return violationsToJSON(patches.NewValidator(p).Violations(b)), nil
}

func (a patchesAdapter) solved(puzzleJSON, boardJSON []byte) (bool, error) {
	p, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return false, err
	}
	return patches.NewValidator(p).Solved(b), nil
}
