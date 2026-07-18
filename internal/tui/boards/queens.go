// Package boards holds the per-game BoardAdapter implementations. This file
// is the Queens adapter; see docs/plan/games/queens.md ("TUI interaction")
// and docs/plan/docs/03-tui-design.md (board-adapter pattern, the
// two-handed scheme table, and the accessibility note calling for region
// borders + letter tags in Queens specifically) for the design this
// implements.
//
// Every package-level identifier in this file is prefixed with "queens"
// because sibling adapters for the other four games live in this same
// package and are written concurrently by other agents.
package boards

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	queensgame "github.com/Jensen95/tui-games/internal/games/queens"
	"github.com/Jensen95/tui-games/internal/tui"
)

func init() {
	tui.Register(queensgame.GameID, queensNew)
}

// Rendering geometry constants for the Queens grid. Cells are 3 columns wide
// (room for " Q ", " x ", or a region letter tag) and 1 row tall; a
// 1-column/1-row gutter between cells carries the region-boundary border
// glyphs that are Queens' required non-color accessibility channel
// (03-tui-design.md: "region borders and letter tags for Queens per the
// accessibility note").
const (
	queensCellWidth  = 3
	queensCellHeight = 1
	queensColGutter  = 1
	queensRowGutter  = 1
)

// queensCellState is the TUI-only per-cell state. The engine's Board only
// ever models Empty/Queen (queensgame.Cell) — Marked ("X" note) is purely a
// player scratch mark with no representation in the engine, per
// docs/plan/games/queens.md's data model. queensAdapter tracks this richer
// three-state model itself and folds Marked down to Empty whenever it builds
// a queensgame.Board to hand to the validator.
type queensCellState uint8

const (
	queensCellEmpty queensCellState = iota
	queensCellMarked
	queensCellQueen
)

// next implements the legacy fallback's full cycle: empty -> X -> queen ->
// empty (docs/plan/games/queens.md, "Space cycles empty->X->queen->empty").
func (s queensCellState) next() queensCellState {
	return (s + 1) % 3
}

// queensSnapshot is one entry of the undo stack: the adapter's full mutable
// state immediately before a mutating action, so Undo can restore it byte
// for byte.
type queensSnapshot struct {
	state  []queensCellState
	cursor engine.Cell
}

// queensAdapter implements tui.BoardAdapter for Queens. It never
// re-implements the game's rules: Violations/Solved always delegate to
// queensgame.Validator, and Hint always walks the recorded solution.
type queensAdapter struct {
	puzzle      queensgame.Puzzle
	solution    queensgame.Solution
	hasSolution bool
	validator   *queensgame.Validator

	// state is the current per-cell TUI state, row-major, length N*N.
	state []queensCellState
	// givenSet holds the row-major indices of the puzzle's pre-placed
	// queens; those cells are immutable to every mutating action.
	givenSet map[int]bool

	cursor engine.Cell

	// dragging is the mouse-only drag-paint state machine: while true
	// (between a valid left-button press and its matching release), motion
	// over new cells paints an X mark across them, mirroring LinkedIn's
	// "drag to mark" affordance (03-tui-design.md, "Navigation — mouse").
	dragging bool
	// dragVisited tracks which cell indices this drag has already painted,
	// so re-entering a cell (or the press cell itself) during the same drag
	// doesn't repaint / double-toggle it.
	dragVisited map[int]bool

	history []queensSnapshot
}

// queensNew is the tui.AdapterFactory for Queens, registered against
// queensgame.GameID.
func queensNew(gen engine.Generated) tui.BoardAdapter {
	puzzle, ok := gen.Puzzle.(queensgame.Puzzle)
	if !ok {
		panic(fmt.Sprintf("boards: queens adapter got unexpected puzzle type %T", gen.Puzzle))
	}
	solution, hasSolution := gen.Solution.(queensgame.Solution) // absent solution just disables Hint

	a := &queensAdapter{
		puzzle:      puzzle,
		solution:    solution,
		hasSolution: hasSolution,
		validator:   queensgame.NewValidator(),
	}
	a.resetState()
	return a
}

