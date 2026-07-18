package boards

import (
	"bytes"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	pz "github.com/Jensen95/tui-games/internal/games/patches"
	"github.com/Jensen95/tui-games/internal/tui"
)

// ---------------------------------------------------------------------------
// Test helpers.
// ---------------------------------------------------------------------------

func patchesKeyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func patchesKeySpace() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
}

// patchesKeyShiftSpace encodes the enhanced-terminal Shift+Space shape (see
// internal/tui/keys_test.go's fixtures): the base Code stays KeySpace and
// Mod carries ModShift.
func patchesKeyShiftSpace() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModShift}
}

func patchesKeyX() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'x', Text: "x"}
}

// patchesMoveCursorTo drives the adapter's cursor from wherever it is to
// target via plain wasd key presses (no anchor active), the same input path
// a real player's movement would take.
func patchesMoveCursorTo(a *patchesAdapter, target engine.Cell) {
	for a.cursor.Row < target.Row {
		a.HandleKey(patchesKeyRune('s'))
	}
	for a.cursor.Row > target.Row {
		a.HandleKey(patchesKeyRune('w'))
	}
	for a.cursor.Col < target.Col {
		a.HandleKey(patchesKeyRune('d'))
	}
	for a.cursor.Col > target.Col {
		a.HandleKey(patchesKeyRune('a'))
	}
}

// patchesPlaceRect drives the full anchor -> stretch -> commit sequence for
// one solution rectangle via the keyboard two-handed scheme, starting from
// wherever the cursor currently sits.
func patchesPlaceRect(t *testing.T, a *patchesAdapter, clueIdx int, rect pz.Rect) {
	t.Helper()
	clueCell := engine.CellAt(clueIdx, a.cols)

	patchesMoveCursorTo(a, clueCell)
	if changed := a.HandleKey(patchesKeySpace()); changed {
		t.Fatalf("anchoring at %+v reported changed=true, want false (no board mutation yet)", clueCell)
	}
	if !a.hasAnchor {
		t.Fatalf("expected anchor active after Space on clue cell %+v", clueCell)
	}

	up := clueCell.Row - rect.R0
	down := (rect.R0 + rect.H - 1) - clueCell.Row
	left := clueCell.Col - rect.C0
	right := (rect.C0 + rect.W - 1) - clueCell.Col

	for i := 0; i < up; i++ {
		a.HandleKey(patchesKeyRune('w'))
	}
	for i := 0; i < down; i++ {
		a.HandleKey(patchesKeyRune('s'))
	}
	for i := 0; i < left; i++ {
		a.HandleKey(patchesKeyRune('a'))
	}
	for i := 0; i < right; i++ {
		a.HandleKey(patchesKeyRune('d'))
	}

	if got := a.box; got != (patchesBox{r0: rect.R0, c0: rect.C0, r1: rect.R0 + rect.H - 1, c1: rect.C0 + rect.W - 1}) {
		t.Fatalf("stretched box = %+v, want bounds of %+v", got, rect)
	}

	if changed := a.HandleKey(patchesKeySpace()); !changed {
		t.Fatalf("committing rect %+v (clue %d) reported changed=false, want true", rect, clueIdx)
	}
	if a.hasAnchor {
		t.Fatalf("anchor still active after commit of rect %+v", rect)
	}
}

func patchesGenerate(t *testing.T, seed int64) (engine.Generated, *pz.Puzzle, *pz.Solution) {
	t.Helper()
	gen, err := pz.Entry().Generate(engine.Easy, engine.NewRand(seed))
	if err != nil {
		t.Fatalf("Generate(seed=%d) failed: %v", seed, err)
	}
	p, ok := gen.Puzzle.(*pz.Puzzle)
	if !ok {
		t.Fatalf("Generate returned Puzzle of type %T, want *pz.Puzzle", gen.Puzzle)
	}
	sol, ok := gen.Solution.(*pz.Solution)
	if !ok {
		t.Fatalf("Generate returned Solution of type %T, want *pz.Solution", gen.Solution)
	}
	return gen, p, sol
}

