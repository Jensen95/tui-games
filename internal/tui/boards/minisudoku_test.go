package boards

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	minisudokugame "github.com/Jensen95/tui-games/internal/games/minisudoku"
	"github.com/Jensen95/tui-games/internal/tui"
)

// ---------------------------------------------------------------------------
// Test helpers.
// ---------------------------------------------------------------------------

// minisudokuTestGenerate builds a fixed-seed puzzle via the real
// engine/games entry point (never hand-rolled), the same path production
// code uses.
func minisudokuTestGenerate(t *testing.T) (minisudokugame.Puzzle, minisudokugame.Solution, engine.Generated) {
	t.Helper()
	gen, err := minisudokugame.Entry().Generate(engine.Easy, engine.NewRand(7))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	p, ok := gen.Puzzle.(minisudokugame.Puzzle)
	if !ok {
		t.Fatalf("generated puzzle has unexpected type %T", gen.Puzzle)
	}
	sol, ok := gen.Solution.(minisudokugame.Solution)
	if !ok {
		t.Fatalf("generated solution has unexpected type %T", gen.Solution)
	}
	if len(sol.Cells) != p.N*p.N {
		t.Fatalf("fixture invariant broken: solution length %d != %d cells", len(sol.Cells), p.N*p.N)
	}
	return p, sol, gen
}

// minisudokuKeyRune builds a plain (unshifted) key press for a printable
// rune, e.g. a digit or a letter fallback key.
func minisudokuKeyRune(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// minisudokuKeyShiftDigit encodes Shift+digit in its enhanced-terminal shape
// (internal/tui/keys_test.go's fixture shape): the base digit Code with a
// real ModShift bit. tui.IsShifted reads that directly, so this is
// unambiguous regardless of tui.EnhancedKeyboardActive.
func minisudokuKeyShiftDigit(d rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: d, Mod: tea.ModShift}
}

// minisudokuMoveCursorTo drives the adapter's cursor from wherever it is to
// target via plain wasd key presses, the same input path a real player's
// movement would take.
func minisudokuMoveCursorTo(a *minisudokuAdapter, target engine.Cell) {
	for a.cursor.Row < target.Row {
		a.HandleKey(minisudokuKeyRune('s'))
	}
	for a.cursor.Row > target.Row {
		a.HandleKey(minisudokuKeyRune('w'))
	}
	for a.cursor.Col < target.Col {
		a.HandleKey(minisudokuKeyRune('d'))
	}
	for a.cursor.Col > target.Col {
		a.HandleKey(minisudokuKeyRune('a'))
	}
}

// minisudokuFirstNonGivenIdx returns the row-major index of the first
// non-given cell, failing the test if the fixture puzzle has none.
func minisudokuFirstNonGivenIdx(t *testing.T, a *minisudokuAdapter) int {
	t.Helper()
	n := a.puzzle.N
	for idx := 0; idx < n*n; idx++ {
		if !a.isGiven(idx) {
			return idx
		}
	}
	t.Fatalf("fixture puzzle has no non-given cell")
	return -1
}

// ---------------------------------------------------------------------------
// Tests.
// ---------------------------------------------------------------------------

