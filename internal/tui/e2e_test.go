// Package tui_test holds the shell's end-to-end tests (04-testing-strategy.md
// layer 6): a full happy-path solve per game and one invalid-move path
// asserting live violation feedback, both driven entirely through the root
// tui.Model.
//
// teatest was evaluated first, per the plan: charm.land/x/exp/teatest/v2
// does not exist (its module declares itself as
// github.com/charmbracelet/x/exp/teatest/v2 under a mismatched path, so
// `go get`-ing the charm.land import path fails outright). The real package,
// github.com/charmbracelet/x/exp/teatest/v2, does resolve — its go.mod
// originally pinned an old charm.land/bubbletea/v2 v2.0.0-rc.1, but Go's MVS
// cleanly upgrades that requirement to this repo's pinned v2.0.8 with no
// downgrades elsewhere, and its TestModel API only depends on the stable
// tea.Model interface (Init/Update/View), which v2.0.8 still satisfies — it
// compiles and a smoke run passes. In practice, though, it renders through a
// real tea.Program's diffing renderer: tm.Output() delivers incremental,
// partial-repaint chunks (cursor-addressed ANSI, not full frames), so
// asserting "screen X is showing" via substring matches on whatever chunk
// WaitFor happens to read is flaky by construction — confirmed by an actual
// flake on a repeated local run of a first draft of this suite. That directly
// conflicts with "keep it deterministic," the harder requirement, so
// teatest is not adopted: go.mod/go.sum are untouched, and every test below
// drives tui.Model directly (NewModel, Update, View), executing
// generateCmd's returned Cmd inline for a synchronous puzzleReadyMsg — no
// goroutines, no incremental-render races.
//
// This file is deliberately an external test package (tui_test, not tui): it
// exercises the shell exactly as a real terminal driver would — NewModel,
// Update, View — with no access to unexported fields (screen, gameView,
// menuModel, puzzleReadyMsg...). The one wrinkle that forces a small
// reflect+unsafe helper (puzzleReadyGen, below) is reading the
// engine.Generated payload back out of generateCmd's unexported
// puzzleReadyMsg result: the shell has no other way to hand a black-box
// caller "the puzzle + recorded solution actually generated," and scripting
// a real per-game key sequence (mirroring boards/*_test.go at the adapter
// level, but through the root model) needs exactly that. Every other field
// access goes through ordinary exported API.
package tui_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"unsafe"

	tea "charm.land/bubbletea/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	_ "github.com/Jensen95/tui-games/internal/games/all" // registers every game's engine.Entry (init side effects)
	minisudokugame "github.com/Jensen95/tui-games/internal/games/minisudoku"
	pz "github.com/Jensen95/tui-games/internal/games/patches"
	queensgame "github.com/Jensen95/tui-games/internal/games/queens"
	tangogame "github.com/Jensen95/tui-games/internal/games/tango"
	zipgame "github.com/Jensen95/tui-games/internal/games/zip"
	"github.com/Jensen95/tui-games/internal/tui"
	_ "github.com/Jensen95/tui-games/internal/tui/boards" // registers the five board adapters (init side effects)
)

// ---------------------------------------------------------------------------
// Driving the root Model directly (no real terminal, no real async): a thin
// wrapper that reassigns the returned tea.Model back to a tui.Model, the
// same pattern internal/tui/app_test.go uses in-package.
// ---------------------------------------------------------------------------

type driver struct {
	t *testing.T
	m tui.Model
}

func newDriver(t *testing.T) *driver {
	t.Helper()
	return &driver{t: t, m: tui.NewModel(tui.Dark())}
}

// send feeds one message through Update and keeps the resulting Model,
// returning whatever Cmd (if any) came back — e.g. generateCmd's Cmd, which
// the caller executes directly (per 04-testing-strategy.md: "no real
// async") to obtain the puzzleReadyMsg.
func (d *driver) send(msg tea.Msg) tea.Cmd {
	d.t.Helper()
	next, cmd := d.m.Update(msg)
	nm, ok := next.(tui.Model)
	if !ok {
		d.t.Fatalf("Update returned %T, want tui.Model", next)
	}
	d.m = nm
	return cmd
}

func (d *driver) key(k tea.KeyPressMsg) { d.send(k) }

func (d *driver) view() string { return d.m.View().Content }

