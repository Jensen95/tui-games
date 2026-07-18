package tango

import (
	"fmt"

	"github.com/Jensen95/tui-games/internal/engine"
)

// Validator implements engine.Validator[Board]. It referees a board state
// (partial or complete): Violations reports only rules that are ALREADY
// broken by filled cells (never flags an unfilled cell), and Solved reports
// whether the board is a complete, fully valid solution.
type Validator struct{}

var _ engine.Validator[Board] = Validator{}

// Violations returns every currently-violated rule for board b:
//   - "balance": a row or column already has more than N/2 of one symbol
//     filled (can never be fixed by filling the remaining cells).
//   - "three-in-a-row": three identical, already-filled symbols consecutive
//     horizontally or vertically. Diagonals are never checked — the classic
//     Tango/Takuzu misconception this package must not reproduce.
//   - "edge-constraint": a "=" edge whose two (already-filled) cells differ,
//     or a "×" edge whose two (already-filled) cells match.
func (v Validator) Violations(b Board) []engine.Violation {
	n := b.N
	half := n / 2
	var viols []engine.Violation

	// Row balance.
	for row := 0; row < n; row++ {
		sunN, moonN := 0, 0
		base := row * n
		for col := 0; col < n; col++ {
			switch b.Cells[base+col] {
			case Sun:
				sunN++
			case Moon:
				moonN++
			}
		}
		if sunN > half || moonN > half {
			cells := make([]engine.Cell, n)
			for col := 0; col < n; col++ {
				cells[col] = engine.Cell{Row: row, Col: col}
			}
			viols = append(viols, engine.Violation{
				Rule:    "balance",
				Message: fmt.Sprintf("row %d has more than %d of one symbol", row, half),
				Cells:   cells,
			})
		}
	}

	// Column balance.
	for col := 0; col < n; col++ {
		sunN, moonN := 0, 0
		for row := 0; row < n; row++ {
			switch b.Cells[row*n+col] {
			case Sun:
				sunN++
			case Moon:
				moonN++
			}
		}
		if sunN > half || moonN > half {
			cells := make([]engine.Cell, n)
			for row := 0; row < n; row++ {
				cells[row] = engine.Cell{Row: row, Col: col}
			}
			viols = append(viols, engine.Violation{
				Rule:    "balance",
				Message: fmt.Sprintf("column %d has more than %d of one symbol", col, half),
				Cells:   cells,
			})
		}
	}

	// Horizontal three-in-a-row.
	for row := 0; row < n; row++ {
		base := row * n
		for col := 0; col+2 < n; col++ {
			i0 := base + col
			a, b1, c := b.Cells[i0], b.Cells[i0+1], b.Cells[i0+2]
			if a != Empty && a == b1 && b1 == c {
				viols = append(viols, engine.Violation{
					Rule:    "three-in-a-row",
					Message: fmt.Sprintf("row %d has three consecutive identical symbols starting at column %d", row, col),
					Cells: []engine.Cell{
						{Row: row, Col: col}, {Row: row, Col: col + 1}, {Row: row, Col: col + 2},
					},
				})
			}
		}
	}

	// Vertical three-in-a-row.
	for col := 0; col < n; col++ {
		for row := 0; row+2 < n; row++ {
			i0 := row*n + col
			a, b1, c := b.Cells[i0], b.Cells[i0+n], b.Cells[i0+2*n]
			if a != Empty && a == b1 && b1 == c {
				viols = append(viols, engine.Violation{
					Rule:    "three-in-a-row",
					Message: fmt.Sprintf("column %d has three consecutive identical symbols starting at row %d", col, row),
					Cells: []engine.Cell{
						{Row: row, Col: col}, {Row: row + 1, Col: col}, {Row: row + 2, Col: col},
					},
				})
			}
		}
	}

	// Edge constraints.
	viols = append(viols, edgeViolations(b.Cells, b.HEdges, n)...)
	viols = append(viols, edgeViolations(b.Cells, b.VEdges, n)...)

	return viols
}

// edgeViolations checks one edge set (H or V) against already-filled cells.
func edgeViolations(cells []Symbol, edges map[[2]int]Relation, n int) []engine.Violation {
	var viols []engine.Violation
	for pair, rel := range edges {
		a, b := pair[0], pair[1]
		ca, cb := cells[a], cells[b]
		if ca == Empty || cb == Empty {
			continue
		}
		broken := (rel == Equal && ca != cb) || (rel == Cross && ca == cb)
		if !broken {
			continue
		}
		viols = append(viols, engine.Violation{
			Rule:    "edge-constraint",
			Message: fmt.Sprintf("edge between cells %d and %d violates its constraint", a, b),
			Cells:   []engine.Cell{engine.CellAt(a, n), engine.CellAt(b, n)},
		})
	}
	return viols
}

// Solved reports whether b is a complete (no Empty cells) board with zero
// violations.
func (v Validator) Solved(b Board) bool {
	for _, c := range b.Cells {
		if c == Empty {
			return false
		}
	}
	return len(v.Violations(b)) == 0
}
