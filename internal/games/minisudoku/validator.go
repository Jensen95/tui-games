package minisudoku

import (
	"fmt"

	"github.com/Jensen95/tui-games/internal/engine"
)

// Validator implements engine.Validator[Board]. It referees a board state
// (partial or complete): Violations reports only rules that are ALREADY
// broken by filled cells (an empty cell, value 0, is never flagged), and
// Solved reports whether the board is a complete, fully valid solution.
type Validator struct{}

var _ engine.Validator[Board] = Validator{}

// Violations returns every currently-violated rule for board b:
//   - "value": a filled cell holds a digit outside 1..N.
//   - "row": a digit repeated within a row (among filled, in-range cells).
//   - "column": a digit repeated within a column.
//   - "box": a digit repeated within a 2×3 box. The box is 2 rows × 3
//     columns (not 3×2, not 2×2) — see docs/plan/games/mini-sudoku.md
//     "Gotchas".
func (v Validator) Violations(b Board) []engine.Violation {
	var viols []engine.Violation

	// Invalid values (out of range, filled cells only).
	for i, val := range b.Cells {
		if val != 0 && (val < 1 || val > N) {
			cell := engine.CellAt(i, N)
			viols = append(viols, engine.Violation{
				Rule:    "value",
				Message: fmt.Sprintf("cell (%d,%d) has invalid digit %d", cell.Row, cell.Col, val),
				Cells:   []engine.Cell{cell},
			})
		}
	}

	// Row duplicates.
	for row := 0; row < N; row++ {
		var byDigit [N + 1][]engine.Cell
		for col := 0; col < N; col++ {
			val := b.Cells[row*N+col]
			if val >= 1 && val <= N {
				byDigit[val] = append(byDigit[val], engine.Cell{Row: row, Col: col})
			}
		}
		for d := 1; d <= N; d++ {
			if len(byDigit[d]) > 1 {
				viols = append(viols, engine.Violation{
					Rule:    "row",
					Message: fmt.Sprintf("digit %d repeated in row %d", d, row),
					Cells:   byDigit[d],
				})
			}
		}
	}

	// Column duplicates.
	for col := 0; col < N; col++ {
		var byDigit [N + 1][]engine.Cell
		for row := 0; row < N; row++ {
			val := b.Cells[row*N+col]
			if val >= 1 && val <= N {
				byDigit[val] = append(byDigit[val], engine.Cell{Row: row, Col: col})
			}
		}
		for d := 1; d <= N; d++ {
			if len(byDigit[d]) > 1 {
				viols = append(viols, engine.Violation{
					Rule:    "column",
					Message: fmt.Sprintf("digit %d repeated in column %d", d, col),
					Cells:   byDigit[d],
				})
			}
		}
	}

	// Box duplicates (2×3 boxes: 2 rows tall, 3 columns wide).
	var boxCells [N][]engine.Cell
	for row := 0; row < N; row++ {
		for col := 0; col < N; col++ {
			box := boxID(row, col, BoxH, BoxW, N)
			boxCells[box] = append(boxCells[box], engine.Cell{Row: row, Col: col})
		}
	}
	for box := 0; box < N; box++ {
		var byDigit [N + 1][]engine.Cell
		for _, cell := range boxCells[box] {
			val := b.Cells[cell.Row*N+cell.Col]
			if val >= 1 && val <= N {
				byDigit[val] = append(byDigit[val], cell)
			}
		}
		for d := 1; d <= N; d++ {
			if len(byDigit[d]) > 1 {
				viols = append(viols, engine.Violation{
					Rule:    "box",
					Message: fmt.Sprintf("digit %d repeated in box %d", d, box),
					Cells:   byDigit[d],
				})
			}
		}
	}

	return viols
}

// Solved reports whether b is a complete (every cell in 1..N) board with
// zero violations.
func (v Validator) Solved(b Board) bool {
	if len(b.Cells) != N*N {
		return false
	}
	for _, c := range b.Cells {
		if c < 1 || c > N {
			return false
		}
	}
	return len(v.Violations(b)) == 0
}