func krune(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func keySpace() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "} }

// keyShiftSpace encodes the enhanced-terminal Shift+Space shape (real ModShift
// bit) — tui.IsShifted reads that directly, so it's unambiguous regardless of
// whether this test binary's keyboard-enhancement flag is on (it never is:
// nothing in this file sends a tea.KeyboardEnhancementsMsg).
func keyShiftSpace() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModShift} }

// moveTo drives cur from wherever it is to target via plain wasd key
// presses — the same input path a real player's movement would take, and
// the same helper shape as every boards/*_test.go's moveCursorTo.
func moveTo(d *driver, cur *engine.Cell, target engine.Cell) {
	for cur.Row < target.Row {
		d.key(krune('s'))
		cur.Row++
	}
	for cur.Row > target.Row {
		d.key(krune('w'))
		cur.Row--
	}
	for cur.Col < target.Col {
		d.key(krune('d'))
		cur.Col++
	}
	for cur.Col > target.Col {
		d.key(krune('a'))
		cur.Col--
	}
}

// ---------------------------------------------------------------------------
// puzzleReadyGen: read the engine.Generated payload out of an opaque
// puzzleReadyMsg. See the package doc comment for why this exists.
// ---------------------------------------------------------------------------

func puzzleReadyGen(t *testing.T, msg tea.Msg) engine.Generated {
	t.Helper()
	rv := reflect.ValueOf(msg)
	if rv.Kind() != reflect.Struct {
		t.Fatalf("generate Cmd produced %T, want a puzzleReadyMsg struct", msg)
	}

	// rv itself (fresh from an interface value) is fully usable, but a field
	// obtained from it by name is marked read-only because the field is
	// unexported. Copying the whole struct into an addressable temporary and
	// re-wrapping the field's address via unsafe is the standard, narrowly-
	// scoped way to read (never set) an unexported field's value from
	// outside its package.
	addr := reflect.New(rv.Type()).Elem()
	addr.Set(rv)
	unexported := func(name string) reflect.Value {
		f := addr.FieldByName(name)
		return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	}

	if errIface := unexported("err").Interface(); errIface != nil {
		if err, ok := errIface.(error); ok && err != nil {
			t.Fatalf("puzzle generation failed: %v", err)
		}
	}

	gen, ok := unexported("gen").Interface().(engine.Generated)
	if !ok {
		t.Fatalf("puzzleReadyMsg.gen has unexpected type %T", unexported("gen").Interface())
	}
	return gen
}

// ---------------------------------------------------------------------------
// Menu navigation + generation, shared by every direct-model test.
// ---------------------------------------------------------------------------

// selectGame drives the Menu -> Generating -> Playing transition for game id:
// cursors down to its entry (engine.All() is sorted by ID, so the cursor
// count is just its rank), presses the primary action, executes the
// returned generateCmd Cmd directly (no real async), and feeds the resulting
// puzzleReadyMsg back through Update. It asserts each screen actually
// changed via the rendered view (this package has no access to m.screen).
func selectGame(t *testing.T, d *driver, id engine.GameID) (engine.Entry, engine.Generated) {
	t.Helper()

	entries := engine.All()
	idx := -1
	var entry engine.Entry
	for i, e := range entries {
		if e.ID == id {
			idx, entry = i, e
			break
		}
	}
	if idx < 0 {
		t.Fatalf("game %q not found in engine.All() — is its games package imported?", id)
	}

	for i := 0; i < idx; i++ {
		d.key(tea.KeyPressMsg{Code: tea.KeyDown})
	}

	cmd := d.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("selecting %q produced no generate Cmd", id)
	}
	if !strings.Contains(d.view(), "generating puzzle") {
		t.Fatalf("expected the Generating screen after selecting %q, got:\n%s", id, d.view())
	}

	msg := cmd() // execute the Cmd directly: no real async, per 04-testing-strategy.md
	gen := puzzleReadyGen(t, msg)
	if feedCmd := d.send(msg); feedCmd != nil {
		t.Fatalf("feeding the puzzleReadyMsg back produced an unexpected Cmd")
	}
	if !strings.Contains(d.view(), entry.Name) {
		t.Fatalf("expected the Playing screen (title %q) after puzzleReadyMsg, got:\n%s", entry.Name, d.view())
	}
	if strings.Contains(d.view(), "Solved in") {
		t.Fatalf("a freshly-generated puzzle must not already report solved:\n%s", d.view())
	}
	return entry, gen
}

