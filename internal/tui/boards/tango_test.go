package boards

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	tangogame "github.com/Jensen95/tui-games/internal/games/tango"
	"github.com/Jensen95/tui-games/internal/tui"
)

// ---------------------------------------------------------------------------
// Test helpers.
// ---------------------------------------------------------------------------

// tangoTestGenerate builds a fixed-seed puzzle via the real engine/games
// entry point (never hand-rolled), the same path production code uses.
func tangoTestGenerate(t *testing.T) (tangogame.Puzzle, tangogame.Board, engine.Generated) {
	t.Helper()
	gen, err := tangogame.Entry().Generate(engine.Easy, engine.NewRand(7))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	p, ok := gen.Puzzle.(tangogame.Puzzle)
	if !ok {
		t.Fatalf("generated puzzle has unexpected type %T", gen.Puzzle)
	}
	sol, ok := gen.Solution.(tangogame.Board)
	if !ok {
		t.Fatalf("generated solution has unexpected type %T", gen.Solution)
	}
	if len(sol.Cells) != p.N*p.N {
		t.Fatalf("fixture invariant broken: solution length %d != %d cells", len(sol.Cells), p.N*p.N)
	}
	return p, sol, gen
}

// tangoKeySpace / tangoKeyShiftSpace encode the two-handed scheme's primary
// and secondary actions. The shifted variant carries a real Mod bit
// (internal/tui/keys_test.go's fixture shape) — tui.IsShifted reads that
// directly, so this is unambiguous regardless of tui.EnhancedKeyboardActive.
func tangoKeySpace() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "} }
func tangoKeyShiftSpace() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModShift}
}
func tangoKeyRune(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// tangoMoveCursorTo drives the adapter's cursor from wherever it is to
// target via plain wasd key presses, the same input path a real player's
// movement would take.
func tangoMoveCursorTo(a *tangoAdapter, target engine.Cell) {
	for a.cursor.Row < target.Row {
		a.HandleKey(tangoKeyRune('s'))
	}
	for a.cursor.Row > target.Row {
		a.HandleKey(tangoKeyRune('w'))
	}
	for a.cursor.Col < target.Col {
		a.HandleKey(tangoKeyRune('d'))
	}
	for a.cursor.Col > target.Col {
		a.HandleKey(tangoKeyRune('a'))
	}
}

// tangoFirstNonGivenIdx returns the row-major index of the first non-given
// cell, failing the test if the fixture puzzle has none.
func tangoFirstNonGivenIdx(t *testing.T, a *tangoAdapter) int {
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

// TestTango_ScriptedFullSolve is the proof the adapter works end to end:
// drive it with synthetic tea.KeyPressMsg values along the two-handed scheme
// (wasd to move, Space for sun, Shift+Space for moon) to the recorded
// solution, and confirm Solved() flips true with no violations.
func TestTango_ScriptedFullSolve(t *testing.T) {
	puzzle, solution, gen := tangoTestGenerate(t)
	adapter := tangoNew(gen)
	a, ok := adapter.(*tangoAdapter)
	if !ok {
		t.Fatalf("tangoNew returned unexpected type %T", adapter)
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
		tangoMoveCursorTo(a, target)

		switch solution.Cells[idx] {
		case tangogame.Sun:
			if changed := a.HandleKey(tangoKeySpace()); !changed {
				t.Fatalf("cell %d (%v): expected Space to place a sun", idx, target)
			}
		case tangogame.Moon:
			if changed := a.HandleKey(tangoKeyShiftSpace()); !changed {
				t.Fatalf("cell %d (%v): expected Shift+Space to place a moon", idx, target)
			}
		default:
			t.Fatalf("solution cell %d has non-sun/moon symbol %v", idx, solution.Cells[idx])
		}
		if a.cells[idx] != solution.Cells[idx] {
			t.Fatalf("cell %d: got %v, want %v", idx, a.cells[idx], solution.Cells[idx])
		}
	}

	if v := a.Violations(); len(v) != 0 {
		t.Fatalf("expected no violations after driving the recorded solution, got %v", v)
	}
	if !a.Solved() {
		t.Fatalf("expected Solved() to be true after driving the recorded solution")
	}
}

// TestTango_SecondaryActionAndLegacyFallback exercises both halves of
// Tango's secondary action (Shift+Space placing/clearing a moon) and the
// legacy fallback path (plain Space cycling empty->sun->moon->empty, and
// 'm' placing a moon directly) that must work regardless of terminal
// capability.
func TestTango_SecondaryActionAndLegacyFallback(t *testing.T) {
	_, _, gen := tangoTestGenerate(t)
	a := tangoNew(gen).(*tangoAdapter)

	idx := tangoFirstNonGivenIdx(t, a)
	tangoMoveCursorTo(a, engine.CellAt(idx, a.puzzle.N))

	shiftSpace := tangoKeyShiftSpace()
	if !tui.IsSpace(shiftSpace) || !tui.IsShifted(shiftSpace) {
		t.Fatalf("test fixture key does not look like Shift+Space")
	}
	if changed := a.HandleKey(shiftSpace); !changed {
		t.Fatalf("expected Shift+Space to place a moon")
	}
	if a.cells[idx] != tangogame.Moon {
		t.Fatalf("expected cell to be Moon after Shift+Space, got %v", a.cells[idx])
	}
	if changed := a.HandleKey(shiftSpace); !changed {
		t.Fatalf("expected a second Shift+Space to clear the moon")
	}
	if a.cells[idx] != tangogame.Empty {
		t.Fatalf("expected cell to be Empty after second Shift+Space, got %v", a.cells[idx])
	}

	// A bare Space on a legacy terminal is indistinguishable from
	// Shift+Space (no Mod bit at all) and must be treated as the primary/
	// legacy-cycle action, never the secondary one.
	plainSpace := tangoKeySpace()
	if tui.IsShifted(plainSpace) {
		t.Fatalf("plain space must never look shifted")
	}

	// boards package tests cannot set tui's unexported enhanced-keyboard
	// flag, so it always reads false here — meaning plain Space always takes
	// the legacy fallback (full cycle) path in this test binary.
	if tui.EnhancedKeyboardActive() {
		t.Fatalf("expected EnhancedKeyboardActive() to default false in the boards test binary")
	}

	a.HandleKey(plainSpace) // empty -> sun
	if a.cells[idx] != tangogame.Sun {
		t.Fatalf("expected legacy Space to cycle Empty->Sun, got %v", a.cells[idx])
	}
	a.HandleKey(plainSpace) // sun -> moon
	if a.cells[idx] != tangogame.Moon {
		t.Fatalf("expected legacy Space to cycle Sun->Moon, got %v", a.cells[idx])
	}
	a.HandleKey(plainSpace) // moon -> empty
	if a.cells[idx] != tangogame.Empty {
		t.Fatalf("expected legacy Space to cycle Moon->Empty, got %v", a.cells[idx])
	}

	// Legacy 'm' fallback always sets a moon directly (idempotent, not a
	// toggle).
	mKey := tangoKeyRune('m')
	if changed := a.HandleKey(mKey); !changed {
		t.Fatalf("expected 'm' to place a moon")
	}
	if a.cells[idx] != tangogame.Moon {
		t.Fatalf("expected cell to be Moon after 'm', got %v", a.cells[idx])
	}
	if changed := a.HandleKey(mKey); changed {
		t.Fatalf("expected 'm' on an already-moon cell to be a no-op")
	}
}

// TestTango_Mouse exercises Tango's defining mouse interaction: left-click
// cycles a cell (empty->sun->moon->empty), right-click clears it, and both
// respect given-cell immutability.
func TestTango_Mouse(t *testing.T) {
	p, _, gen := tangoTestGenerate(t)
	a := tangoNew(gen).(*tangoAdapter)

	idx := tangoFirstNonGivenIdx(t, a)
	ref := tui.CellRef{Cell: engine.CellAt(idx, p.N), Valid: true}

	leftClick := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseLeft}
	if changed := a.HandleMouse(leftClick, ref); !changed {
		t.Fatalf("expected left-click to place a sun on an empty cell")
	}
	if a.cells[idx] != tangogame.Sun {
		t.Fatalf("expected first left-click to cycle Empty->Sun, got %v", a.cells[idx])
	}
	if a.cursor != ref.Cell {
		t.Fatalf("expected click to move the cursor to the clicked cell, got %v want %v", a.cursor, ref.Cell)
	}

	a.HandleMouse(leftClick, ref) // sun -> moon
	if a.cells[idx] != tangogame.Moon {
		t.Fatalf("expected second left-click to cycle Sun->Moon, got %v", a.cells[idx])
	}

	rightClick := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseRight}
	if changed := a.HandleMouse(rightClick, ref); !changed {
		t.Fatalf("expected right-click to clear the cell")
	}
	if a.cells[idx] != tangogame.Empty {
		t.Fatalf("expected right-click to clear the cell, got %v", a.cells[idx])
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
		t.Fatalf("expected a motion event (no press) to be a no-op for Tango")
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
	if changed := a.HandleMouse(leftClick, givenRef); changed {
		t.Fatalf("expected left-click on a given cell to be a no-op")
	}
	if a.cells[givenIdx] != before {
		t.Fatalf("given cell was mutated by a click: got %v want %v", a.cells[givenIdx], before)
	}
}

// TestTango_GivenCellsImmutable checks given-cell immutability across every
// keyboard mutation path.
func TestTango_GivenCellsImmutable(t *testing.T) {
	p, _, gen := tangoTestGenerate(t)
	a := tangoNew(gen).(*tangoAdapter)

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
	tangoMoveCursorTo(a, engine.CellAt(givenIdx, p.N))
	before := a.cells[givenIdx]

	for _, k := range []tea.KeyPressMsg{tangoKeySpace(), tangoKeyShiftSpace(), tangoKeyRune('m')} {
		if changed := a.HandleKey(k); changed {
			t.Fatalf("expected key %+v on a given cell to be a no-op", k)
		}
	}
	if a.cells[givenIdx] != before {
		t.Fatalf("given cell was mutated: got %v want %v", a.cells[givenIdx], before)
	}
}

// TestTango_UndoResetHint covers Hint (revealing the recorded solution one
// empty cell at a time), Undo (unwinding one mutation at a time, including
// being a no-op once history is empty), and Reset (back to the
// ungenerated-move state) — plus that none of this ever mutates the
// puzzle's own given cells.
func TestTango_UndoResetHint(t *testing.T) {
	p, sol, gen := tangoTestGenerate(t)
	a := tangoNew(gen).(*tangoAdapter)

	idx1 := tangoFirstNonGivenIdx(t, a)
	a.Hint()
	if a.cells[idx1] != sol.Cells[idx1] {
		t.Fatalf("expected first hint to fill cell %d with %v, got %v", idx1, sol.Cells[idx1], a.cells[idx1])
	}
	wantCursor := engine.CellAt(idx1, p.N)
	if a.cursor != wantCursor {
		t.Fatalf("expected hint to move the cursor to %v, got %v", wantCursor, a.cursor)
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
		t.Fatalf("expected second hint to fill cell %d with %v, got %v", idx2, sol.Cells[idx2], a.cells[idx2])
	}

	a.Undo()
	if a.cells[idx2] != tangogame.Empty {
		t.Fatalf("expected undo to revert the second hint, got %v", a.cells[idx2])
	}
	a.Undo()
	if a.cells[idx1] != tangogame.Empty {
		t.Fatalf("expected undo to revert the first hint, got %v", a.cells[idx1])
	}

	// Undo past the bottom of history is a no-op, not a panic.
	a.Undo()
	if a.cells[idx1] != tangogame.Empty {
		t.Fatalf("expected extra undo beyond history to stay a no-op")
	}

	for i, sym := range p.Givens {
		if a.cells[i] != sym {
			t.Fatalf("given cell %d was mutated by play: got %v want %v", i, a.cells[i], sym)
		}
	}

	a.Hint()
	a.Hint()
	a.Reset()
	for idx, sym := range a.cells {
		if _, given := p.Givens[idx]; given {
			continue
		}
		if sym != tangogame.Empty {
			t.Fatalf("expected reset to clear non-given cell %d, got %v", idx, sym)
		}
	}
	if a.cursor != (engine.Cell{}) {
		t.Fatalf("expected reset to move the cursor back to the origin, got %v", a.cursor)
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
	before := append([]tangogame.Symbol(nil), a.cells...)
	a.Hint()
	for i := range a.cells {
		if a.cells[i] != before[i] {
			t.Fatalf("expected Hint on a full board to be a no-op, cell %d changed", i)
		}
	}
}

// TestTango_GeometryMatchesRenderedView asserts GridGeometry's arithmetic
// (rows/cols, cell size, gutters) matches the actual dimensions of the grid
// portion of View's output, which is what makes the shell's mouse
// hit-testing (tui.CellFromPoint) land on the right cells.
func TestTango_GeometryMatchesRenderedView(t *testing.T) {
	_, _, gen := tangoTestGenerate(t)
	a := tangoNew(gen).(*tangoAdapter)

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

// TestTango_ViewShowsGivensAndHelp is a light content/structure check (per
// 04-testing-strategy.md: substrings, not golden frames) that the renderer
// actually reflects puzzle state and advertises the active binding set.
func TestTango_ViewShowsGivensAndHelp(t *testing.T) {
	_, _, gen := tangoTestGenerate(t)
	a := tangoNew(gen).(*tangoAdapter)
	view := a.View(tui.Grey())

	if !strings.Contains(view, "☀") && !strings.Contains(view, "☾") {
		t.Fatalf("expected View to render at least one given sun/moon glyph, got:\n%s", view)
	}

	if tui.EnhancedKeyboardActive() {
		if !strings.Contains(view, "Shift+Space") {
			t.Fatalf("expected help line to advertise Shift+Space when enhanced keyboard is active")
		}
	} else if !strings.Contains(view, "m: moon") {
		t.Fatalf("expected help line to advertise the legacy 'm' fallback on a legacy keyboard")
	}
}