// resetState rebuilds the per-cell state array to the puzzle's ungenerated-
// move state: every given cell holds its pre-placed queen, everything else
// is empty, cursor back at (0,0), and no undo history.
func (a *queensAdapter) resetState() {
	n := a.puzzle.N
	a.state = make([]queensCellState, n*n)
	a.givenSet = make(map[int]bool, len(a.puzzle.Givens))
	for _, g := range a.puzzle.Givens {
		a.givenSet[g] = true
		a.state[g] = queensCellQueen
	}
	a.cursor = engine.Cell{}
	a.dragging = false
	a.dragVisited = nil
	a.history = nil
}

func (a *queensAdapter) pushHistory() {
	a.history = append(a.history, queensSnapshot{
		state:  append([]queensCellState(nil), a.state...),
		cursor: a.cursor,
	})
}

// ---------------------------------------------------------------------------
// Keyboard: wasd/arrows/hjkl move the cursor. Space is the two-handed
// scheme's primary action AND the legacy fallback in one: on the first press
// over an empty cell it places an X mark ("Primary (Space): place X mark"),
// and because it always advances the same empty->X->queen->empty cycle it is
// also the full legacy fallback ("Space cycles empty->X->queen->empty") —
// there is no way to tell, from a single keypress, whether a terminal could
// have disambiguated Shift+Space, so the cycle must carry the whole legacy
// feature set on its own. Shift+Space (detectable only on enhanced
// terminals, see tui.IsShifted) is the secondary action: place a queen
// directly (again clears). 'x' is the direct-mark fallback key, working on
// every terminal. See docs/plan/games/queens.md's "TUI interaction" and the
// two-handed scheme table in 03-tui-design.md.
// ---------------------------------------------------------------------------

// queensDirFromKey maps a key press to a unit movement vector, covering
// arrows, wasd, and hjkl.
func queensDirFromKey(k tea.KeyPressMsg) (engine.Cell, bool) {
	switch k.Code {
	case tea.KeyUp, 'w', 'k':
		return engine.Cell{Row: -1}, true
	case tea.KeyDown, 's', 'j':
		return engine.Cell{Row: 1}, true
	case tea.KeyLeft, 'a', 'h':
		return engine.Cell{Col: -1}, true
	case tea.KeyRight, 'd', 'l':
		return engine.Cell{Col: 1}, true
	default:
		return engine.Cell{}, false
	}
}

// HandleKey implements tui.BoardAdapter.
func (a *queensAdapter) HandleKey(k tea.KeyPressMsg) bool {
	if dir, ok := queensDirFromKey(k); ok {
		return a.move(dir)
	}
	if tui.IsSpace(k) {
		if tui.IsShifted(k) {
			return a.toggleQueen(a.cursor)
		}
		return a.cycleCell(a.cursor)
	}
	if k.Code == 'x' {
		return a.markDirect(a.cursor)
	}
	return false
}

// move steps the cursor one cell in dir, clamped to stay in bounds. It never
// mutates board state, so it isn't recorded on the undo stack (mirroring
// zip's plain-move behavior).
func (a *queensAdapter) move(dir engine.Cell) bool {
	target := engine.Cell{Row: a.cursor.Row + dir.Row, Col: a.cursor.Col + dir.Col}
	if !engine.InBounds(target, a.puzzle.N, a.puzzle.N) {
		return false
	}
	a.cursor = target
	return true
}

// cycleCell implements both the primary action's first step (empty -> X)
// and the legacy fallback's full cycle (empty -> X -> queen -> empty). It is
// a no-op on a given cell.
func (a *queensAdapter) cycleCell(cell engine.Cell) bool {
	idx := engine.Index(cell, a.puzzle.N)
	if a.givenSet[idx] {
		return false
	}
	a.pushHistory()
	a.state[idx] = a.state[idx].next()
	return true
}