// assertWinFlow drives the shared post-solve checks every game test shares:
// the win banner is showing, 'n' regenerates a fresh (unsolved) puzzle
// still on the Playing screen, and Esc returns to the Menu.
func assertWinFlow(t *testing.T, d *driver, entry engine.Entry) {
	t.Helper()

	view := d.view()
	if !strings.Contains(view, "Solved in") || !strings.Contains(view, "new puzzle") {
		t.Fatalf("expected the WinSummary solved banner after the scripted solve, got:\n%s", view)
	}

	// 'n': new puzzle, same game/difficulty.
	cmd := d.send(krune('n'))
	if cmd == nil {
		t.Fatalf("'n' from WinSummary produced no Cmd")
	}
	msg := cmd()
	puzzleReadyGen(t, msg) // fails loudly if generation errored
	if feedCmd := d.send(msg); feedCmd != nil {
		t.Fatalf("feeding the new puzzleReadyMsg back produced an unexpected Cmd")
	}
	freshView := d.view()
	if strings.Contains(freshView, "Solved in") {
		t.Fatalf("expected a freshly-regenerated puzzle to be unsolved, got:\n%s", freshView)
	}
	if !strings.Contains(freshView, entry.Name) {
		t.Fatalf("expected to still be on the Playing screen after 'n', got:\n%s", freshView)
	}

	// Esc: back to Menu.
	d.key(tea.KeyPressMsg{Code: tea.KeyEscape})
	menuView := d.view()
	if !strings.Contains(menuView, "lig — pick a game") {
		t.Fatalf("expected the Menu screen after Esc, got:\n%s", menuView)
	}
}

// ---------------------------------------------------------------------------
// Per-game scripted solves: mirror boards/*_test.go's adapter-level scripted
// solves, but every key press goes through the root Model.
// ---------------------------------------------------------------------------

func solveTango(t *testing.T, d *driver, gen engine.Generated) {
	t.Helper()
	puzzle, ok := gen.Puzzle.(tangogame.Puzzle)
	if !ok {
		t.Fatalf("tango: gen.Puzzle has unexpected type %T", gen.Puzzle)
	}
	sol, ok := gen.Solution.(tangogame.Board)
	if !ok {
		t.Fatalf("tango: gen.Solution has unexpected type %T", gen.Solution)
	}

	cur := engine.Cell{}
	n := puzzle.N
	for idx := 0; idx < n*n; idx++ {
		if _, given := puzzle.Givens[idx]; given {
			continue
		}
		moveTo(d, &cur, engine.CellAt(idx, n))
		switch sol.Cells[idx] {
		case tangogame.Sun:
			d.key(keySpace()) // bare Space cycles empty->sun first, per the legacy fallback
		case tangogame.Moon:
			d.key(keyShiftSpace())
		default:
			t.Fatalf("tango: solution cell %d has non-sun/moon symbol %v", idx, sol.Cells[idx])
		}
	}
}

func solveMiniSudoku(t *testing.T, d *driver, gen engine.Generated) {
	t.Helper()
	puzzle, ok := gen.Puzzle.(minisudokugame.Puzzle)
	if !ok {
		t.Fatalf("minisudoku: gen.Puzzle has unexpected type %T", gen.Puzzle)
	}
	sol, ok := gen.Solution.(minisudokugame.Solution)
	if !ok {
		t.Fatalf("minisudoku: gen.Solution has unexpected type %T", gen.Solution)
	}

	cur := engine.Cell{}
	n := puzzle.N
	for idx := 0; idx < n*n; idx++ {
		if _, given := puzzle.Givens[idx]; given {
			continue
		}
		moveTo(d, &cur, engine.CellAt(idx, n))
		d.key(krune(rune('0' + sol.Cells[idx])))
	}
}

