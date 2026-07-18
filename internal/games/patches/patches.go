// Package patches implements LinkedIn's rectangle-partition puzzle with shape constraints.
//
// See docs/plan/games/patches.md for the full spec: rules, data model,
// generation approach, solver approach, and the TDD test matrix this package
// is built against.
package patches

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sort"

	"github.com/Jensen95/tui-games/internal/engine"
)

// GameID is this game's identifier in the engine registry.
const GameID engine.GameID = "patches"

// Shape constrains the aspect ratio of a rectangle.
type Shape uint8

const (
	Square Shape = iota
	Wide         // width > height
	Tall         // height > width
	Free         // any aspect ratio
)

// String returns the display name of the shape.
func (s Shape) String() string {
	switch s {
	case Square:
		return "square"
	case Wide:
		return "wide"
	case Tall:
		return "tall"
	case Free:
		return "free"
	default:
		return "unknown"
	}
}

// Clue is a cell's clue: number and shape type.
type Clue struct {
	Number int
	Shape  Shape
}

// Puzzle is the patches game puzzle.
type Puzzle struct {
	R       int          // rows
	C       int          // columns
	Clues   map[int]Clue // anchor cell index -> clue
	SeedVal int64
	Diff    engine.Difficulty
}

// GameID implements engine.Puzzle.
func (p *Puzzle) GameID() engine.GameID {
	return GameID
}

// Difficulty implements engine.Puzzle.
func (p *Puzzle) Difficulty() engine.Difficulty {
	return p.Diff
}

// Seed implements engine.Puzzle.
func (p *Puzzle) Seed() int64 {
	return p.SeedVal
}

var _ engine.Puzzle = (*Puzzle)(nil)

// Rect is a rectangle defined by top-left position and dimensions.
type Rect struct {
	R0, C0 int // top-left (row, col)
	W, H   int // width and height
}

// Solution is a complete tiling of rectangles, one per clue.
type Solution struct {
	Rects []Rect
}

// Board represents the current state of the puzzle being solved.
type Board struct {
	P *Puzzle
	// Cells: which rectangle index each cell belongs to, or -1 if uncovered.
	Cells []int
}

// NewBoard creates a fresh, fully-uncovered board for a puzzle.
func NewBoard(p *Puzzle) *Board {
	cells := make([]int, p.R*p.C)
	for i := range cells {
		cells[i] = -1
	}
	return &Board{P: p, Cells: cells}
}

// sortedClueKeys returns a puzzle's clue anchor indices in ascending order,
// giving every algorithm here (solver, generator, fingerprinter) a
// deterministic iteration order over the otherwise-unordered Clues map.
func sortedClueKeys(clues map[int]Clue) []int {
	keys := make([]int, 0, len(clues))
	for k := range clues {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

// ---------------------------------------------------------------------------
// Encode / Decode (clues only — the solution never leaks into the encoding).
// ---------------------------------------------------------------------------

type wireClue struct {
	Idx    int `json:"idx"`
	Number int `json:"number"`
	Shape  int `json:"shape"`
}

type wirePuzzle struct {
	R     int        `json:"r"`
	C     int        `json:"c"`
	Clues []wireClue `json:"clues"` // sorted by idx
	Diff  int        `json:"diff"`
}

// Encode serializes a puzzle's clues to stable JSON. It never includes the
// solution.
func Encode(p *Puzzle) []byte {
	w := wirePuzzle{R: p.R, C: p.C, Diff: int(p.Diff)}
	for _, k := range sortedClueKeys(p.Clues) {
		c := p.Clues[k]
		w.Clues = append(w.Clues, wireClue{Idx: k, Number: c.Number, Shape: int(c.Shape)})
	}
	b, _ := json.Marshal(w)
	return b
}

// Decode reverses Encode.
func Decode(data []byte) (*Puzzle, error) {
	var w wirePuzzle
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	if w.R <= 0 || w.C <= 0 {
		return nil, fmt.Errorf("patches: bad dimensions %dx%d", w.R, w.C)
	}
	n := w.R * w.C
	p := &Puzzle{R: w.R, C: w.C, Clues: make(map[int]Clue, len(w.Clues)), Diff: engine.Difficulty(w.Diff)}
	for _, c := range w.Clues {
		if c.Idx < 0 || c.Idx >= n {
			return nil, fmt.Errorf("patches: clue index %d out of range", c.Idx)
		}
		p.Clues[c.Idx] = Clue{Number: c.Number, Shape: Shape(c.Shape)}
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Registry entry.
// ---------------------------------------------------------------------------

// Entry returns the engine registry entry for patches. The orchestrator
// wires it into internal/games/all; this package does not self-register.
func Entry() engine.Entry {
	return engine.Entry{
		ID:   GameID,
		Name: "Patches",
		Generate: func(diff engine.Difficulty, r *rand.Rand) (engine.Generated, error) {
			p, sol, err := NewGenerator().Generate(diff, r)
			if err != nil {
				return engine.Generated{}, err
			}
			fp := NewFingerprinter()
			return engine.Generated{
				Puzzle:      p,
				Solution:    sol,
				Encoded:     Encode(p),
				Fingerprint: fp.Fingerprint(p),
			}, nil
		},
		Verify: func(encoded []byte) error {
			return verifyEncoded(encoded)
		},
	}
}

// verifyEncoded decodes and independently re-checks the generation invariant:
// valid dimensions/clues, clue numbers summing to R*C, and exactly one
// solution per the complete solver.
func verifyEncoded(encoded []byte) error {
	p, err := Decode(encoded)
	if err != nil {
		return err
	}
	if p.R <= 0 || p.C <= 0 {
		return fmt.Errorf("patches: invalid dimensions %dx%d", p.R, p.C)
	}
	if len(p.Clues) == 0 {
		return fmt.Errorf("patches: no clues")
	}
	sum := 0
	for idx, c := range p.Clues {
		if idx < 0 || idx >= p.R*p.C {
			return fmt.Errorf("patches: clue index %d out of range", idx)
		}
		if c.Number <= 0 {
			return fmt.Errorf("patches: clue %d has non-positive number %d", idx, c.Number)
		}
		sum += c.Number
	}
	if sum != p.R*p.C {
		return fmt.Errorf("patches: clue numbers sum to %d, want %d", sum, p.R*p.C)
	}
	solver := NewSolver(p)
	if count := solver.CountSolutions(p, 2); count != 1 {
		return fmt.Errorf("patches: puzzle has %d solutions (want exactly 1)", count)
	}
	return nil
}