// TestMiniSudoku_ScriptedFullSolve is the proof the adapter works end to
// end: drive it with synthetic tea.KeyPressMsg values along the two-handed
// scheme (wasd to move, digits 1-6 to place) to the recorded solution, and
// confirm Solved() flips true with no violations.
func TestMiniSudoku_ScriptedFullSolve(t *testing.T) {
	puzzle, solution, gen := minisudokuTestGenerate(t)
	adapter := minisudokuNew(gen)
	a, ok := adapter.(*minisudokuAdapter)
	if !ok {
		t.Fatalf("minisudokuNew returned unexpected type %T", adapter)
	}

	if a.Solved() {
		t.Fatalf("fresh adapter must not report solved")
	}

	n := puzzle.N
	for idx := 0; idx < n*n; idx++ {
		if a.isGiven(idx) {
			continue
		}
		target := engine.CellAt(idx, n)
		minisudokuMoveCursorTo(a, target)

		digit := solution.Cells[idx]
		key := minisudokuKeyRune(rune('0' + digit))
		if changed := a.HandleKey(key); !changed {
			t.Fatalf("cell %d (%v): expected digit %d to change the board", idx, target, digit)
		}
		if a.cells[idx] != digit {
			t.Fatalf("cell %d: got %d, want %d", idx, a.cells[idx], digit)
		}
	}

	if v := a.Violations(); len(v) != 0 {
		t.Fatalf("expected no violations after driving the recorded solution, got %v", v)
	}
	if !a.Solved() {
		t.Fatalf("expected Solved() to be true after driving the recorded solution")
	}
}

// TestMiniSudoku_SecondaryActionAndLegacyFallback exercises Mini Sudoku's
// actual secondary action (per docs/plan/docs/03-tui-design.md's two-handed
// scheme table, Mini Sudoku's row): Shift+1-6 toggles a pencil-mark note —
// tested here in its enhanced-terminal encoding (real ModShift bit) — and
// its legacy fallback ('e' toggles note-entry mode so a plain digit key
// toggles a note instead of placing it), which must work identically
// regardless of terminal capability. Mini Sudoku's primary action is a bare
// digit key, not Space, so Shift+Space plays no role in this game's scheme.
func TestMiniSudoku_SecondaryActionAndLegacyFallback(t *testing.T) {
	_, _, gen := minisudokuTestGenerate(t)
	a := minisudokuNew(gen).(*minisudokuAdapter)

	idx := minisudokuFirstNonGivenIdx(t, a)
	minisudokuMoveCursorTo(a, engine.CellAt(idx, a.puzzle.N))

	// Enhanced-terminal encoding: Shift+3 toggles note 3 on/off.
	shift3 := minisudokuKeyShiftDigit('3')
	if !tui.IsShifted(shift3) {
		t.Fatalf("test fixture key does not look shifted")
	}
	if changed := a.HandleKey(shift3); !changed {
		t.Fatalf("expected Shift+3 to toggle note 3")
	}
	if a.notes[idx]&(1<<2) == 0 {
		t.Fatalf("expected note bit for digit 3 to be set, got notes=%08b", a.notes[idx])
	}
	if a.cells[idx] != 0 {
		t.Fatalf("expected Shift+3 to leave the cell's digit empty, got %d", a.cells[idx])
	}
	if changed := a.HandleKey(shift3); !changed {
		t.Fatalf("expected a second Shift+3 to clear note 3")
	}
	if a.notes[idx]&(1<<2) != 0 {
		t.Fatalf("expected note bit for digit 3 to be cleared, got notes=%08b", a.notes[idx])
	}

	// A bare digit press must never look shifted (the legacy terminal can't
	// tell Shift+digit apart from digit at all, per tui.IsShifted).
	plainFive := minisudokuKeyRune('5')
	if tui.IsShifted(plainFive) {
		t.Fatalf("plain digit must never look shifted")
	}
	if tui.EnhancedKeyboardActive() {
		t.Fatalf("expected EnhancedKeyboardActive() to default false in the boards test binary")
	}

	// Without note mode, a plain digit places the digit (primary action).
	if changed := a.HandleKey(plainFive); !changed {
		t.Fatalf("expected plain digit 5 to place a digit")
	}
	if a.cells[idx] != 5 {
		t.Fatalf("expected cell to hold digit 5, got %d", a.cells[idx])
	}

	// Clear back to empty so note-mode toggling is observable.
	a.HandleKey(minisudokuKeyRune('0'))
	if a.cells[idx] != 0 {
		t.Fatalf("expected '0' to clear the cell, got %d", a.cells[idx])
	}

	// Legacy fallback: 'e' toggles note-entry mode; while active, plain
	// digits toggle notes instead of placing them.
	if changed := a.HandleKey(minisudokuKeyRune('e')); !changed {
		t.Fatalf("expected 'e' to report a change (note mode toggled)")
	}
	if !a.noteMode {
		t.Fatalf("expected note mode to be active after 'e'")
	}
	if changed := a.HandleKey(minisudokuKeyRune('2')); !changed {
		t.Fatalf("expected digit '2' in note mode to toggle a note")
	}
	if a.notes[idx]&(1<<1) == 0 {
		t.Fatalf("expected note bit for digit 2 to be set, got notes=%08b", a.notes[idx])
	}
	if a.cells[idx] != 0 {
		t.Fatalf("expected note-mode digit entry to leave the cell empty, got %d", a.cells[idx])
	}

	// Toggling 'e' again exits note mode; plain digits place again.
	a.HandleKey(minisudokuKeyRune('e'))
	if a.noteMode {
		t.Fatalf("expected note mode to be inactive after second 'e'")
	}
	a.HandleKey(minisudokuKeyRune('6'))
	if a.cells[idx] != 6 {
		t.Fatalf("expected digit '6' outside note mode to place the digit, got %d", a.cells[idx])
	}
}

