package boards

import (
	"bytes"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	queensgame "github.com/Jensen95/tui-games/internal/games/queens"
	"github.com/Jensen95/tui-games/internal/tui"
)

// ---------------------------------------------------------------------------
// Test helpers.
// ---------------------------------------------------------------------------

func queensKeyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func queensKeySpace() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
}

// queensKeyShiftSpace encodes the enhanced-terminal Shift+Space shape (see
// internal/tui/keys_test.go's fixtures): the base Code stays KeySpace and
// Mod carries ModShift.
func queensKeyShiftSpace() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModShift}
}

func queensKeyX() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'x', Text: "x"}
}

// queensMoveCursorTo drives the adapter's cursor from wherever it is to
// target via plain wasd key presses, the same input path a real player's
// movement would take.
func queensMoveCursorTo(a *queensAdapter, target engine.Cell) {
	for a.cursor.Row < target.Row {
		a.HandleKey(queensKeyRune('s'))
	}
	for a.cursor.Row > target.Row {
		a.HandleKey(queensKeyRune('w'))
	}
	for a.cursor.Col < target.Col {
		a.HandleKey(queensKeyRune('d'))
	}
	for a.cursor.Col > target.Col {
		a.HandleKey(queensKeyRune('a'))
	}
}

// queensPlaceQueenViaSecondary drives the cursor to cell and uses the
// secondary action (Shift+Space) to place a queen there directly, the
// two-handed scheme's intended path for a full solve.
func queensPlaceQueenViaSecondary(a *queensAdapter, cell engine.Cell) {
	queensMoveCursorTo(a, cell)
	a.HandleKey(queensKeyShiftSpace())
}

func queensGenerate(t *testing.T, seed int64) (engine.Generated, queensgame.Puzzle, queensgame.Solution) {
	t.Helper()
	gen, err := queensgame.Entry().Generate(engine.Easy, engine.NewRand(seed))
	if err != nil {
		t.Fatalf("Generate(seed=%d) failed: %v", seed, err)
	}
	p, ok := gen.Puzzle.(queensgame.Puzzle)
	if !ok {
		t.Fatalf("Generate returned Puzzle of type %T, want queensgame.Puzzle", gen.Puzzle)
	}
	sol, ok := gen.Solution.(queensgame.Solution)
	if !ok {
		t.Fatalf("Generate returned Solution of type %T, want queensgame.Solution", gen.Solution)
	}
	return gen, p, sol
}

// queensTinyPuzzle is a hand-built 4x4 puzzle small enough to reason about by
// hand: each region is one full row (region id == row), so it's trivially
// valid/connected, and its solution is a real 4-queens-with-no-touch
// placement (verified by hand in the design notes: every pair of queens has
// chebyshev distance >= 2).
func queensTinyPuzzle(givens []int) queensgame.Puzzle {
	region := make([]int, 16)
	for i := range region {
		region[i] = i / 4
	}
	return queensgame.Puzzle{N: 4, Region: region, Givens: givens, SeedV: 1, DiffV: engine.Easy}
}

func queensTinySolution() queensgame.Solution {
	return queensgame.Solution{N: 4, QueenAt: []int{1, 3, 0, 2}}
}

func queensNewTinyAdapter(givens []int) *queensAdapter {
	gen := engine.Generated{Puzzle: queensTinyPuzzle(givens), Solution: queensTinySolution()}
	a, _ := queensNew(gen).(*queensAdapter)
	return a
}

// ---------------------------------------------------------------------------
// 1. Scripted full solve — the proof the adapter works end to end.
// ---------------------------------------------------------------------------