// toggleQueen implements the secondary action (Shift+Space): place a queen
// directly, or clear it if one is already there. A cell that only holds an X
// mark is overwritten with a queen (not cleared first) — the same
// "again clears" pattern Tango/Zip use for their secondary actions applies
// only to the queen state itself. No-op on a given cell.
func (a *queensAdapter) toggleQueen(cell engine.Cell) bool {
	idx := engine.Index(cell, a.puzzle.N)
	if a.givenSet[idx] {
		return false
	}
	a.pushHistory()
	if a.state[idx] == queensCellQueen {
		a.state[idx] = queensCellEmpty
	} else {
		a.state[idx] = queensCellQueen
	}
	return true
}

// markDirect implements the legacy fallback key 'x': place an X mark
// directly (not a toggle — pressing it again on an already-marked cell is a
// no-op, per docs/plan/docs/03-tui-design.md's "x places an X directly").
// No-op on a given cell.
func (a *queensAdapter) markDirect(cell engine.Cell) bool {
	idx := engine.Index(cell, a.puzzle.N)
	if a.givenSet[idx] || a.state[idx] == queensCellMarked {
		return false
	}
	a.pushHistory()
	a.state[idx] = queensCellMarked
	return true
}

// ---------------------------------------------------------------------------
// Mouse: click cycles a cell (mirrors the keyboard primary/legacy cycle);
// click-drag paints X marks across dragged cells (LinkedIn's "drag to
// mark"); right-click clears. This is Queens' defining mouse interaction
// (03-tui-design.md, "Navigation — mouse").
// ---------------------------------------------------------------------------

// HandleMouse implements tui.BoardAdapter.
func (a *queensAdapter) HandleMouse(ev tui.MouseEvent, cell tui.CellRef) bool {
	switch ev.Type {
	case tui.MouseEventPress:
		return a.mousePress(ev, cell)
	case tui.MouseEventMotion:
		return a.mouseMotion(cell)
	case tui.MouseEventRelease:
		a.dragging = false
		a.dragVisited = nil
		return false
	default:
		return false
	}
}

func (a *queensAdapter) mousePress(ev tui.MouseEvent, cell tui.CellRef) bool {
	if !cell.Valid {
		a.dragging = false
		a.dragVisited = nil
		return false
	}
	a.cursor = cell.Cell
	idx := engine.Index(cell.Cell, a.puzzle.N)

	if ev.Button == tea.MouseRight {
		a.dragging = false
		a.dragVisited = nil
		if a.givenSet[idx] || a.state[idx] == queensCellEmpty {
			return false
		}
		a.pushHistory()
		a.state[idx] = queensCellEmpty
		return true
	}

	// Left (or any other) button: the click itself cycles the cell, same as
	// keyboard Space; a subsequent drag over other cells paints X marks
	// instead (mirroring LinkedIn), tracked via dragVisited so the press
	// cell is never repainted by its own drag.
	a.dragging = true
	a.dragVisited = map[int]bool{idx: true}
	return a.cycleCell(cell.Cell)
}

func (a *queensAdapter) mouseMotion(cell tui.CellRef) bool {
	if !a.dragging || !cell.Valid {
		return false
	}
	idx := engine.Index(cell.Cell, a.puzzle.N)
	if a.dragVisited[idx] {
		return false
	}
	a.dragVisited[idx] = true
	a.cursor = cell.Cell
	if a.givenSet[idx] || a.state[idx] == queensCellMarked {
		return false
	}
	a.pushHistory()
	a.state[idx] = queensCellMarked
	return true
}

// ---------------------------------------------------------------------------
// Undo / Reset / Hint.
// ---------------------------------------------------------------------------

// Undo implements tui.BoardAdapter.
func (a *queensAdapter) Undo() {
	if len(a.history) == 0 {
		return
	}
	last := a.history[len(a.history)-1]
	a.history = a.history[:len(a.history)-1]
	a.state = last.state
	a.cursor = last.cursor
}

// Reset implements tui.BoardAdapter: back to the ungenerated-move state
// (givens re-seeded, everything else empty, cursor at (0,0), history
// cleared). The puzzle's own region coloring and givens are never mutated
// by play, so there is nothing else to restore.
func (a *queensAdapter) Reset() {
	a.resetState()
}