// TestMiniSudoku_Mouse exercises Mini Sudoku's defining mouse interaction
// (click-then-type, per docs/plan/games/mini-sudoku.md's "Mouse" section):
// left-click focuses a cell, right-click clears it, and the wheel cycles
// its digit — all respecting given-cell immutability.
func TestMiniSudoku_Mouse(t *testing.T) {
	p, _, gen := minisudokuTestGenerate(t)
	a := minisudokuNew(gen).(*minisudokuAdapter)

	idx := minisudokuFirstNonGivenIdx(t, a)
	ref := tui.CellRef{Cell: engine.CellAt(idx, p.N), Valid: true}

	leftClick := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseLeft}
	if changed := a.HandleMouse(leftClick, ref); !changed {
		t.Fatalf("expected left-click to move the cursor (a change) onto a fresh adapter")
	}
	if a.cursor != ref.Cell {
		t.Fatalf("expected click to focus the clicked cell, got %v want %v", a.cursor, ref.Cell)
	}
	if changed := a.HandleMouse(leftClick, ref); changed {
		t.Fatalf("expected a second left-click on the same cell to be a no-op")
	}

	wheelUp := tui.MouseEvent{Type: tui.MouseEventWheel, Button: tea.MouseWheelUp}
	if changed := a.HandleMouse(wheelUp, ref); !changed {
		t.Fatalf("expected wheel-up to cycle the digit up from empty")
	}
	if a.cells[idx] != 1 {
		t.Fatalf("expected wheel-up from empty to land on digit 1, got %d", a.cells[idx])
	}
	a.HandleMouse(wheelUp, ref)
	if a.cells[idx] != 2 {
		t.Fatalf("expected a second wheel-up to land on digit 2, got %d", a.cells[idx])
	}

	wheelDown := tui.MouseEvent{Type: tui.MouseEventWheel, Button: tea.MouseWheelDown}
	if changed := a.HandleMouse(wheelDown, ref); !changed {
		t.Fatalf("expected wheel-down to cycle the digit back down")
	}
	if a.cells[idx] != 1 {
		t.Fatalf("expected wheel-down to land back on digit 1, got %d", a.cells[idx])
	}

	rightClick := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseRight}
	if changed := a.HandleMouse(rightClick, ref); !changed {
		t.Fatalf("expected right-click to clear the cell")
	}
	if a.cells[idx] != 0 {
		t.Fatalf("expected right-click to clear the cell, got %d", a.cells[idx])
	}
	if changed := a.HandleMouse(rightClick, ref); changed {
		t.Fatalf("expected right-click on an already-empty cell to be a no-op")
	}

	invalidRef := tui.CellRef{Valid: false}
	if changed := a.HandleMouse(leftClick, invalidRef); changed {
		t.Fatalf("expected a click resolving to no cell to be a no-op")
	}

	motion := tui.MouseEvent{Type: tui.MouseEventMotion}
	if changed := a.HandleMouse(motion, ref); changed {
		t.Fatalf("expected a bare motion event (no press/wheel) to be a no-op for Mini Sudoku")
	}

	givenIdx := -1
	for i := 0; i < p.N*p.N; i++ {
		if a.isGiven(i) {
			givenIdx = i
			break
		}
	}
	if givenIdx < 0 {
		t.Fatalf("fixture puzzle has no given cells")
	}
	givenRef := tui.CellRef{Cell: engine.CellAt(givenIdx, p.N), Valid: true}
	before := a.cells[givenIdx]
	if changed := a.HandleMouse(rightClick, givenRef); changed {
		t.Fatalf("expected right-click on a given cell to be a no-op")
	}
	if changed := a.HandleMouse(wheelUp, givenRef); changed {
		t.Fatalf("expected wheel-up on a given cell to be a no-op")
	}
	if a.cells[givenIdx] != before {
		t.Fatalf("given cell was mutated: got %d want %d", a.cells[givenIdx], before)
	}
}