func TestQueens_ScriptedFullSolve(t *testing.T) {
	gen, puzzle, sol := queensGenerate(t, 7)
	cluesBefore, err := queensgame.Encode(puzzle)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	a, ok := queensNew(gen).(*queensAdapter)
	if !ok {
		t.Fatalf("queensNew returned %T, want *queensAdapter", queensNew(gen))
	}

	if a.Solved() {
		t.Fatalf("fresh board reports Solved() = true")
	}

	givenSet := make(map[int]bool, len(puzzle.Givens))
	for _, g := range puzzle.Givens {
		givenSet[g] = true
	}

	for row, col := range sol.QueenAt {
		idx := row*puzzle.N + col
		if givenSet[idx] {
			continue // already placed by the puzzle's givens
		}
		queensPlaceQueenViaSecondary(a, engine.Cell{Row: row, Col: col})
	}

	if violations := a.Violations(); len(violations) != 0 {
		t.Fatalf("Violations() after full scripted solve = %+v, want empty", violations)
	}
	if !a.Solved() {
		t.Fatalf("Solved() = false after placing every solution queen via the keyboard scheme")
	}

	// Given-cell immutability: driving the whole solve must never touch the
	// puzzle's own clue data.
	if cluesAfter, err := queensgame.Encode(puzzle); err != nil {
		t.Fatalf("Encode: %v", err)
	} else if !bytes.Equal(cluesBefore, cluesAfter) {
		t.Fatalf("puzzle clues mutated by play: before=%s after=%s", cluesBefore, cluesAfter)
	}
}

// TestQueens_ScriptedFullSolve_MultipleSeeds sweeps a few more seeds so the
// scripted-solve proof isn't a fluke of one particular region shape.
func TestQueens_ScriptedFullSolve_MultipleSeeds(t *testing.T) {
	for _, seed := range []int64{1, 3, 42} {
		gen, puzzle, sol := queensGenerate(t, seed)
		a, ok := queensNew(gen).(*queensAdapter)
		if !ok {
			t.Fatalf("seed %d: queensNew returned unexpected type", seed)
		}
		givenSet := make(map[int]bool, len(puzzle.Givens))
		for _, g := range puzzle.Givens {
			givenSet[g] = true
		}
		for row, col := range sol.QueenAt {
			idx := row*puzzle.N + col
			if givenSet[idx] {
				continue
			}
			queensPlaceQueenViaSecondary(a, engine.Cell{Row: row, Col: col})
		}
		if !a.Solved() {
			t.Errorf("seed %d: Solved() = false after full scripted solve", seed)
		}
		if v := a.Violations(); len(v) != 0 {
			t.Errorf("seed %d: Violations() = %+v, want empty", seed, v)
		}
	}
}

// ---------------------------------------------------------------------------
// 2. Secondary action: Shift+Space, and the legacy fallback (bare Space
// cycle + 'x' direct mark).
// ---------------------------------------------------------------------------

func TestQueens_SecondaryAction_ShiftSpacePlacesQueenAndClears(t *testing.T) {
	a := queensNewTinyAdapter(nil)
	cell := engine.Cell{Row: 0, Col: 1}
	queensMoveCursorTo(a, cell)

	if changed := a.HandleKey(queensKeyShiftSpace()); !changed {
		t.Fatalf("Shift+Space reported changed=false, want true")
	}
	idx := engine.Index(cell, a.puzzle.N)
	if a.state[idx] != queensCellQueen {
		t.Fatalf("state after Shift+Space = %v, want queensCellQueen", a.state[idx])
	}

	// Again clears, per the two-handed scheme.
	if changed := a.HandleKey(queensKeyShiftSpace()); !changed {
		t.Fatalf("second Shift+Space reported changed=false, want true")
	}
	if a.state[idx] != queensCellEmpty {
		t.Fatalf("state after second Shift+Space = %v, want queensCellEmpty", a.state[idx])
	}
}

func TestQueens_SecondaryAction_ShiftSpaceOverwritesMark(t *testing.T) {
	a := queensNewTinyAdapter(nil)
	cell := engine.Cell{Row: 2, Col: 2}
	queensMoveCursorTo(a, cell)

	a.HandleKey(queensKeyX()) // mark X first
	idx := engine.Index(cell, a.puzzle.N)
	if a.state[idx] != queensCellMarked {
		t.Fatalf("setup: expected marked cell")
	}

	if changed := a.HandleKey(queensKeyShiftSpace()); !changed {
		t.Fatalf("Shift+Space over a mark reported changed=false, want true")
	}
	if a.state[idx] != queensCellQueen {
		t.Fatalf("state after Shift+Space over a mark = %v, want queensCellQueen", a.state[idx])
	}
}