// Hint implements tui.BoardAdapter: finds the first row whose queen doesn't
// match the recorded solution (either missing or misplaced — this also
// self-corrects a wrong guess) and plants it there, clearing any wrong
// placement in that row first. No-op if there's no recorded solution or the
// board already agrees with the solution on every row.
func (a *queensAdapter) Hint() {
	if !a.hasSolution || a.solution.N != a.puzzle.N {
		return
	}
	n := a.puzzle.N
	for row := 0; row < n; row++ {
		wantIdx := row*n + a.solution.QueenAt[row]

		curIdx := -1
		for c := 0; c < n; c++ {
			i := row*n + c
			if a.state[i] == queensCellQueen {
				curIdx = i
				break
			}
		}
		if curIdx == wantIdx {
			continue
		}

		a.pushHistory()
		if curIdx != -1 && !a.givenSet[curIdx] {
			a.state[curIdx] = queensCellEmpty
		}
		if !a.givenSet[wantIdx] {
			a.state[wantIdx] = queensCellQueen
		}
		a.cursor = engine.CellAt(wantIdx, n)
		return
	}
}

// ---------------------------------------------------------------------------
// Validator delegation.
// ---------------------------------------------------------------------------

// board converts the adapter's richer TUI state to the engine's Board,
// folding Marked cells down to Empty — the engine never learns about
// scratch marks, per docs/plan/games/queens.md's data model.
func (a *queensAdapter) board() queensgame.Board {
	cells := make([]queensgame.Cell, len(a.state))
	for i, s := range a.state {
		if s == queensCellQueen {
			cells[i] = queensgame.Queen
		} else {
			cells[i] = queensgame.Empty
		}
	}
	return queensgame.Board{N: a.puzzle.N, Region: a.puzzle.Region, Cells: cells}
}

// Violations implements tui.BoardAdapter by delegating to the engine
// validator — the adapter never referees the rules itself.
func (a *queensAdapter) Violations() []engine.Violation {
	return a.validator.Violations(a.board())
}

// Solved implements tui.BoardAdapter by delegating to the engine validator.
func (a *queensAdapter) Solved() bool {
	return a.validator.Solved(a.board())
}

// GridGeometry implements tui.BoardAdapter. Origin is (0,0): the grid is the
// first thing View renders, so its top-left cell sits at the top-left of the
// string this adapter returns.
func (a *queensAdapter) GridGeometry() tui.Geometry {
	return tui.Geometry{
		OriginX:    0,
		OriginY:    0,
		CellWidth:  queensCellWidth,
		CellHeight: queensCellHeight,
		Rows:       a.puzzle.N,
		Cols:       a.puzzle.N,
		ColGutter:  queensColGutter,
		RowGutter:  queensRowGutter,
	}
}

// ---------------------------------------------------------------------------
// Rendering.
// ---------------------------------------------------------------------------

// violationCells returns the set of cell indices any current violation
// names, so the grid renderer can style them in the error style.
func (a *queensAdapter) violationCells() map[int]bool {
	set := make(map[int]bool)
	for _, v := range a.Violations() {
		for _, c := range v.Cells {
			set[engine.Index(c, a.puzzle.N)] = true
		}
	}
	return set
}

// View implements tui.BoardAdapter.
func (a *queensAdapter) View(theme tui.Theme) string {
	violCells := a.violationCells()

	n := a.puzzle.N
	lines := make([]string, 0, 2*n-1)
	for r := 0; r < n; r++ {
		lines = append(lines, a.renderCellRow(theme, r, violCells))
		if r < n-1 {
			lines = append(lines, a.renderGutterRow(theme, r))
		}
	}
	grid := strings.Join(lines, "\n")

	placed := 0
	for _, s := range a.state {
		if s == queensCellQueen {
			placed++
		}
	}
	dimStyle := lipgloss.NewStyle().Foreground(theme.Dim)
	status := dimStyle.Render(fmt.Sprintf("queens: %d/%d", placed, n))

	help := "Space: cycle empty/X/queen   x: mark X   u: undo   Ctrl+R: reset   H: hint"
	if tui.EnhancedKeyboardActive() {
		help = "Space: mark X (again clears)   Shift+Space: place queen (again clears)   x: mark X   u: undo   Ctrl+R: reset   H: hint"
	}

	// Deliberately not lipgloss.JoinVertical: it pads every line to the
	// block's widest line, which would corrupt the grid's per-cell width
	// that GridGeometry promises to callers doing mouse hit-testing (see
	// TestQueens_GridGeometryMatchesRenderedView).
	return grid + "\n\n" + status + "\n" + dimStyle.Render(help)
}