// TestMiniSudoku_GivenCellsImmutable checks given-cell immutability across
// every keyboard mutation path (digit placement, note toggling in both
// keyboard modes, and clearing).
func TestMiniSudoku_GivenCellsImmutable(t *testing.T) {
	p, _, gen := minisudokuTestGenerate(t)
	a := minisudokuNew(gen).(*minisudokuAdapter)

	givenIdx := -1
	for i := 0; i < p.N*p.N; i++ {
		if a.isGiven(i) {
			givenIdx = i
			break
		}
	}
	if givenIdx < 0 {
		t.Fatalf("fixture puzzle has no given cells")
	}
	minisudokuMoveCursorTo(a, engine.CellAt(givenIdx, p.N))
	before := a.cells[givenIdx]

	keys := []tea.KeyPressMsg{
		minisudokuKeyRune('1'),
		minisudokuKeyShiftDigit('2'),
		minisudokuKeyRune('0'),
		minisudokuKeyRune(tea.KeyBackspace),
	}
	for _, k := range keys {
		if changed := a.HandleKey(k); changed {
			t.Fatalf("expected key %+v on a given cell to be a no-op", k)
		}
	}
	if a.cells[givenIdx] != before {
		t.Fatalf("given cell was mutated: got %d want %d", a.cells[givenIdx], before)
	}
	if a.notes[givenIdx] != 0 {
		t.Fatalf("given cell gained pencil marks: %08b", a.notes[givenIdx])
	}

	// Note mode toggled on: digits over a given cell must still be a no-op.
	a.HandleKey(minisudokuKeyRune('e'))
	if changed := a.HandleKey(minisudokuKeyRune('3')); changed {
		t.Fatalf("expected note-mode digit entry on a given cell to be a no-op")
	}
	if a.cells[givenIdx] != before || a.notes[givenIdx] != 0 {
		t.Fatalf("given cell was mutated via note mode")
	}
}

