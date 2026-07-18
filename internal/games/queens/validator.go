package queens

import "github.com/Jensen95/tui-games/internal/engine"

// Validator referees Board state. See engine.Validator[Board].
type Validator struct{}

// NewValidator returns a Queens board validator.
func NewValidator() *Validator { return &Validator{} }

var _ engine.Validator[Board] = (*Validator)(nil)

// queenCells returns the cells holding a queen on b, in row-major order.
func queenCells(b Board) []engine.Cell {
	out := make([]engine.Cell, 0, b.N)
	for i, c := range b.Cells {
		if c == Queen {
			out = append(out, engine.CellAt(i, b.N))
		}
	}
	return out
}

// adjacent reports whether two distinct cells are 8-neighbor (edge- or
// corner-) adjacent: chebyshev distance exactly 1. Queens far apart on a
// shared diagonal are NOT adjacent (the local-only diagonal rule).
func adjacent(a, b engine.Cell) bool {
	dr := a.Row - b.Row
	if dr < 0 {
		dr = -dr
	}
	dc := a.Col - b.Col
	if dc < 0 {
		dc = -dc
	}
	if dr == 0 && dc == 0 {
		return false
	}
	return dr <= 1 && dc <= 1
}

// Violations returns every currently-broken rule on b. For a partial board
// (fewer than N queens placed) it reports only already-violated rules, never
// "missing" placements. It emits one Violation per (pair, broken rule).
func (v *Validator) Violations(b Board) []engine.Violation {
	queens := queenCells(b)
	var out []engine.Violation
	for i := 0; i < len(queens); i++ {
		for j := i + 1; j < len(queens); j++ {
			a, c := queens[i], queens[j]
			cells := []engine.Cell{a, c}
			if a.Row == c.Row {
				out = append(out, engine.Violation{
					Rule:    RuleSameRow,
					Message: "two queens share a row",
					Cells:   cells,
				})
			}
			if a.Col == c.Col {
				out = append(out, engine.Violation{
					Rule:    RuleSameCol,
					Message: "two queens share a column",
					Cells:   cells,
				})
			}
			if b.Region[engine.Index(a, b.N)] == b.Region[engine.Index(c, b.N)] {
				out = append(out, engine.Violation{
					Rule:    RuleSameRegion,
					Message: "two queens share a region",
					Cells:   cells,
				})
			}
			if adjacent(a, c) {
				out = append(out, engine.Violation{
					Rule:    RuleAdjacent,
					Message: "two queens touch (including diagonally)",
					Cells:   cells,
				})
			}
		}
	}
	return out
}

// Solved reports whether b is a complete, valid N-queens-with-regions
// solution: exactly N queens, one per row/col/region, none touching
// (including diagonally, local 8-neighbor only).
func (v *Validator) Solved(b Board) bool {
	n := b.N
	if n <= 0 || len(b.Cells) != n*n || len(b.Region) != n*n {
		return false
	}
	queens := queenCells(b)
	if len(queens) != n {
		return false
	}
	rowSeen := make([]bool, n)
	colSeen := make([]bool, n)
	regionSeen := make([]bool, n)
	for _, q := range queens {
		if rowSeen[q.Row] || colSeen[q.Col] {
			return false
		}
		reg := b.Region[engine.Index(q, n)]
		if reg < 0 || reg >= n || regionSeen[reg] {
			return false
		}
		rowSeen[q.Row] = true
		colSeen[q.Col] = true
		regionSeen[reg] = true
	}
	for i := 0; i < len(queens); i++ {
		for j := i + 1; j < len(queens); j++ {
			if adjacent(queens[i], queens[j]) {
				return false
			}
		}
	}
	return true
}