func TestQueens_LegacyFallback_SpaceCyclesThroughAllThreeStates(t *testing.T) {
	a := queensNewTinyAdapter(nil)
	cell := engine.Cell{Row: 1, Col: 1}
	queensMoveCursorTo(a, cell)
	idx := engine.Index(cell, a.puzzle.N)

	if a.state[idx] != queensCellEmpty {
		t.Fatalf("setup: expected empty cell")
	}

	if changed := a.HandleKey(queensKeySpace()); !changed || a.state[idx] != queensCellMarked {
		t.Fatalf("1st Space: changed=%v state=%v, want true/queensCellMarked", changed, a.state[idx])
	}
	if changed := a.HandleKey(queensKeySpace()); !changed || a.state[idx] != queensCellQueen {
		t.Fatalf("2nd Space: changed=%v state=%v, want true/queensCellQueen", changed, a.state[idx])
	}
	if changed := a.HandleKey(queensKeySpace()); !changed || a.state[idx] != queensCellEmpty {
		t.Fatalf("3rd Space: changed=%v state=%v, want true/queensCellEmpty (full cycle back)", changed, a.state[idx])
	}
}

func TestQueens_LegacyFallback_XMarksDirectly(t *testing.T) {
	a := queensNewTinyAdapter(nil)
	cell := engine.Cell{Row: 3, Col: 0}
	queensMoveCursorTo(a, cell)
	idx := engine.Index(cell, a.puzzle.N)

	if changed := a.HandleKey(queensKeyX()); !changed {
		t.Fatalf("'x' reported changed=false, want true")
	}
	if a.state[idx] != queensCellMarked {
		t.Fatalf("state after 'x' = %v, want queensCellMarked", a.state[idx])
	}

	// Pressing 'x' again on an already-marked cell is a no-op (direct set,
	// not a toggle).
	if changed := a.HandleKey(queensKeyX()); changed {
		t.Fatalf("repeated 'x' on an already-marked cell reported changed=true, want false")
	}
}

// ---------------------------------------------------------------------------
// 3. Mouse: click cycles; click-drag paints X marks; right-click clears.
// ---------------------------------------------------------------------------

func TestQueens_Mouse_ClickCyclesCell(t *testing.T) {
	a := queensNewTinyAdapter(nil)
	press := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseLeft}
	release := tui.MouseEvent{Type: tui.MouseEventRelease}
	cellRef := tui.CellRef{Cell: engine.Cell{Row: 0, Col: 0}, Valid: true}
	idx := engine.Index(cellRef.Cell, a.puzzle.N)

	if changed := a.HandleMouse(press, cellRef); !changed || a.state[idx] != queensCellMarked {
		t.Fatalf("press 1: changed=%v state=%v, want true/queensCellMarked", changed, a.state[idx])
	}
	a.HandleMouse(release, cellRef)

	if changed := a.HandleMouse(press, cellRef); !changed || a.state[idx] != queensCellQueen {
		t.Fatalf("press 2: changed=%v state=%v, want true/queensCellQueen", changed, a.state[idx])
	}
	a.HandleMouse(release, cellRef)
}

func TestQueens_Mouse_DragPaintsXAcrossCells(t *testing.T) {
	a := queensNewTinyAdapter(nil)
	press := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseLeft}
	motion := tui.MouseEvent{Type: tui.MouseEventMotion}
	release := tui.MouseEvent{Type: tui.MouseEventRelease}

	start := engine.Cell{Row: 1, Col: 0}
	a.HandleMouse(press, tui.CellRef{Cell: start, Valid: true})
	startIdx := engine.Index(start, a.puzzle.N)
	if a.state[startIdx] != queensCellMarked {
		t.Fatalf("press did not mark the start cell: state=%v", a.state[startIdx])
	}

	next := engine.Cell{Row: 1, Col: 1}
	if changed := a.HandleMouse(motion, tui.CellRef{Cell: next, Valid: true}); !changed {
		t.Fatalf("drag motion onto a new cell reported changed=false, want true")
	}
	nextIdx := engine.Index(next, a.puzzle.N)
	if a.state[nextIdx] != queensCellMarked {
		t.Fatalf("state after drag motion = %v, want queensCellMarked (painted)", a.state[nextIdx])
	}

	// Re-entering the start cell during the same drag must not repaint/toggle it.
	if changed := a.HandleMouse(motion, tui.CellRef{Cell: start, Valid: true}); changed {
		t.Fatalf("re-entering the start cell mid-drag reported changed=true, want false")
	}

	a.HandleMouse(release, tui.CellRef{Cell: next, Valid: true})
	if a.dragging {
		t.Fatalf("dragging still true after release")
	}

	// Motion after release must not paint.
	third := engine.Cell{Row: 2, Col: 2}
	if changed := a.HandleMouse(motion, tui.CellRef{Cell: third, Valid: true}); changed {
		t.Fatalf("motion after release reported changed=true, want false")
	}
}

