package boards

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	zipgame "github.com/Jensen95/tui-games/internal/games/zip"
	"github.com/Jensen95/tui-games/internal/tui"
)

// zipTestGenerate builds a fixed-seed puzzle via the real engine/games entry
// point (never hand-rolled), the same path production code uses.
func zipTestGenerate(t *testing.T) (zipgame.Puzzle, zipgame.Solution, engine.Generated) {
	t.Helper()
	gen, err := zipgame.Entry().Generate(engine.Easy, engine.NewRand(7))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	p, ok := gen.Puzzle.(zipgame.Puzzle)
	if !ok {
		t.Fatalf("generated puzzle has unexpected type %T", gen.Puzzle)
	}
	sol, ok := gen.Solution.(zipgame.Solution)
	if !ok {
		t.Fatalf("generated solution has unexpected type %T", gen.Solution)
	}
	if len(sol.Path) != p.R*p.C {
		t.Fatalf("fixture invariant broken: solution length %d != %d cells", len(sol.Path), p.R*p.C)
	}
	return p, sol, gen
}

// zipKeyForStep returns the wasd key that steps the keyboard cursor from an
// orthogonally adjacent cell "from" to "to", failing the test if the two
// aren't adjacent (which would mean the fixture solution itself is broken).
func zipKeyForStep(t *testing.T, from, to engine.Cell) tea.KeyPressMsg {
	t.Helper()
	dr, dc := to.Row-from.Row, to.Col-from.Col
	switch {
	case dr == -1 && dc == 0:
		return tea.KeyPressMsg{Code: 'w'}
	case dr == 1 && dc == 0:
		return tea.KeyPressMsg{Code: 's'}
	case dr == 0 && dc == -1:
		return tea.KeyPressMsg{Code: 'a'}
	case dr == 0 && dc == 1:
		return tea.KeyPressMsg{Code: 'd'}
	default:
		t.Fatalf("solution step %v -> %v is not orthogonally adjacent", from, to)
		return tea.KeyPressMsg{}
	}
}

// TestZip_ScriptedFullSolve is the proof the adapter works end to end: drive
// it with synthetic tea.KeyPressMsg values along the two-handed scheme
// (Space to lower the pen, wasd to draw) all the way to the recorded
// solution, and confirm Solved() flips true with no violations.
func TestZip_ScriptedFullSolve(t *testing.T) {
	p, sol, gen := zipTestGenerate(t)
	adapter := zipNew(gen)
	ad, ok := adapter.(*zipAdapter)
	if !ok {
		t.Fatalf("zipNew returned unexpected type %T", adapter)
	}

	if ad.Solved() {
		t.Fatalf("fresh adapter must not report solved")
	}

	if changed := ad.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace}); !changed {
		t.Fatalf("expected Space to report a change (pen down at start)")
	}
	if !ad.penDown {
		t.Fatalf("expected pen to be down after Space")
	}
	if len(ad.path) != 1 || ad.path[0] != sol.Path[0] {
		t.Fatalf("expected path to start at solution[0]=%d, got %v", sol.Path[0], ad.path)
	}

	for i := 1; i < len(sol.Path); i++ {
		from := engine.CellAt(sol.Path[i-1], p.C)
		to := engine.CellAt(sol.Path[i], p.C)
		key := zipKeyForStep(t, from, to)
		if changed := ad.HandleKey(key); !changed {
			t.Fatalf("step %d (%v -> %v): expected movement to change the board", i, from, to)
		}
	}

	if len(ad.path) != len(sol.Path) {
		t.Fatalf("expected full path length %d, got %d", len(sol.Path), len(ad.path))
	}
	if v := ad.Violations(); len(v) != 0 {
		t.Fatalf("expected no violations on the completed solution, got %v", v)
	}
	if !ad.Solved() {
		t.Fatalf("expected Solved() to be true after driving the recorded solution")
	}
}