// ---------------------------------------------------------------------------
// 1. Scripted full solve — the proof the adapter works end to end.
// ---------------------------------------------------------------------------

func TestPatches_ScriptedFullSolve(t *testing.T) {
	gen, puzzle, _ := patchesGenerate(t, 7)
	cluesBefore := append([]byte(nil), pz.Encode(puzzle)...)

	a, ok := patchesNew(gen).(*patchesAdapter)
	if !ok {
		t.Fatalf("patchesNew returned %T, want *patchesAdapter", patchesNew(gen))
	}

	if a.Solved() {
		t.Fatalf("fresh board reports Solved() = true")
	}

	cands := a.sortedSolutionRects()
	if len(cands) == 0 {
		t.Fatalf("generated puzzle has no clues to solve")
	}

	for _, cand := range cands {
		patchesPlaceRect(t, a, cand.clueIdx, cand.rect)
	}

	if violations := a.Violations(); len(violations) != 0 {
		t.Fatalf("Violations() after full scripted solve = %+v, want empty", violations)
	}
	if !a.Solved() {
		t.Fatalf("Solved() = false after placing every solution rectangle via the keyboard scheme")
	}

	// Given-cell immutability: driving the whole solve must never touch the
	// puzzle's own clue data.
	if cluesAfter := pz.Encode(puzzle); !bytes.Equal(cluesBefore, cluesAfter) {
		t.Fatalf("puzzle clues mutated by play: before=%s after=%s", cluesBefore, cluesAfter)
	}
}