func TestQueens_Mouse_RightClickClears(t *testing.T) {
	a := queensNewTinyAdapter(nil)
	cell := engine.Cell{Row: 0, Col: 2}
	queensMoveCursorTo(a, cell)
	a.HandleKey(queensKeyShiftSpace()) // place a queen
	idx := engine.Index(cell, a.puzzle.N)
	if a.state[idx] != queensCellQueen {
		t.Fatalf("setup: expected a queen at %+v", cell)
	}

	rightPress := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseRight}
	if changed := a.HandleMouse(rightPress, tui.CellRef{Cell: cell, Valid: true}); !changed {
		t.Fatalf("right-click reported changed=false, want true")
	}
	if a.state[idx] != queensCellEmpty {
		t.Fatalf("state after right-click = %v, want queensCellEmpty", a.state[idx])
	}

	// Right-click on an already-empty cell is a no-op.
	if changed := a.HandleMouse(rightPress, tui.CellRef{Cell: cell, Valid: true}); changed {
		t.Fatalf("right-click on an empty cell reported changed=true, want false")
	}
}

func TestQueens_Mouse_InvalidCellIsNoop(t *testing.T) {
	a := queensNewTinyAdapter(nil)
	press := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseLeft}
	if changed := a.HandleMouse(press, tui.CellRef{Valid: false}); changed {
		t.Fatalf("press on an invalid cell reported changed=true, want false")
	}
	if a.dragging {
		t.Fatalf("press on an invalid cell started a drag")
	}
}

// ---------------------------------------------------------------------------
// 4. Undo / Reset / Hint, and given-cell immutability.
// ---------------------------------------------------------------------------

func TestQueens_Undo(t *testing.T) {
	a := queensNewTinyAdapter(nil)
	cell := engine.Cell{Row: 0, Col: 1}
	queensMoveCursorTo(a, cell)
	a.HandleKey(queensKeyShiftSpace())
	idx := engine.Index(cell, a.puzzle.N)
	if a.state[idx] != queensCellQueen {
		t.Fatalf("setup: place failed")
	}

	a.Undo()
	if a.state[idx] != queensCellEmpty {
		t.Fatalf("state after Undo = %v, want queensCellEmpty", a.state[idx])
	}

	// Undo on an empty history is a no-op, not a panic.
	a.Undo()
	if a.state[idx] != queensCellEmpty {
		t.Fatalf("state after Undo-on-empty-history = %v, want queensCellEmpty", a.state[idx])
	}
}

func TestQueens_Undo_MultipleSteps(t *testing.T) {
	a := queensNewTinyAdapter(nil)
	queensPlaceQueenViaSecondary(a, engine.Cell{Row: 0, Col: 1})
	queensPlaceQueenViaSecondary(a, engine.Cell{Row: 1, Col: 3})

	idx0 := engine.Index(engine.Cell{Row: 0, Col: 1}, a.puzzle.N)
	idx1 := engine.Index(engine.Cell{Row: 1, Col: 3}, a.puzzle.N)
	if a.state[idx0] != queensCellQueen || a.state[idx1] != queensCellQueen {
		t.Fatalf("setup: expected both queens placed")
	}

	a.Undo() // undoes the second placement
	if a.state[idx1] != queensCellEmpty {
		t.Fatalf("state[idx1] after one Undo = %v, want queensCellEmpty", a.state[idx1])
	}
	if a.state[idx0] != queensCellQueen {
		t.Fatalf("state[idx0] after one Undo = %v, want still queensCellQueen", a.state[idx0])
	}

	a.Undo() // undoes the first placement too
	if a.state[idx0] != queensCellEmpty {
		t.Fatalf("state[idx0] after two Undos = %v, want queensCellEmpty", a.state[idx0])
	}
}