// TestZip_SecondaryActionAndLegacyErase exercises both halves of Zip's
// secondary action: Shift+Space on an enhanced-keyboard KeyPressMsg, and the
// legacy Backspace fallback that must work identically regardless of
// terminal capability.
func TestZip_SecondaryActionAndLegacyErase(t *testing.T) {
	p, sol, gen := zipTestGenerate(t)
	adapter := zipNew(gen)
	ad := adapter.(*zipAdapter)

	ad.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace}) // pen down at start
	stepKey := zipKeyForStep(t, engine.CellAt(sol.Path[0], p.C), engine.CellAt(sol.Path[1], p.C))
	ad.HandleKey(stepKey)
	if len(ad.path) != 2 {
		t.Fatalf("setup: expected path len 2, got %d (%v)", len(ad.path), ad.path)
	}

	shiftSpace := tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModShift}
	if !tui.IsSpace(shiftSpace) || !tui.IsShifted(shiftSpace) {
		t.Fatalf("test fixture key does not look like Shift+Space")
	}
	if changed := ad.HandleKey(shiftSpace); !changed {
		t.Fatalf("expected Shift+Space to report a change")
	}
	if len(ad.path) != 1 {
		t.Fatalf("expected Shift+Space to erase the last segment, got %v", ad.path)
	}

	// Redo the move, then erase it via the legacy Backspace fallback.
	ad.HandleKey(stepKey)
	if len(ad.path) != 2 {
		t.Fatalf("setup: expected path len 2 again, got %d (%v)", len(ad.path), ad.path)
	}
	if changed := ad.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace}); !changed {
		t.Fatalf("expected legacy Backspace to report a change")
	}
	if len(ad.path) != 1 {
		t.Fatalf("expected Backspace to erase the last segment, got %v", ad.path)
	}

	// A bare Space on a legacy terminal is indistinguishable from Shift+Space
	// (no Mod bit at all) and must be treated as the primary action, not the
	// secondary one.
	plainSpace := tea.KeyPressMsg{Code: tea.KeySpace}
	if tui.IsShifted(plainSpace) {
		t.Fatalf("plain space must never look shifted")
	}
}

// TestZip_MouseDrag exercises Zip's defining interaction: click-drag from
// the path's start through cells, with a drag reversal retracting the path,
// via press -> motion(while held) -> release.
func TestZip_MouseDrag(t *testing.T) {
	p, sol, gen := zipTestGenerate(t)
	ad := zipNew(gen).(*zipAdapter)

	press := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseLeft}
	motion := tui.MouseEvent{Type: tui.MouseEventMotion}

	startRef := tui.CellRef{Cell: engine.CellAt(sol.Path[0], p.C), Valid: true}
	if changed := ad.HandleMouse(press, startRef); !changed {
		t.Fatalf("expected press on the start cell to begin the path")
	}
	if !ad.dragging {
		t.Fatalf("expected press to enter dragging state")
	}
	if len(ad.path) != 1 || ad.path[0] != sol.Path[0] {
		t.Fatalf("expected path=[%d], got %v", sol.Path[0], ad.path)
	}

	const drawn = 4
	for i := 1; i < drawn && i < len(sol.Path); i++ {
		ref := tui.CellRef{Cell: engine.CellAt(sol.Path[i], p.C), Valid: true}
		if changed := ad.HandleMouse(motion, ref); !changed {
			t.Fatalf("motion step %d: expected drag to extend the path", i)
		}
	}
	if len(ad.path) != drawn {
		t.Fatalf("expected path len %d after dragging forward, got %d (%v)", drawn, len(ad.path), ad.path)
	}

	// Dragging backwards onto an already-drawn (non-tail) cell retracts to it.
	backIdx := 2
	backRef := tui.CellRef{Cell: engine.CellAt(sol.Path[backIdx], p.C), Valid: true}
	if changed := ad.HandleMouse(motion, backRef); !changed {
		t.Fatalf("expected dragging back over an earlier path cell to retract")
	}
	if len(ad.path) != backIdx+1 {
		t.Fatalf("expected retraction to len %d, got %d (%v)", backIdx+1, len(ad.path), ad.path)
	}

	release := tui.MouseEvent{Type: tui.MouseEventRelease}
	ad.HandleMouse(release, backRef)
	if ad.dragging {
		t.Fatalf("expected release to clear the dragging state")
	}

	// Pressing off the current path (with a path already in progress) must
	// not clobber it.
	offIdx := -1
	inPath := make(map[int]bool, len(ad.path))
	for _, idx := range ad.path {
		inPath[idx] = true
	}
	for idx := 0; idx < p.R*p.C; idx++ {
		if !inPath[idx] {
			offIdx = idx
			break
		}
	}
	if offIdx < 0 {
		t.Fatalf("test setup: no off-path cell available")
	}
	offRef := tui.CellRef{Cell: engine.CellAt(offIdx, p.C), Valid: true}
	pathBefore := append([]int(nil), ad.path...)
	if changed := ad.HandleMouse(press, offRef); changed {
		t.Fatalf("expected press off the current path to be a no-op")
	}
	if !equalIntSlices(ad.path, pathBefore) {
		t.Fatalf("expected path to be unchanged by an off-path press, got %v want %v", ad.path, pathBefore)
	}
}

func equalIntSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestZip_UndoResetHint covers Hint (walking the recorded solution one cell
// at a time), Undo (unwinding one mutation at a time, including being a
// no-op once history is empty), and Reset (back to the ungenerated-move
// state) — plus that none of this ever mutates the puzzle's own waypoint
// data (Zip's equivalent of given-cell immutability).
func TestZip_UndoResetHint(t *testing.T) {
	p, sol, gen := zipTestGenerate(t)
	ad := zipNew(gen).(*zipAdapter)

	ad.Hint()
	if len(ad.path) != 1 || ad.path[0] != sol.Path[0] {
		t.Fatalf("expected first hint to plant solution[0]=%d, got %v", sol.Path[0], ad.path)
	}
	if !ad.penDown {
		t.Fatalf("expected hint to put the pen down")
	}

	ad.Hint()
	if len(ad.path) != 2 || ad.path[1] != sol.Path[1] {
		t.Fatalf("expected second hint to plant solution[1]=%d, got %v", sol.Path[1], ad.path)
	}

	ad.Undo()
	if len(ad.path) != 1 {
		t.Fatalf("expected undo to revert to len 1, got %d (%v)", len(ad.path), ad.path)
	}
	ad.Undo()
	if len(ad.path) != 0 {
		t.Fatalf("expected undo to revert to an empty path, got %v", ad.path)
	}

	// Undo past the bottom of history is a no-op, not a panic.
	ad.Undo()
	if len(ad.path) != 0 {
		t.Fatalf("expected extra undo beyond history to stay a no-op")
	}

	wantStart, ok := zipStartCell(p)
	if !ok {
		t.Fatalf("fixture puzzle has no start cell")
	}
	gotStart, ok := zipStartCell(ad.puzzle)
	if !ok || gotStart != wantStart {
		t.Fatalf("puzzle waypoint data was mutated by play: got start=%d ok=%v want=%d", gotStart, ok, wantStart)
	}

	ad.Hint()
	ad.Hint()
	ad.Reset()
	if len(ad.path) != 0 {
		t.Fatalf("expected reset to clear the path, got %v", ad.path)
	}
	if ad.penDown {
		t.Fatalf("expected reset to lift the pen")
	}
	if len(ad.history) != 0 {
		t.Fatalf("expected reset to clear undo history, got %d entries", len(ad.history))
	}
	wantCursor := engine.CellAt(wantStart, p.C)
	if ad.cursor != wantCursor {
		t.Fatalf("expected reset to put the cursor back on the start cell, got %v want %v", ad.cursor, wantCursor)
	}
}

// TestZip_GeometryMatchesRenderedView asserts GridGeometry's arithmetic
// (rows/cols, cell size, gutters) matches the actual dimensions of the grid
// portion of View's output, which is what makes the shell's mouse
// hit-testing (tui.CellFromPoint) land on the right cells.
func TestZip_GeometryMatchesRenderedView(t *testing.T) {
	_, _, gen := zipTestGenerate(t)
	ad := zipNew(gen).(*zipAdapter)

	view := ad.View(tui.Dark())
	geo := ad.GridGeometry()

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

// TestZip_ViewShowsWaypointsAndCursor is a light content/structure check
// (per 04-testing-strategy.md: substrings, not golden frames) that the
// renderer actually reflects puzzle state: waypoint numbers appear, and the
// help line advertises the active binding set for the current keyboard mode.
func TestZip_ViewShowsWaypointsAndCursor(t *testing.T) {
	_, _, gen := zipTestGenerate(t)
	ad := zipNew(gen).(*zipAdapter)
	view := ad.View(tui.Grey())

	// The start cell (waypoint 1) must always render as a two-space-padded
	// "1", per renderCell's waypoint formatting.
	if !strings.Contains(view, " 1 ") {
		t.Fatalf("expected View to render the start waypoint's number, got:\n%s", view)
	}

	if !strings.Contains(view, "remaining:") {
		t.Fatalf("expected View to show a remaining-cell count, got:\n%s", view)
	}
	if !strings.Contains(view, "pen: up") {
		t.Fatalf("expected fresh adapter's View to report pen up, got:\n%s", view)
	}
	if tui.EnhancedKeyboardActive() {
		if !strings.Contains(view, "Shift+Space") {
			t.Fatalf("expected help line to advertise Shift+Space when enhanced keyboard is active")
		}
	} else if !strings.Contains(view, "Backspace") {
		t.Fatalf("expected help line to advertise the Backspace fallback on a legacy keyboard")
	}
}