func solveZip(t *testing.T, d *driver, gen engine.Generated) {
	t.Helper()
	puzzle, ok := gen.Puzzle.(zipgame.Puzzle)
	if !ok {
		t.Fatalf("zip: gen.Puzzle has unexpected type %T", gen.Puzzle)
	}
	sol, ok := gen.Solution.(zipgame.Solution)
	if !ok {
		t.Fatalf("zip: gen.Solution has unexpected type %T", gen.Solution)
	}

	d.key(keySpace()) // pen down at the start cell (the adapter's cursor already sits there)
	for i := 1; i < len(sol.Path); i++ {
		from := engine.CellAt(sol.Path[i-1], puzzle.C)
		to := engine.CellAt(sol.Path[i], puzzle.C)
		d.key(zipStepKey(t, from, to))
	}
}

func zipStepKey(t *testing.T, from, to engine.Cell) tea.KeyPressMsg {
	t.Helper()
	dr, dc := to.Row-from.Row, to.Col-from.Col
	switch {
	case dr == -1 && dc == 0:
		return krune('w')
	case dr == 1 && dc == 0:
		return krune('s')
	case dr == 0 && dc == -1:
		return krune('a')
	case dr == 0 && dc == 1:
		return krune('d')
	default:
		t.Fatalf("zip: solution step %v -> %v is not orthogonally adjacent", from, to)
		return tea.KeyPressMsg{}
	}
}

func solvePatches(t *testing.T, d *driver, gen engine.Generated) {
	t.Helper()
	puzzle, ok := gen.Puzzle.(*pz.Puzzle)
	if !ok {
		t.Fatalf("patches: gen.Puzzle has unexpected type %T", gen.Puzzle)
	}
	sol, ok := gen.Solution.(*pz.Solution)
	if !ok {
		t.Fatalf("patches: gen.Solution has unexpected type %T", gen.Solution)
	}

	type candidate struct {
		clueIdx int
		rect    pz.Rect
	}
	var cands []candidate
	for _, rect := range sol.Rects {
		idx, found := patchesClueForRect(puzzle, rect)
		if !found {
			t.Fatalf("patches: solution rect %+v contains no clue", rect)
		}
		cands = append(cands, candidate{clueIdx: idx, rect: rect})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].clueIdx < cands[j].clueIdx })

	cur := engine.Cell{}
	for _, cand := range cands {
		clueCell := engine.CellAt(cand.clueIdx, puzzle.C)
		moveTo(d, &cur, clueCell)
		d.key(keySpace()) // anchor a 1x1 rectangle at the clue

		up := clueCell.Row - cand.rect.R0
		down := (cand.rect.R0 + cand.rect.H - 1) - clueCell.Row
		left := clueCell.Col - cand.rect.C0
		right := (cand.rect.C0 + cand.rect.W - 1) - clueCell.Col
		for i := 0; i < up; i++ {
			d.key(krune('w'))
		}
		for i := 0; i < down; i++ {
			d.key(krune('s'))
		}
		for i := 0; i < left; i++ {
			d.key(krune('a'))
		}
		for i := 0; i < right; i++ {
			d.key(krune('d'))
		}

		d.key(keySpace()) // commit
		cur = clueCell    // commitActive resets the cursor to the anchor cell
	}
}

// patchesClueForRect returns the anchor-cell index of the one clue rect
// contains, mirroring the adapter's own patchesClueForRect (unexported
// there): every solution rectangle contains exactly one clue.
func patchesClueForRect(p *pz.Puzzle, rect pz.Rect) (int, bool) {
	for r := rect.R0; r < rect.R0+rect.H; r++ {
		for c := rect.C0; c < rect.C0+rect.W; c++ {
			idx := r*p.C + c
			if _, ok := p.Clues[idx]; ok {
				return idx, true
			}
		}
	}
	return 0, false
}

func solveQueens(t *testing.T, d *driver, gen engine.Generated) {
	t.Helper()
	puzzle, ok := gen.Puzzle.(queensgame.Puzzle)
	if !ok {
		t.Fatalf("queens: gen.Puzzle has unexpected type %T", gen.Puzzle)
	}
	sol, ok := gen.Solution.(queensgame.Solution)
	if !ok {
		t.Fatalf("queens: gen.Solution has unexpected type %T", gen.Solution)
	}

	given := make(map[int]bool, len(puzzle.Givens))
	for _, g := range puzzle.Givens {
		given[g] = true
	}

	cur := engine.Cell{}
	for row, col := range sol.QueenAt {
		idx := row*puzzle.N + col
		if given[idx] {
			continue
		}
		moveTo(d, &cur, engine.Cell{Row: row, Col: col})
		d.key(keyShiftSpace()) // secondary action: place a queen directly
	}
}