// TestPatches_ScriptedFullSolve_MultipleSeeds sweeps a few more seeds so the
// scripted-solve proof isn't a fluke of one particular partition shape.
func TestPatches_ScriptedFullSolve_MultipleSeeds(t *testing.T) {
	for _, seed := range []int64{1, 3, 42} {
		gen, _, _ := patchesGenerate(t, seed)
		a, ok := patchesNew(gen).(*patchesAdapter)
		if !ok {
			t.Fatalf("seed %d: patchesNew returned unexpected type", seed)
		}
		for _, cand := range a.sortedSolutionRects() {
			patchesPlaceRect(t, a, cand.clueIdx, cand.rect)
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
// 2. Secondary action: Shift+Space (enhanced) and the legacy 'x' fallback.
// ---------------------------------------------------------------------------

func patchesTinyPuzzle() *pz.Puzzle {
	// 2x2 grid, two 2x1 "wide" rectangles stacked — small enough to reason
	// about by hand, with clues at (0,0)=idx0 and (1,0)=idx2.
	return &pz.Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]pz.Clue{
			0: {Number: 2, Shape: pz.Wide},
			2: {Number: 2, Shape: pz.Wide},
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}
}

func patchesTinySolution() *pz.Solution {
	return &pz.Solution{Rects: []pz.Rect{
		{R0: 0, C0: 0, W: 2, H: 1},
		{R0: 1, C0: 0, W: 2, H: 1},
	}}
}

func patchesNewTinyAdapter() *patchesAdapter {
	p := patchesTinyPuzzle()
	return &patchesAdapter{
		puzzle:    p,
		solution:  patchesTinySolution(),
		board:     pz.NewBoard(p),
		validator: pz.NewValidator(p),
		rows:      p.R,
		cols:      p.C,
		cellWidth: patchesCellWidth(p),
	}
}

func TestPatches_SecondaryAction_ShiftSpaceCancelsActiveRect(t *testing.T) {
	a := patchesNewTinyAdapter()

	patchesMoveCursorTo(a, engine.Cell{Row: 0, Col: 0})
	a.HandleKey(patchesKeySpace()) // anchor
	if !a.hasAnchor {
		t.Fatalf("expected anchor active")
	}
	a.HandleKey(patchesKeyRune('d')) // stretch to 2 wide

	if changed := a.HandleKey(patchesKeyShiftSpace()); changed {
		t.Fatalf("Shift+Space cancel reported changed=true, want false (no board mutation)")
	}
	if a.hasAnchor {
		t.Fatalf("Shift+Space did not cancel the active rectangle")
	}
	for i, l := range a.board.Cells {
		if l != -1 {
			t.Fatalf("board.Cells[%d] = %d after canceling an uncommitted rect, want -1 (untouched)", i, l)
		}
	}
}

func TestPatches_SecondaryAction_ShiftSpaceRemovesPlacedRect(t *testing.T) {
	a := patchesNewTinyAdapter()

	patchesPlaceRect(t, a, 0, pz.Rect{R0: 0, C0: 0, W: 2, H: 1})
	if a.board.Cells[0] == -1 {
		t.Fatalf("setup: rectangle wasn't placed")
	}

	patchesMoveCursorTo(a, engine.Cell{Row: 0, Col: 1})
	if changed := a.HandleKey(patchesKeyShiftSpace()); !changed {
		t.Fatalf("Shift+Space remove reported changed=false, want true")
	}
	if a.board.Cells[0] != -1 || a.board.Cells[1] != -1 {
		t.Fatalf("board.Cells after remove = %v, want all -1", a.board.Cells)
	}
}

func TestPatches_LegacyFallback_XCancelsAndRemoves(t *testing.T) {
	a := patchesNewTinyAdapter()

	// Cancel path.
	patchesMoveCursorTo(a, engine.Cell{Row: 0, Col: 0})
	a.HandleKey(patchesKeySpace())
	if changed := a.HandleKey(patchesKeyX()); changed {
		t.Fatalf("'x' cancel reported changed=true, want false")
	}
	if a.hasAnchor {
		t.Fatalf("'x' did not cancel the active rectangle")
	}

	// Remove path.
	patchesPlaceRect(t, a, 0, pz.Rect{R0: 0, C0: 0, W: 2, H: 1})
	patchesMoveCursorTo(a, engine.Cell{Row: 0, Col: 0})
	if changed := a.HandleKey(patchesKeyX()); !changed {
		t.Fatalf("'x' remove reported changed=false, want true")
	}
	for i, l := range a.board.Cells {
		if l != -1 {
			t.Fatalf("board.Cells[%d] = %d after 'x' remove, want -1", i, l)
		}
	}
}

// ---------------------------------------------------------------------------
// 3. Mouse: press on a clue, drag to the opposite corner, release commits;
// click a placed rectangle to remove it.
// ---------------------------------------------------------------------------

func TestPatches_Mouse_DragCommitsRectangle(t *testing.T) {
	a := patchesNewTinyAdapter()

	press := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseLeft}
	motion := tui.MouseEvent{Type: tui.MouseEventMotion}
	release := tui.MouseEvent{Type: tui.MouseEventRelease}

	if changed := a.HandleMouse(press, tui.CellRef{Cell: engine.Cell{Row: 0, Col: 0}, Valid: true}); changed {
		t.Fatalf("press reported changed=true, want false")
	}
	if !a.hasAnchor {
		t.Fatalf("press on a clue cell did not start a drag")
	}

	if changed := a.HandleMouse(motion, tui.CellRef{Cell: engine.Cell{Row: 0, Col: 1}, Valid: true}); changed {
		t.Fatalf("motion reported changed=true, want false")
	}
	if want := (patchesBox{r0: 0, c0: 0, r1: 0, c1: 1}); a.box != want {
		t.Fatalf("box after drag motion = %+v, want %+v", a.box, want)
	}

	changed := a.HandleMouse(release, tui.CellRef{Cell: engine.Cell{Row: 0, Col: 1}, Valid: true})
	if !changed {
		t.Fatalf("release reported changed=false, want true (rectangle committed)")
	}
	if a.hasAnchor {
		t.Fatalf("drag still active after release")
	}
	if a.board.Cells[0] == -1 || a.board.Cells[1] == -1 || a.board.Cells[0] != a.board.Cells[1] {
		t.Fatalf("board.Cells after drag-commit = %v, want [0,0] both covered by the same label", a.board.Cells)
	}
}

func TestPatches_Mouse_ClickPlacedRectangleRemovesIt(t *testing.T) {
	a := patchesNewTinyAdapter()
	patchesPlaceRect(t, a, 0, pz.Rect{R0: 0, C0: 0, W: 2, H: 1})

	press := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseLeft}
	changed := a.HandleMouse(press, tui.CellRef{Cell: engine.Cell{Row: 0, Col: 1}, Valid: true})
	if !changed {
		t.Fatalf("clicking a placed rectangle reported changed=false, want true")
	}
	if a.board.Cells[0] != -1 || a.board.Cells[1] != -1 {
		t.Fatalf("board.Cells after click-remove = %v, want all -1", a.board.Cells)
	}
}