func TestQueens_Reset(t *testing.T) {
	a := queensNewTinyAdapter(nil)
	queensPlaceQueenViaSecondary(a, engine.Cell{Row: 0, Col: 1})
	queensMoveCursorTo(a, engine.Cell{Row: 2, Col: 2})

	a.Reset()

	if a.cursor != (engine.Cell{}) {
		t.Fatalf("Reset left cursor at %+v, want zero value", a.cursor)
	}
	if len(a.history) != 0 {
		t.Fatalf("Reset left %d history entries, want 0", len(a.history))
	}
	for i, s := range a.state {
		if s != queensCellEmpty {
			t.Fatalf("state[%d] = %v after Reset, want queensCellEmpty", i, s)
		}
	}
	if a.Solved() {
		t.Fatalf("Reset board reports Solved() = true")
	}
	// Undo after Reset must be a safe no-op (history was cleared).
	a.Undo()
}

func TestQueens_Reset_RestoresGivens(t *testing.T) {
	givenIdx := 1 // row0, col1 — matches queensTinySolution's row 0
	a := queensNewTinyAdapter([]int{givenIdx})
	if a.state[givenIdx] != queensCellQueen {
		t.Fatalf("setup: given cell should start as a queen")
	}

	queensPlaceQueenViaSecondary(a, engine.Cell{Row: 1, Col: 3})
	a.Reset()

	if a.state[givenIdx] != queensCellQueen {
		t.Fatalf("state[given] after Reset = %v, want queensCellQueen (re-seeded)", a.state[givenIdx])
	}
}

func TestQueens_Hint_PlacesOneCorrectQueenAndProgresses(t *testing.T) {
	a := queensNewTinyAdapter(nil)

	a.Hint()
	idx0 := engine.Index(engine.Cell{Row: 0, Col: 1}, a.puzzle.N)
	if a.state[idx0] != queensCellQueen {
		t.Fatalf("after first Hint, state[row0] = %v, want queensCellQueen at col 1", a.state[idx0])
	}
	if a.Solved() {
		t.Fatalf("Solved() = true after only one hint on a four-row puzzle")
	}

	a.Hint()
	a.Hint()
	a.Hint()
	if !a.Solved() {
		t.Fatalf("Solved() = false after hinting every row")
	}
	if v := a.Violations(); len(v) != 0 {
		t.Fatalf("Violations() = %+v after hinting the whole puzzle, want empty", v)
	}

	// A further Hint on an already-solved board is a no-op.
	before := append([]queensCellState(nil), a.state...)
	a.Hint()
	for i, s := range a.state {
		if s != before[i] {
			t.Fatalf("Hint on a solved board changed state[%d]: %v -> %v", i, before[i], s)
		}
	}
}

func TestQueens_Hint_OverwritesAWrongGuess(t *testing.T) {
	a := queensNewTinyAdapter(nil)

	// Wrongly place row 0's queen at col 0 instead of the correct col 1.
	queensPlaceQueenViaSecondary(a, engine.Cell{Row: 0, Col: 0})
	wrongIdx := engine.Index(engine.Cell{Row: 0, Col: 0}, a.puzzle.N)
	if a.state[wrongIdx] != queensCellQueen {
		t.Fatalf("setup: expected wrong guess placed")
	}

	a.Hint()
	correctIdx := engine.Index(engine.Cell{Row: 0, Col: 1}, a.puzzle.N)
	if a.state[correctIdx] != queensCellQueen {
		t.Fatalf("after Hint over a wrong guess, state[correct] = %v, want queensCellQueen", a.state[correctIdx])
	}
	if a.state[wrongIdx] != queensCellEmpty {
		t.Fatalf("after Hint over a wrong guess, state[wrong] = %v, want queensCellEmpty (cleared)", a.state[wrongIdx])
	}
}