// renderCellRow renders one grid row of cells, joined with styled
// col-gutter region-border glyphs between them.
func (a *queensAdapter) renderCellRow(theme tui.Theme, r int, violCells map[int]bool) string {
	n := a.puzzle.N
	var b strings.Builder
	for c := 0; c < n; c++ {
		b.WriteString(a.renderCell(theme, r, c, violCells))
		if c < n-1 {
			b.WriteString(a.renderColGutter(theme, r, c))
		}
	}
	return b.String()
}

// renderCell renders one cell's 3-character body: the queen/mark glyph when
// occupied, or (empty cells only) the region's colorblind-safe letter tag —
// the "letter tags" half of 03-tui-design.md's Queens accessibility note.
// Region fill color is the background regardless of occupancy; the border
// glyphs in the gutters (see renderColGutter/renderGutterRow) stay the
// non-color channel even over an occupied cell.
func (a *queensAdapter) renderCell(theme tui.Theme, r, c int, violCells map[int]bool) string {
	n := a.puzzle.N
	idx := r*n + c
	region := a.puzzle.Region[idx]
	isCursor := a.cursor == (engine.Cell{Row: r, Col: c})
	isGiven := a.givenSet[idx]
	isViolation := violCells[idx]
	state := a.state[idx]

	var text string
	switch state {
	case queensCellQueen:
		text = " Q "
	case queensCellMarked:
		text = " x "
	default:
		text = " " + tui.RegionLabel(region) + " "
	}

	style := lipgloss.NewStyle().Background(theme.RegionColor(region))
	if isCursor {
		style = style.Background(theme.Accent)
	}
	bold := isCursor || isGiven || isViolation || state == queensCellQueen
	switch {
	case isViolation:
		style = style.Foreground(theme.Error)
	case isCursor:
		style = style.Foreground(theme.OnAccent)
	case state == queensCellQueen && isGiven:
		style = style.Foreground(theme.Accent)
	case state == queensCellQueen:
		style = style.Foreground(theme.Piece)
	case state == queensCellMarked:
		style = style.Foreground(theme.Elim)
	default:
		style = style.Foreground(theme.Dim)
	}
	if bold {
		style = style.Bold(true)
	}
	if isGiven {
		style = style.Underline(true)
	}
	return style.Render(text)
}

// renderColGutter renders the 1-character gutter between cell (r,c) and
// (r,c+1): a border glyph when the two cells belong to different regions
// (Queens' required non-color channel), a plain space otherwise.
func (a *queensAdapter) renderColGutter(theme tui.Theme, r, c int) string {
	n := a.puzzle.N
	idxA := r*n + c
	idxB := r*n + c + 1
	if a.puzzle.Region[idxA] == a.puzzle.Region[idxB] {
		return " "
	}
	return lipgloss.NewStyle().Foreground(theme.Border).Bold(true).Render("┃")
}

// renderGutterRow renders the 1-row gutter between grid row r and r+1: each
// 3-wide column segment is a border glyph when the two cells above/below
// belong to different regions, a blank segment otherwise, with a 1-char gap
// between segments to match renderCellRow's layout.
func (a *queensAdapter) renderGutterRow(theme tui.Theme, r int) string {
	n := a.puzzle.N
	var b strings.Builder
	for c := 0; c < n; c++ {
		idxA := r*n + c
		idxB := (r+1)*n + c
		if a.puzzle.Region[idxA] == a.puzzle.Region[idxB] {
			b.WriteString("   ")
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(theme.Border).Bold(true).Render("━━━"))
		}
		if c < n-1 {
			b.WriteString(" ")
		}
	}
	return b.String()
}

// Compile-time check that queensAdapter satisfies tui.BoardAdapter.
var _ tui.BoardAdapter = (*queensAdapter)(nil)