func TestPatches_Mouse_InvalidCellIsNoop(t *testing.T) {
	a := patchesNewTinyAdapter()
	press := tui.MouseEvent{Type: tui.MouseEventPress, Button: tea.MouseLeft}
	if changed := a.HandleMouse(press, tui.CellRef{Valid: false}); changed {
		t.Fatalf("press on an invalid cell reported changed=true, want false")
	}
	if a.hasAnchor {
		t.Fatalf("press on an invalid cell started a drag")
	}
}

// ---------------------------------------------------------------------------
// 4. Undo / Reset / Hint, and given-cell immutability.
// ---------------------------------------------------------------------------

func TestPatches_Undo(t *testing.T) {
	a := patchesNewTinyAdapter()
	patchesPlaceRect(t, a, 0, pz.Rect{R0: 0, C0: 0, W: 2, H: 1})
	if a.board.Cells[0] == -1 {
		t.Fatalf("setup: place failed")
	}

	a.Undo()
	for i, l := range a.board.Cells {
		if l != -1 {
			t.Fatalf("board.Cells[%d] = %d after Undo, want -1", i, l)
		}
	}

	// Undo on an empty history is a no-op, not a panic.
	a.Undo()
	for i, l := range a.board.Cells {
		if l != -1 {
			t.Fatalf("board.Cells[%d] = %d after Undo-on-empty-history, want -1", i, l)
		}
	}
}

func TestPatches_Undo_MultipleSteps(t *testing.T) {
	a := patchesNewTinyAdapter()
	patchesPlaceRect(t, a, 0, pz.Rect{R0: 0, C0: 0, W: 2, H: 1})
	patchesPlaceRect(t, a, 2, pz.Rect{R0: 1, C0: 0, W: 2, H: 1})
	if !a.Solved() {
		t.Fatalf("setup: expected solved tiny puzzle")
	}

	a.Undo() // undoes the second placement
	if a.board.Cells[2] != -1 || a.board.Cells[3] != -1 {
		t.Fatalf("board.Cells after one Undo = %v, want bottom row cleared", a.board.Cells)
	}
	if a.board.Cells[0] == -1 || a.board.Cells[1] == -1 {
		t.Fatalf("board.Cells after one Undo = %v, want top row still placed", a.board.Cells)
	}

	a.Undo() // undoes the first placement too
	for i, l := range a.board.Cells {
		if l != -1 {
			t.Fatalf("board.Cells[%d] = %d after two Undos, want -1", i, l)
		}
	}
}

func TestPatches_Reset(t *testing.T) {
	a := patchesNewTinyAdapter()
	patchesPlaceRect(t, a, 0, pz.Rect{R0: 0, C0: 0, W: 2, H: 1})

	patchesMoveCursorTo(a, engine.Cell{Row: 1, Col: 0})
	a.HandleKey(patchesKeySpace()) // leave an anchor active too

	a.Reset()

	if a.hasAnchor {
		t.Fatalf("Reset left an anchor active")
	}
	if len(a.history) != 0 {
		t.Fatalf("Reset left %d history entries, want 0", len(a.history))
	}
	for i, l := range a.board.Cells {
		if l != -1 {
			t.Fatalf("board.Cells[%d] = %d after Reset, want -1", i, l)
		}
	}
	if a.Solved() {
		t.Fatalf("Reset board reports Solved() = true")
	}
	// Undo after Reset must be a safe no-op (history was cleared).
	a.Undo()
}

