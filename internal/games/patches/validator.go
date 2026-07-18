package patches

import (
	"sort"

	"github.com/Jensen95/tui-games/internal/engine"
)

// Rule identifiers used in engine.Violation.Rule. Stable and
// machine-checkable; tests assert on these exact strings.
const (
	// RuleExactCover fires when the board's cell-to-rectangle labeling
	// leaves a cell uncovered, or when the cells sharing a label don't form
	// a solid axis-aligned rectangle (which is how an "overlap" surfaces in
	// this representation: two rectangles fighting over the same cells
	// leaves at least one of them with a ragged, non-rectangular footprint).
	RuleExactCover = "exact-cover"
	// RuleOneClue fires when a rectangle contains zero or more than one clue.
	RuleOneClue = "one-clue"
	// RuleArea fires when a rectangle's cell count doesn't match its clue's number.
	RuleArea = "area"
	// RuleShape fires when a rectangle's dimensions don't satisfy its clue's shape.
	RuleShape = "shape"
)

// Validator checks if a board state is valid. See engine.Validator[*Board].
type Validator struct {
	P *Puzzle
}

// NewValidator creates a validator for a puzzle.
func NewValidator(p *Puzzle) *Validator {
	return &Validator{P: p}
}

var _ engine.Validator[*Board] = (*Validator)(nil)

// rectGroup accumulates the bounding box and cell count of every board cell
// sharing one rectangle label.
type rectGroup struct {
	minR, maxR, minC, maxC int
	count                  int
}

func (g *rectGroup) w() int    { return g.maxC - g.minC + 1 }
func (g *rectGroup) h() int    { return g.maxR - g.minR + 1 }
func (g *rectGroup) area() int { return g.w() * g.h() }

// isSolidRect reports whether this group's cells exactly fill their bounding
// box — the only way a partition (one label per cell) can fail this is if
// two "rectangles" are fighting over territory, which leaves at least one of
// them with a gap inside its own bounding box.
func (g *rectGroup) isSolidRect() bool { return g.count == g.area() }

// groupsFromBoard scans b.Cells and returns one rectGroup per distinct
// non-negative label, plus whether any cell is uncovered (label < 0).
func groupsFromBoard(b *Board) (map[int]*rectGroup, bool) {
	groups := make(map[int]*rectGroup)
	hasGap := false
	c := b.P.C
	for i, label := range b.Cells {
		if label < 0 {
			hasGap = true
			continue
		}
		row, col := i/c, i%c
		g, ok := groups[label]
		if !ok {
			g = &rectGroup{minR: row, maxR: row, minC: col, maxC: col}
			groups[label] = g
		} else {
			g.minR = min(g.minR, row)
			g.maxR = max(g.maxR, row)
			g.minC = min(g.minC, col)
			g.maxC = max(g.maxC, col)
		}
		g.count++
	}
	return groups, hasGap
}

// shapeOK reports whether a w×h rectangle satisfies a clue's shape constraint.
func shapeOK(shape Shape, w, h int) bool {
	switch shape {
	case Square:
		return w == h
	case Wide:
		return w > h
	case Tall:
		return h > w
	case Free:
		return true
	default:
		return false
	}
}

// cluesInRect returns the sorted anchor-cell indices of every clue whose cell
// lies within the given bounding box.
func cluesInRect(p *Puzzle, minR, maxR, minC, maxC int) []int {
	var out []int
	for idx := range p.Clues {
		r, c := idx/p.C, idx%p.C
		if r >= minR && r <= maxR && c >= minC && c <= maxC {
			out = append(out, idx)
		}
	}
	sort.Ints(out)
	return out
}

// Violations returns all currently-violated rules for board b. For a
// partial board it only reports rules that are already broken — a gap from
// cells simply not yet assigned is the one exception this game's spec calls
// out explicitly as a genuine violation (partial coverage never "solves"),
// but an in-progress board with well-formed placed rectangles and untouched
// remaining cells reports no violations for those untouched cells beyond the
// blanket "not fully covered" signal.
func (v *Validator) Violations(b *Board) []engine.Violation {
	var out []engine.Violation

	groups, hasGap := groupsFromBoard(b)
	if hasGap {
		out = append(out, engine.Violation{
			Rule:    RuleExactCover,
			Message: "some cells are not covered by any rectangle",
		})
	}

	labels := make([]int, 0, len(groups))
	for l := range groups {
		labels = append(labels, l)
	}
	sort.Ints(labels)

	for _, label := range labels {
		g := groups[label]
		if !g.isSolidRect() {
			out = append(out, engine.Violation{
				Rule:    RuleExactCover,
				Message: "a rectangle's cells do not form a solid axis-aligned rectangle (overlap or fragmentation)",
			})
			continue
		}

		clueCells := cluesInRect(b.P, g.minR, g.maxR, g.minC, g.maxC)
		if len(clueCells) != 1 {
			out = append(out, engine.Violation{
				Rule:    RuleOneClue,
				Message: "a rectangle does not contain exactly one clue",
			})
			continue
		}

		clueIdx := clueCells[0]
		clue := b.P.Clues[clueIdx]
		w, h := g.w(), g.h()
		cell := engine.CellAt(clueIdx, b.P.C)

		if w*h != clue.Number {
			out = append(out, engine.Violation{
				Rule:    RuleArea,
				Message: "a rectangle's area does not match its clue's number",
				Cells:   []engine.Cell{cell},
			})
		}
		if !shapeOK(clue.Shape, w, h) {
			out = append(out, engine.Violation{
				Rule:    RuleShape,
				Message: "a rectangle's shape does not match its clue's shape",
				Cells:   []engine.Cell{cell},
			})
		}
	}

	return out
}

// Solved reports whether b is a complete, valid tiling: every cell covered
// exactly once, every rectangle solid with exactly one clue, and every
// clue's area+shape satisfied.
func (v *Validator) Solved(b *Board) bool {
	if b.P == nil || len(b.Cells) != b.P.R*b.P.C {
		return false
	}
	for _, c := range b.Cells {
		if c < 0 {
			return false
		}
	}
	return len(v.Violations(b)) == 0
}