// ---------------------------------------------------------------------------
// 1. Happy-path solves, driven directly through the root Model.
// ---------------------------------------------------------------------------

func TestE2E_Tango_FullSolveAndWinFlow(t *testing.T) {
	d := newDriver(t)
	d.send(tea.WindowSizeMsg{Width: 100, Height: 40})

	entry, gen := selectGame(t, d, tangogame.GameID)
	solveTango(t, d, gen)
	assertWinFlow(t, d, entry)
}

func TestE2E_MiniSudoku_FullSolveAndWinFlow(t *testing.T) {
	d := newDriver(t)
	d.send(tea.WindowSizeMsg{Width: 100, Height: 40})

	entry, gen := selectGame(t, d, minisudokugame.GameID)
	solveMiniSudoku(t, d, gen)
	assertWinFlow(t, d, entry)
}

func TestE2E_Zip_FullSolveAndWinFlow(t *testing.T) {
	d := newDriver(t)
	d.send(tea.WindowSizeMsg{Width: 100, Height: 40})

	entry, gen := selectGame(t, d, zipgame.ID)
	solveZip(t, d, gen)
	assertWinFlow(t, d, entry)
}

func TestE2E_Patches_FullSolveAndWinFlow(t *testing.T) {
	d := newDriver(t)
	d.send(tea.WindowSizeMsg{Width: 100, Height: 40})

	entry, gen := selectGame(t, d, pz.GameID)
	solvePatches(t, d, gen)
	assertWinFlow(t, d, entry)
}

// ---------------------------------------------------------------------------
// 2. Queens: also driven directly through the root Model (see the package
// doc comment for why teatest's real-program approach was tried and
// rejected as too flaky for this suite's determinism requirement).
// ---------------------------------------------------------------------------

func TestE2E_Queens_FullSolveAndWinFlow(t *testing.T) {
	d := newDriver(t)
	d.send(tea.WindowSizeMsg{Width: 100, Height: 40})

	entry, gen := selectGame(t, d, queensgame.GameID)
	solveQueens(t, d, gen)
	assertWinFlow(t, d, entry)
}

// ---------------------------------------------------------------------------
// 3. Invalid-move path: a violating placement must show its message in the
// rendered view (Tango's three-in-a-row rule, chosen because it only needs
// three arbitrary non-given cells in a row — no full-board setup).
// ---------------------------------------------------------------------------

func TestE2E_Tango_InvalidMove_ShowsViolation(t *testing.T) {
	d := newDriver(t)
	d.send(tea.WindowSizeMsg{Width: 100, Height: 40})

	_, gen := selectGame(t, d, tangogame.GameID)
	puzzle, ok := gen.Puzzle.(tangogame.Puzzle)
	if !ok {
		t.Fatalf("tango: gen.Puzzle has unexpected type %T", gen.Puzzle)
	}

	row, col, found := tangoThreeConsecutiveNonGiven(puzzle)
	if !found {
		t.Fatalf("fixture puzzle has no row with three consecutive non-given cells to violate")
	}

	cur := engine.Cell{}
	for i := 0; i < 3; i++ {
		moveTo(d, &cur, engine.Cell{Row: row, Col: col + i})
		d.key(keySpace()) // bare Space cycles empty->sun on a fresh cell
	}

	view := d.view()
	if !strings.Contains(view, "three consecutive identical symbols") {
		t.Fatalf("expected the three-in-a-row violation message in the rendered view, got:\n%s", view)
	}
}

// tangoThreeConsecutiveNonGiven finds a row and starting column with three
// consecutive non-given cells, so a test can place the same symbol in all
// three without fighting the puzzle's own givens.
func tangoThreeConsecutiveNonGiven(p tangogame.Puzzle) (row, col int, ok bool) {
	n := p.N
	for r := 0; r < n; r++ {
		for c := 0; c+2 < n; c++ {
			base := r*n + c
			if _, given := p.Givens[base]; given {
				continue
			}
			if _, given := p.Givens[base+1]; given {
				continue
			}
			if _, given := p.Givens[base+2]; given {
				continue
			}
			return r, c, true
		}
	}
	return 0, 0, false
}