func TestPatches_Hint_PlacesOneCorrectRectangleAndProgresses(t *testing.T) {
	a := patchesNewTinyAdapter()

	a.Hint()
	if a.board.Cells[0] == -1 || a.board.Cells[1] == -1 || a.board.Cells[0] != a.board.Cells[1] {
		t.Fatalf("after first Hint, board.Cells = %v, want top row covered by one label", a.board.Cells)
	}
	if a.board.Cells[2] != -1 || a.board.Cells[3] != -1 {
		t.Fatalf("after first Hint, bottom row = %v, want still uncovered (only one rect revealed)", a.board.Cells[2:4])
	}
	if a.Solved() {
		t.Fatalf("Solved() = true after only one hint on a two-rectangle puzzle")
	}

	a.Hint()
	if !a.Solved() {
		t.Fatalf("Solved() = false after hinting every rectangle")
	}
	if v := a.Violations(); len(v) != 0 {
		t.Fatalf("Violations() = %+v after hinting the whole puzzle, want empty", v)
	}

	// A further Hint on an already-solved board is a no-op.
	before := append([]int(nil), a.board.Cells...)
	a.Hint()
	for i, l := range a.board.Cells {
		if l != before[i] {
			t.Fatalf("Hint on a solved board changed cell %d: %d -> %d", i, before[i], l)
		}
	}
}

func TestPatches_Hint_OverwritesAWrongGuess(t *testing.T) {
	a := patchesNewTinyAdapter()

	// Wrongly place a 1x1 at the first clue instead of the correct 2x1.
	patchesMoveCursorTo(a, engine.Cell{Row: 0, Col: 0})
	a.HandleKey(patchesKeySpace()) // anchor, no stretch
	if changed := a.HandleKey(patchesKeySpace()); !changed {
		t.Fatalf("committing the (wrong) 1x1 rectangle reported changed=false")
	}
	if v := a.Violations(); len(v) == 0 {
		t.Fatalf("expected the undersized rectangle to violate area, got none")
	}

	a.Hint()
	if a.board.Cells[0] == -1 || a.board.Cells[1] == -1 || a.board.Cells[0] != a.board.Cells[1] {
		t.Fatalf("after Hint over a wrong guess, board.Cells = %v, want top row correctly covered", a.board.Cells)
	}
}

func TestPatches_Hint_NoopWithoutSolution(t *testing.T) {
	a := patchesNewTinyAdapter()
	a.solution = nil
	a.Hint() // must not panic
	for i, l := range a.board.Cells {
		if l != -1 {
			t.Fatalf("board.Cells[%d] = %d after Hint with no solution, want -1", i, l)
		}
	}
}

// ---------------------------------------------------------------------------
// GivenCellImmutability and Violations passthrough.
// ---------------------------------------------------------------------------

func TestPatches_GivenClueDataNeverMutatedByPlay(t *testing.T) {
	a := patchesNewTinyAdapter()
	before := pz.Encode(a.puzzle)

	patchesPlaceRect(t, a, 0, pz.Rect{R0: 0, C0: 0, W: 2, H: 1})
	a.Undo()
	patchesPlaceRect(t, a, 0, pz.Rect{R0: 0, C0: 0, W: 2, H: 1})
	a.Hint()
	a.Reset()

	if after := pz.Encode(a.puzzle); !bytes.Equal(before, after) {
		t.Fatalf("puzzle clues mutated: before=%s after=%s", before, after)
	}
}

