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

// rectApply is the Apply payload shape for Patches hints: the bounding box
// (inclusive on both ends, exactly like patches.Rect) of one solution
// rectangle to reveal. The UI clears any rectangle currently overlapping
// these cells in full, then plants a fresh label across the whole box —
// mirroring internal/tui/boards/patches.go's applyHintRect.
type rectApply struct {
	R0 int `json:"r0"`
	C0 int `json:"c0"`
	R1 int `json:"r1"`
	C1 int `json:"c1"`
}

// patchesHintRect is a solution rectangle's bounding box (inclusive), as
// reconstructed from the solution's label grid (see hint below) rather than
// patches.Solution.Rects directly — the wire "solution" JSON for Patches is
// the label grid (patchesSolutionWire), not the solver's Rect list, per
// api.md.
type patchesHintRect struct {
	r0, c0, r1, c1 int
}

// patchesClueForRectBounds returns the anchor-cell index of the one clue
// rect (given as an inclusive bounding box) contains, per the generation
// invariant that every solution rectangle contains exactly one clue. Mirrors
// internal/tui/boards/patches.go's patchesClueForRect.
func patchesClueForRectBounds(p *patches.Puzzle, rect patchesHintRect) (int, bool) {
	for r := rect.r0; r <= rect.r1; r++ {
		for c := rect.c0; c <= rect.c1; c++ {
			idx := r*p.C + c
			if _, ok := p.Clues[idx]; ok {
				return idx, true
			}
		}
	}
	return 0, false
}

// patchesRectMatchesBoard reports whether rect is already exactly reflected
// on the board: every one of its cells shares a single label, and that label
// doesn't leak outside rect's bounds. Mirrors
// internal/tui/boards/patches.go's rectMatchesBoard.
func patchesRectMatchesBoard(b *patches.Board, cols int, rect patchesHintRect) bool {
	label := -2
	for r := rect.r0; r <= rect.r1; r++ {
		for c := rect.c0; c <= rect.c1; c++ {
			l := b.Cells[r*cols+c]
			if label == -2 {
				label = l
			} else if l != label {
				return false
			}
		}
	}
	if label < 0 {
		return false
	}
	for i, l := range b.Cells {
		if l != label {
			continue
		}
		cell := engine.CellAt(i, cols)
		if cell.Row < rect.r0 || cell.Row > rect.r1 || cell.Col < rect.c0 || cell.Col > rect.c1 {
			return false
		}
	}
	return true
}

// hint mirrors internal/tui/boards/patches.go's Hint(): walk the recorded
// solution's rectangles (in clue-index order, for a stable/deterministic
// reveal order) and reveal the first one not yet exactly placed on the
// board. Solution rectangles are reconstructed from the solution's label
// grid (bounding box of each distinct label) since that's the wire shape
// api.md documents for Patches' "solution" — not patches.Solution.Rects
// directly.
func (a patchesAdapter) hint(puzzleJSON, boardJSON, solutionJSON []byte) (hintResultJSON, error) {
	p, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return hintResultJSON{}, err
	}
	var sol patchesSolutionWire
	if err := json.Unmarshal(solutionJSON, &sol); err != nil {
		return hintResultJSON{}, fmt.Errorf("patches: decode solution: %w", err)
	}
	solFlat, ferr := flattenIntGrid(sol.Labels, p.R, p.C)
	if ferr != nil {
		return hintResultJSON{}, fmt.Errorf("patches: decode solution: %w", ferr)
	}

	rects := map[int]*patchesHintRect{}
	for i, lbl := range solFlat {
		if lbl < 0 {
			continue
		}
		cell := engine.CellAt(i, p.C)
		rect, ok := rects[lbl]
		if !ok {
			rects[lbl] = &patchesHintRect{r0: cell.Row, c0: cell.Col, r1: cell.Row, c1: cell.Col}
			continue
		}
		if cell.Row < rect.r0 {
			rect.r0 = cell.Row
		}
		if cell.Row > rect.r1 {
			rect.r1 = cell.Row
		}
		if cell.Col < rect.c0 {
			rect.c0 = cell.Col
		}
		if cell.Col > rect.c1 {
			rect.c1 = cell.Col
		}
	}

	type candidate struct {
		clueIdx int
		rect    patchesHintRect
	}
	cands := make([]candidate, 0, len(rects))
	for _, rect := range rects {
		clueIdx, ok := patchesClueForRectBounds(p, *rect)
		if !ok {
			continue
		}
		cands = append(cands, candidate{clueIdx: clueIdx, rect: *rect})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].clueIdx < cands[j].clueIdx })

	for _, cand := range cands {
		if patchesRectMatchesBoard(b, p.C, cand.rect) {
			continue
		}
		rect := cand.rect
		cellsHi := make([]cellJSON, 0, (rect.r1-rect.r0+1)*(rect.c1-rect.c0+1))
		for r := rect.r0; r <= rect.r1; r++ {
			for c := rect.c0; c <= rect.c1; c++ {
				cellsHi = append(cellsHi, cellJSON{Row: r, Col: c})
			}
		}
		return hintResultJSON{
			Message: fmt.Sprintf("hint: rectangle r%dc%d..r%dc%d", rect.r0+1, rect.c0+1, rect.r1+1, rect.c1+1),
			Cells:   cellsHi,
			Apply:   marshalApply(rectApply{R0: rect.r0, C0: rect.c0, R1: rect.r1, C1: rect.c1}),
		}, nil
	}
	return hintResultJSON{Done: true, Message: "already solved"}, nil
}