func TestQueens_Hint_NoopWithoutSolution(t *testing.T) {
	a := queensNewTinyAdapter(nil)
	a.hasSolution = false
	a.Hint() // must not panic
	for i, s := range a.state {
		if s != queensCellEmpty {
			t.Fatalf("state[%d] = %v after Hint with no solution, want queensCellEmpty", i, s)
		}
	}
}

func TestQueens_GivenCellImmutability(t *testing.T) {
	givenIdx := 1 // row0, col1
	a := queensNewTinyAdapter([]int{givenIdx})
	givenCell := engine.CellAt(givenIdx, a.puzzle.N)

	queensMoveCursorTo(a, givenCell)
	if changed := a.HandleKey(queensKeySpace()); changed {
		t.Fatalf("Space on a given cell reported changed=true, want false")
	}
	if changed := a.HandleKey(queensKeyShiftSpace()); changed {
		t.Fatalf("Shift+Space on a given cell reported changed=true, want false")
	}
	if changed := a.HandleKey(queensKeyX()); changed {
		t.Fatalf("'x' on a given cell reported changed=true, want false")
	}
	press := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseLeft}
	if changed := a.HandleMouse(press, tui.CellRef{Cell: givenCell, Valid: true}); changed {
		t.Fatalf("mouse click on a given cell reported changed=true, want false")
	}
	rightPress := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseRight}
	if changed := a.HandleMouse(rightPress, tui.CellRef{Cell: givenCell, Valid: true}); changed {
		t.Fatalf("right-click on a given cell reported changed=true, want false")
	}
	if a.state[givenIdx] != queensCellQueen {
		t.Fatalf("given cell state mutated: %v, want queensCellQueen throughout", a.state[givenIdx])
	}
}

func TestQueens_Violations_DelegatesToEngineValidator(t *testing.T) {
	a := queensNewTinyAdapter(nil)
	// Two queens sharing a row: same row (0) triggers RuleSameRow.
	queensPlaceQueenViaSecondary(a, engine.Cell{Row: 0, Col: 0})
	queensPlaceQueenViaSecondary(a, engine.Cell{Row: 0, Col: 3})

	violations := a.Violations()
	foundRow := false
	for _, v := range violations {
		if v.Rule == queensgame.RuleSameRow {
			foundRow = true
		}
	}
	if !foundRow {
		t.Fatalf("Violations() = %+v, want a %q violation for two queens in row 0", violations, queensgame.RuleSameRow)
	}
}

// ---------------------------------------------------------------------------
// GridGeometry vs. rendered View consistency.
// ---------------------------------------------------------------------------

func TestQueens_GridGeometryMatchesRenderedView(t *testing.T) {
	gen, _, _ := queensGenerate(t, 7)
	a, ok := queensNew(gen).(*queensAdapter)
	if !ok {
		t.Fatalf("queensNew returned unexpected type")
	}

	for _, theme := range tui.Themes() {
		geo := a.GridGeometry()
		view := a.View(theme)
		lines := queensSplitLines(view)

		wantHeight := geo.Rows*geo.CellHeight + (geo.Rows-1)*geo.RowGutter
		if len(lines) < wantHeight {
			t.Fatalf("theme %s: view has %d lines, want at least %d (grid height)", theme.Name, len(lines), wantHeight)
		}

		wantWidth := geo.Cols*geo.CellWidth + (geo.Cols-1)*geo.ColGutter
		for i := 0; i < wantHeight; i++ {
			if w := lipgloss.Width(lines[i]); w != wantWidth {
				t.Fatalf("theme %s: grid line %d width = %d, want %d (line: %q)", theme.Name, i, w, wantWidth, lines[i])
			}
		}

		if geo.OriginX != 0 || geo.OriginY != 0 {
			t.Fatalf("theme %s: origin = (%d,%d), want (0,0) — grid is the first thing View renders", theme.Name, geo.OriginX, geo.OriginY)
		}

		cell, hit := tui.CellFromPoint(geo, geo.CellWidth/2, 0)
		if !hit || cell != (engine.Cell{Row: 0, Col: 0}) {
			t.Fatalf("CellFromPoint at cell (0,0)'s body = %+v, hit=%v, want {0,0},true", cell, hit)
		}
	}
}

// queensSplitLines splits a rendered view into lines.
func queensSplitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}