func TestPatches_Violations_DelegatesToEngineValidator(t *testing.T) {
	// A single clue on a taller grid, so a rectangle can cover exactly that
	// one clue while still landing on the wrong shape (as opposed to the
	// tiny 2-clue puzzle, where stretching toward the other clue would
	// instead trip the one-clue violation first).
	p := &pz.Puzzle{
		R: 2,
		C: 3,
		Clues: map[int]pz.Clue{
			0: {Number: 2, Shape: pz.Wide}, // true solution: 2x1 (width>height)
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}
	a := &patchesAdapter{
		puzzle:    p,
		board:     pz.NewBoard(p),
		validator: pz.NewValidator(p),
		rows:      p.R,
		cols:      p.C,
		cellWidth: patchesCellWidth(p),
	}

	// Wrong shape: a 1x2 (tall) rect over a Wide clue -> shape violation.
	patchesMoveCursorTo(a, engine.Cell{Row: 0, Col: 0})
	a.HandleKey(patchesKeySpace())
	a.HandleKey(patchesKeyRune('s')) // stretch down: now 1 wide x 2 tall
	a.HandleKey(patchesKeySpace())   // commit

	violations := a.Violations()
	foundShape := false
	for _, v := range violations {
		if v.Rule == pz.RuleShape {
			foundShape = true
		}
	}
	if !foundShape {
		t.Fatalf("Violations() = %+v, want a %q violation for the tall rect over a Wide clue", violations, pz.RuleShape)
	}
}

// ---------------------------------------------------------------------------
// GridGeometry vs. rendered View consistency.
// ---------------------------------------------------------------------------

func TestPatches_GridGeometryMatchesRenderedView(t *testing.T) {
	gen, _, _ := patchesGenerate(t, 7)
	a, ok := patchesNew(gen).(*patchesAdapter)
	if !ok {
		t.Fatalf("patchesNew returned unexpected type")
	}

	for _, theme := range tui.Themes() {
		geo := a.GridGeometry()
		view := a.View(theme)
		lines := lipglossSplitLines(view)

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

		// A point square in the middle of cell (0,0)'s body must resolve back
		// to (0,0), proving the geometry the shell uses for hit-testing
		// actually lines up with what got rendered.
		cell, hit := tui.CellFromPoint(geo, geo.CellWidth/2, 0)
		if !hit || cell != (engine.Cell{Row: 0, Col: 0}) {
			t.Fatalf("CellFromPoint at cell (0,0)'s body = %+v, hit=%v, want {0,0},true", cell, hit)
		}
	}
}

// lipglossSplitLines splits a rendered view into lines without pulling in
// strings just for this one call site's worth of splitting logic elsewhere
// in the file.
func lipglossSplitLines(s string) []string {
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

// ---------------------------------------------------------------------------
// Movement / anchoring edge cases.
// ---------------------------------------------------------------------------

func TestPatches_PrimaryAction_IgnoredOffClue(t *testing.T) {
	a := patchesNewTinyAdapter()
	// (0,1) and (1,1) are not clue cells in the tiny puzzle.
	patchesMoveCursorTo(a, engine.Cell{Row: 0, Col: 1})
	if changed := a.HandleKey(patchesKeySpace()); changed {
		t.Fatalf("Space off a clue reported changed=true, want false")
	}
	if a.hasAnchor {
		t.Fatalf("Space off a clue started an anchor")
	}
}

func TestPatches_Commit_RejectsOverlap(t *testing.T) {
	a := patchesNewTinyAdapter()
	patchesPlaceRect(t, a, 0, pz.Rect{R0: 0, C0: 0, W: 2, H: 1})

	// Re-anchoring at (0,0) is impossible now (already covered) — Space
	// there should be a no-op, not a crash, and must not start an anchor.
	patchesMoveCursorTo(a, engine.Cell{Row: 0, Col: 0})
	if changed := a.HandleKey(patchesKeySpace()); changed {
		t.Fatalf("Space on an already-covered clue reported changed=true, want false")
	}
	if a.hasAnchor {
		t.Fatalf("Space on an already-covered clue started an anchor")
	}
}

func TestPatches_StretchClampsToGridBounds(t *testing.T) {
	a := patchesNewTinyAdapter()
	patchesMoveCursorTo(a, engine.Cell{Row: 0, Col: 0})
	a.HandleKey(patchesKeySpace())
	for i := 0; i < 5; i++ {
		a.HandleKey(patchesKeyRune('a')) // already at col 0; must clamp, not go negative
		a.HandleKey(patchesKeyRune('w')) // already at row 0; must clamp
	}
	if a.box.r0 != 0 || a.box.c0 != 0 {
		t.Fatalf("box = %+v after clamped stretches, want r0=c0=0", a.box)
	}
}