// TestMiniSudoku_UndoResetHint covers Hint (revealing a logically-forced
// cell, naming its technique, always matching the recorded solution),
// Undo (unwinding one mutation at a time, including a no-op once history is
// empty), and Reset (back to the ungenerated-move state) — plus that none
// of this ever mutates the puzzle's own given cells.
func TestMiniSudoku_UndoResetHint(t *testing.T) {
	p, sol, gen := minisudokuTestGenerate(t)
	a := minisudokuNew(gen).(*minisudokuAdapter)

	idx1 := minisudokuFirstNonGivenIdx(t, a)
	a.Hint()
	if a.cells[idx1] != sol.Cells[idx1] {
		t.Fatalf("expected first hint to fill cell %d with %d, got %d", idx1, sol.Cells[idx1], a.cells[idx1])
	}
	wantCursor := engine.CellAt(idx1, p.N)
	if a.cursor != wantCursor {
		t.Fatalf("expected hint to move the cursor to %v, got %v", wantCursor, a.cursor)
	}
	if a.lastHintTechnique == "" {
		t.Fatalf("expected Hint to record a technique")
	}

	idx2 := -1
	for i := idx1 + 1; i < p.N*p.N; i++ {
		if !a.isGiven(i) {
			idx2 = i
			break
		}
	}
	if idx2 < 0 {
		t.Fatalf("fixture puzzle needs at least two non-given cells")
	}
	a.Hint()
	if a.cells[idx2] != sol.Cells[idx2] {
		t.Fatalf("expected second hint to fill cell %d with %d, got %d", idx2, sol.Cells[idx2], a.cells[idx2])
	}

	a.Undo()
	if a.cells[idx2] != 0 {
		t.Fatalf("expected undo to revert the second hint, got %d", a.cells[idx2])
	}
	a.Undo()
	if a.cells[idx1] != 0 {
		t.Fatalf("expected undo to revert the first hint, got %d", a.cells[idx1])
	}

	// Undo past the bottom of history is a no-op, not a panic.
	a.Undo()
	if a.cells[idx1] != 0 {
		t.Fatalf("expected extra undo beyond history to stay a no-op")
	}

	for i, v := range p.Givens {
		if a.cells[i] != v {
			t.Fatalf("given cell %d was mutated by play: got %d want %d", i, a.cells[i], v)
		}
	}

	a.Hint()
	a.Hint()
	a.Reset()
	for idx, v := range a.cells {
		if _, given := p.Givens[idx]; given {
			continue
		}
		if v != 0 {
			t.Fatalf("expected reset to clear non-given cell %d, got %d", idx, v)
		}
	}
	for idx, notes := range a.notes {
		if notes != 0 {
			t.Fatalf("expected reset to clear pencil marks, cell %d has %08b", idx, notes)
		}
	}
	if a.cursor != (engine.Cell{}) {
		t.Fatalf("expected reset to move the cursor back to the origin, got %v", a.cursor)
	}
	if a.noteMode {
		t.Fatalf("expected reset to turn note mode off")
	}
	if len(a.history) != 0 {
		t.Fatalf("expected reset to clear undo history, got %d entries", len(a.history))
	}

	// Hint is a no-op once the board has no empty cells left (defensive:
	// walk it to completion via the solution first).
	for idx := range a.cells {
		if !a.isGiven(idx) {
			a.cells[idx] = sol.Cells[idx]
		}
	}
	before := append([]int(nil), a.cells...)
	a.Hint()
	for i := range a.cells {
		if a.cells[i] != before[i] {
			t.Fatalf("expected Hint on a full board to be a no-op, cell %d changed", i)
		}
	}
}

// TestMiniSudoku_NotesClearedOnDigitPlacement checks that placing a digit
// over a cell that had pencil marks discards those marks (they'd be
// meaningless once the cell is resolved), and that Undo restores both the
// digit and the marks together as one atomic move.
func TestMiniSudoku_NotesClearedOnDigitPlacement(t *testing.T) {
	_, _, gen := minisudokuTestGenerate(t)
	a := minisudokuNew(gen).(*minisudokuAdapter)

	idx := minisudokuFirstNonGivenIdx(t, a)
	minisudokuMoveCursorTo(a, engine.CellAt(idx, a.puzzle.N))

	a.HandleKey(minisudokuKeyShiftDigit('1'))
	a.HandleKey(minisudokuKeyShiftDigit('4'))
	if a.notes[idx] == 0 {
		t.Fatalf("expected two pencil marks to be set before placing a digit")
	}

	a.HandleKey(minisudokuKeyRune('6'))
	if a.cells[idx] != 6 {
		t.Fatalf("expected digit 6 to be placed, got %d", a.cells[idx])
	}
	if a.notes[idx] != 0 {
		t.Fatalf("expected pencil marks to be cleared once a digit is placed, got %08b", a.notes[idx])
	}

	a.Undo()
	if a.cells[idx] != 0 || a.notes[idx] == 0 {
		t.Fatalf("expected undo to restore the pre-placement state (empty digit, marks present), got cell=%d notes=%08b", a.cells[idx], a.notes[idx])
	}
}

// TestMiniSudoku_GeometryMatchesRenderedView asserts GridGeometry's
// arithmetic (rows/cols, cell size, gutters) matches the actual dimensions
// of the grid portion of View's output, which is what makes the shell's
// mouse hit-testing (tui.CellFromPoint) land on the right cells.
func TestMiniSudoku_GeometryMatchesRenderedView(t *testing.T) {
	_, _, gen := minisudokuTestGenerate(t)
	a := minisudokuNew(gen).(*minisudokuAdapter)

	view := a.View(tui.Dark())
	geo := a.GridGeometry()

	lines := strings.Split(view, "\n")
	wantHeight := geo.Rows*geo.CellHeight + (geo.Rows-1)*geo.RowGutter
	if geo.OriginY+wantHeight > len(lines) {
		t.Fatalf("view has %d lines, want at least %d starting at origin row %d", len(lines), wantHeight, geo.OriginY)
	}

	wantWidth := geo.Cols*geo.CellWidth + (geo.Cols-1)*geo.ColGutter
	for i := 0; i < wantHeight; i++ {
		line := lines[geo.OriginY+i]
		if gotWidth := lipgloss.Width(line); gotWidth != wantWidth {
			t.Fatalf("grid line %d has rendered width %d, want %d (line=%q)", i, gotWidth, wantWidth, line)
		}
	}

	if geo.OriginX != 0 {
		t.Fatalf("expected OriginX 0 (grid starts at the left of the returned view), got %d", geo.OriginX)
	}
}

// TestMiniSudoku_ViewShowsGivensNotesAndHelp is a light content/structure
// check (per 04-testing-strategy.md: substrings, not golden frames) that the
// renderer actually reflects puzzle/board state (givens, a placed pencil
// mark) and advertises the active binding set.
func TestMiniSudoku_ViewShowsGivensNotesAndHelp(t *testing.T) {
	p, _, gen := minisudokuTestGenerate(t)
	a := minisudokuNew(gen).(*minisudokuAdapter)

	var givenDigit int
	for _, v := range p.Givens {
		givenDigit = v
		break
	}
	if givenDigit == 0 {
		t.Fatalf("fixture puzzle has no given cells")
	}
	view := a.View(tui.Grey())
	if !strings.Contains(view, strconv.Itoa(givenDigit)) {
		t.Fatalf("expected View to render at least one given digit %d, got:\n%s", givenDigit, view)
	}

	idx := minisudokuFirstNonGivenIdx(t, a)
	minisudokuMoveCursorTo(a, engine.CellAt(idx, a.puzzle.N))
	a.HandleKey(minisudokuKeyShiftDigit('5'))
	noteView := a.View(tui.Grey())
	if !strings.Contains(noteView, "5") {
		t.Fatalf("expected View to render pencil mark '5', got:\n%s", noteView)
	}

	if tui.EnhancedKeyboardActive() {
		if !strings.Contains(view, "Shift+1-6") {
			t.Fatalf("expected help line to advertise Shift+1-6 when enhanced keyboard is active")
		}
	} else if !strings.Contains(view, "e: toggle note mode") {
		t.Fatalf("expected help line to advertise the legacy 'e' fallback on a legacy keyboard")
	}
}
