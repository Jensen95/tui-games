// Package boards holds the per-game BoardAdapter implementations. This file
// is the Tango adapter; see docs/plan/games/tango.md ("TUI interaction") and
// docs/plan/docs/03-tui-design.md (board-adapter pattern, the two-handed
// scheme table) for the design this implements.
//
// Every package-level identifier in this file is prefixed with "tango"
// because sibling adapters for the other four games live in this same
// package and are written concurrently by other agents.
package boards

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	tangogame "github.com/Jensen95/tui-games/internal/games/tango"
	"github.com/Jensen95/tui-games/internal/tui"
)

func init() {
	tui.Register(tangogame.GameID, tangoNew)
}

// Rendering geometry constants for the Tango grid. Cells are 3 columns wide
// (a padded sun/moon/empty glyph) and 1 row tall; a 1-column/1-row gutter
// between cells carries the "="/"×" edge-constraint markers, per
// 03-tui-design.md's "gutter markers for Tango edges" guidance.
const (
	tangoCellWidth  = 3
	tangoCellHeight = 1
	tangoColGutter  = 1
	tangoRowGutter  = 1
)

// tangoSnapshot is one entry of the undo stack: the adapter's full mutable
// state immediately before a mutating action, so Undo can restore it byte for
// byte.
type tangoSnapshot struct {
	cells  []tangogame.Symbol
	cursor engine.Cell
}

// tangoAdapter implements tui.BoardAdapter for Tango. It never re-implements
// the game's rules: Violations/Solved always delegate to
// tangogame.Validator, and Hint always delegates to the recorded solution
// (the engine's generator is what produced it).
type tangoAdapter struct {
	puzzle   tangogame.Puzzle
	solution tangogame.Board // the complete solved grid; zero value disables Hint

	// cells is the current (partial or complete) player-editable board state,
	// row-major, len N*N. Givens are pre-seeded here and never mutated.
	cells  []tangogame.Symbol
	cursor engine.Cell

	history []tangoSnapshot
}

// tangoNew is the tui.AdapterFactory for Tango, registered against
// tangogame.GameID.
func tangoNew(gen engine.Generated) tui.BoardAdapter {
	puzzle, ok := gen.Puzzle.(tangogame.Puzzle)
	if !ok {
		panic(fmt.Sprintf("boards: tango adapter got unexpected puzzle type %T", gen.Puzzle))
	}
	solution, _ := gen.Solution.(tangogame.Board) // absent solution just disables Hint

	a := &tangoAdapter{puzzle: puzzle, solution: solution}
	a.cells = tangoInitialCells(puzzle)
	return a
}

// tangoInitialCells builds the givens-only starting board for p: every given
// index seeded with its locked symbol, everything else Empty.
func tangoInitialCells(p tangogame.Puzzle) []tangogame.Symbol {
	cells := make([]tangogame.Symbol, p.N*p.N)
	for idx, sym := range p.Givens {
		cells[idx] = sym
	}
	return cells
}

// isGiven reports whether cell idx is a locked given, immutable to play.
func (a *tangoAdapter) isGiven(idx int) bool {
	_, ok := a.puzzle.Givens[idx]
	return ok
}

func (a *tangoAdapter) pushHistory() {
	a.history = append(a.history, tangoSnapshot{
		cells:  append([]tangogame.Symbol(nil), a.cells...),
		cursor: a.cursor,
	})
}

// setCellIfAllowed is the single mutation path for every symbol-changing
// action: it refuses to touch a given cell, is a no-op if the value would be
// unchanged (so undo history never gains an empty entry), and otherwise
// records history before mutating.
func (a *tangoAdapter) setCellIfAllowed(idx int, next tangogame.Symbol) bool {
	if a.isGiven(idx) {
		return false
	}
	if a.cells[idx] == next {
		return false
	}
	a.pushHistory()
	a.cells[idx] = next
	return true
}

// ---------------------------------------------------------------------------
// Keyboard: wasd/arrows/hjkl move the cursor. Space is the primary action
// (place sun, again clears); Shift+Space is the secondary action (place
// moon, again clears) whenever the key press actually carries the Shift
// modifier (true regardless of terminal capability, per tui.IsShifted). On a
// legacy terminal — where Shift+Space collapses into plain Space with no way
// to recover intent — plain Space instead cycles empty->sun->moon->empty,
// and 'm' places a moon directly, per docs/plan/docs/03-tui-design.md's
// two-handed scheme table.
// ---------------------------------------------------------------------------

// tangoDirFromKey maps a key press to a unit movement vector, covering
// arrows, wasd, and hjkl.
func tangoDirFromKey(k tea.KeyPressMsg) (engine.Cell, bool) {
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
func (a *tangoAdapter) HandleKey(k tea.KeyPressMsg) bool {
	if dir, ok := tangoDirFromKey(k); ok {
		return a.move(dir)
	}

	idx := engine.Index(a.cursor, a.puzzle.N)

	if tui.IsSpace(k) {
		if tui.IsShifted(k) {
			return a.toggleMoon(idx)
		}
		if tui.EnhancedKeyboardActive() {
			return a.toggleSun(idx)
		}
		return a.cycleSymbol(idx)
	}
	if k.Code == 'm' {
		return a.setMoonDirect(idx)
	}
	return false
}

// move steps the cursor one cell in dir, clamped to the grid.
func (a *tangoAdapter) move(dir engine.Cell) bool {
	target := engine.Cell{Row: a.cursor.Row + dir.Row, Col: a.cursor.Col + dir.Col}
	if !engine.InBounds(target, a.puzzle.N, a.puzzle.N) {
		return false
	}
	if target == a.cursor {
		return false
	}
	a.cursor = target
	return true
}

// toggleSun implements the primary action: place a sun, or clear if the cell
// is already a sun.
func (a *tangoAdapter) toggleSun(idx int) bool {
	next := tangogame.Sun
	if a.cells[idx] == tangogame.Sun {
		next = tangogame.Empty
	}
	return a.setCellIfAllowed(idx, next)
}

// toggleMoon implements the secondary action: place a moon, or clear if the
// cell is already a moon.
func (a *tangoAdapter) toggleMoon(idx int) bool {
	next := tangogame.Moon
	if a.cells[idx] == tangogame.Moon {
		next = tangogame.Empty
	}
	return a.setCellIfAllowed(idx, next)
}

// cycleSymbol implements the legacy fallback for a bare Space press:
// empty -> sun -> moon -> empty.
func (a *tangoAdapter) cycleSymbol(idx int) bool {
	var next tangogame.Symbol
	switch a.cells[idx] {
	case tangogame.Empty:
		next = tangogame.Sun
	case tangogame.Sun:
		next = tangogame.Moon
	default:
		next = tangogame.Empty
	}
	return a.setCellIfAllowed(idx, next)
}

// setMoonDirect implements the legacy 'm' fallback: always places a moon
// directly (idempotent — pressing it again on a moon is a no-op, unlike the
// primary/secondary toggles).
func (a *tangoAdapter) setMoonDirect(idx int) bool {
	return a.setCellIfAllowed(idx, tangogame.Moon)
}

// clearCell always sets a cell to Empty (used by the mouse's right-click).
func (a *tangoAdapter) clearCell(idx int) bool {
	return a.setCellIfAllowed(idx, tangogame.Empty)
}

// ---------------------------------------------------------------------------
// Mouse: left-click cycles the cell (empty->sun->moon->empty); right-click
// clears it. Per docs/plan/games/tango.md's "TUI interaction" and
// 03-tui-design.md's "Navigation — mouse" section.
// ---------------------------------------------------------------------------

// HandleMouse implements tui.BoardAdapter.
func (a *tangoAdapter) HandleMouse(ev tui.MouseEvent, cell tui.CellRef) bool {
	if ev.Type != tui.MouseEventPress || !cell.Valid {
		return false
	}
	idx := engine.Index(cell.Cell, a.puzzle.N)
	a.cursor = cell.Cell

	switch ev.Button {
	case tea.MouseLeft:
		return a.cycleSymbol(idx)
	case tea.MouseRight:
		return a.clearCell(idx)
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Undo / Reset / Hint.
// ---------------------------------------------------------------------------

// Undo implements tui.BoardAdapter.
func (a *tangoAdapter) Undo() {
	if len(a.history) == 0 {
		return
	}
	last := a.history[len(a.history)-1]
	a.history = a.history[:len(a.history)-1]
	a.cells = last.cells
	a.cursor = last.cursor
}

// Reset implements tui.BoardAdapter: back to the ungenerated-move state
// (givens only, cursor at the origin, undo history cleared).
func (a *tangoAdapter) Reset() {
	a.cells = tangoInitialCells(a.puzzle)
	a.cursor = engine.Cell{}
	a.history = nil
}

// Hint implements tui.BoardAdapter: reveals one forced move by finding the
// first (row-major) still-empty cell and filling it from the recorded
// solution.
func (a *tangoAdapter) Hint() {
	if len(a.solution.Cells) != len(a.cells) {
		return // no recorded solution to hint from
	}
	for idx, sym := range a.cells {
		if sym != tangogame.Empty {
			continue
		}
		a.pushHistory()
		a.cells[idx] = a.solution.Cells[idx]
		a.cursor = engine.CellAt(idx, a.puzzle.N)
		return
	}
}

// ---------------------------------------------------------------------------
// Validator delegation.
// ---------------------------------------------------------------------------

func (a *tangoAdapter) board() tangogame.Board {
	return tangogame.Board{N: a.puzzle.N, Cells: a.cells, HEdges: a.puzzle.HEdges, VEdges: a.puzzle.VEdges}
}

// Violations implements tui.BoardAdapter by delegating to the engine
// validator — the adapter never referees the rules itself.
func (a *tangoAdapter) Violations() []engine.Violation {
	return tangogame.Validator{}.Violations(a.board())
}

// Solved implements tui.BoardAdapter by delegating to the engine validator.
func (a *tangoAdapter) Solved() bool {
	return tangogame.Validator{}.Solved(a.board())
}

// GridGeometry implements tui.BoardAdapter. Origin is (0,0): the grid is the
// first thing View renders, so its top-left cell sits at the top-left of the
// string this adapter returns.
func (a *tangoAdapter) GridGeometry() tui.Geometry {
	return tui.Geometry{
		OriginX:    0,
		OriginY:    0,
		CellWidth:  tangoCellWidth,
		CellHeight: tangoCellHeight,
		Rows:       a.puzzle.N,
		Cols:       a.puzzle.N,
		ColGutter:  tangoColGutter,
		RowGutter:  tangoRowGutter,
	}
}

// ---------------------------------------------------------------------------
// Rendering.
// ---------------------------------------------------------------------------

// violationCells returns the set of cell indices any current violation
// names, so the grid renderer can style them in the error style.
func (a *tangoAdapter) violationCells() map[int]bool {
	set := make(map[int]bool)
	for _, v := range a.Violations() {
		for _, c := range v.Cells {
			set[engine.Index(c, a.puzzle.N)] = true
		}
	}
	return set
}

// View implements tui.BoardAdapter.
func (a *tangoAdapter) View(theme tui.Theme) string {
	n := a.puzzle.N
	violCells := a.violationCells()

	lines := make([]string, 0, 2*n-1)
	for r := 0; r < n; r++ {
		lines = append(lines, a.renderCellRow(theme, r, violCells))
		if r < n-1 {
			lines = append(lines, a.renderRowGutter(theme, r))
		}
	}
	// Deliberately joined with plain "\n" rather than lipgloss.JoinVertical:
	// JoinVertical pads every line to the widest block's width (here, the
	// help line), which would silently stretch the grid lines beyond
	// GridGeometry's reported width and break the shell's mouse hit-testing
	// (see TestTango_GeometryMatchesRenderedView).
	grid := strings.Join(lines, "\n")

	dim := lipgloss.NewStyle().Foreground(theme.Dim)
	return grid + "\n\n" + dim.Render(a.helpLine())
}

// helpLine advertises the bindings that are actually active for the current
// keyboard mode, per 03-tui-design.md's "show what's actually active".
func (a *tangoAdapter) helpLine() string {
	if tui.EnhancedKeyboardActive() {
		return "Space: sun (again clears)   Shift+Space: moon (again clears)   u: undo   Ctrl+R: reset   H: hint"
	}
	return "Space: cycle sun/moon/empty   m: moon   u: undo   Ctrl+R: reset   H: hint"
}

func (a *tangoAdapter) renderCellRow(theme tui.Theme, r int, violCells map[int]bool) string {
	n := a.puzzle.N
	var b strings.Builder
	for c := 0; c < n; c++ {
		idx := r*n + c
		b.WriteString(a.renderCell(theme, r, c, idx, violCells))
		if c < n-1 {
			b.WriteString(a.renderColGutter(theme, idx))
		}
	}
	return b.String()
}

func (a *tangoAdapter) renderCell(theme tui.Theme, r, c, idx int, violCells map[int]bool) string {
	sym := a.cells[idx]
	given := a.isGiven(idx)
	isCursor := a.cursor == (engine.Cell{Row: r, Col: c})
	isViol := violCells[idx]

	var text string
	switch sym {
	case tangogame.Sun:
		text = " ☀ "
	case tangogame.Moon:
		text = " ☾ "
	default:
		text = " · "
	}

	style := lipgloss.NewStyle()
	if given {
		style = style.Background(theme.Surface).Bold(true)
	}
	if isCursor {
		style = style.Background(theme.Accent)
	}
	switch {
	case isViol:
		style = style.Foreground(theme.Error).Bold(true)
	case isCursor:
		style = style.Foreground(theme.OnAccent).Bold(true)
	case sym == tangogame.Sun:
		style = style.Foreground(theme.Sun)
	case sym == tangogame.Moon:
		style = style.Foreground(theme.Moon)
	default:
		style = style.Foreground(theme.Dim)
	}
	return style.Render(text)
}

// tangoEdgeGlyph renders an H/V edge relation as its gutter marker: "=" for
// Equal, "×" for Cross, a blank space where there's no constraint.
func tangoEdgeGlyph(rel tangogame.Relation, ok bool) string {
	if !ok {
		return " "
	}
	if rel == tangogame.Equal {
		return "="
	}
	return "×"
}

// renderColGutter renders the horizontal-neighbor marker between the cell at
// leftIdx and the cell immediately to its right (leftIdx+1), both on the same
// row.
func (a *tangoAdapter) renderColGutter(theme tui.Theme, leftIdx int) string {
	rel, ok := a.puzzle.HEdges[[2]int{leftIdx, leftIdx + 1}]
	style := lipgloss.NewStyle().Foreground(theme.Dim)
	if ok {
		style = style.Foreground(theme.Accent).Bold(true)
	}
	return style.Render(tangoEdgeGlyph(rel, ok))
}

// renderRowGutter renders the full-width strip between row r and row r+1,
// with each column's vertical-neighbor marker centered under its cell.
func (a *tangoAdapter) renderRowGutter(theme tui.Theme, r int) string {
	n := a.puzzle.N
	var b strings.Builder
	for c := 0; c < n; c++ {
		idx := r*n + c
		below := idx + n
		rel, ok := a.puzzle.VEdges[[2]int{idx, below}]
		style := lipgloss.NewStyle().Foreground(theme.Dim)
		if ok {
			style = style.Foreground(theme.Accent).Bold(true)
		}
		b.WriteString(style.Render(" " + tangoEdgeGlyph(rel, ok) + " "))
		if c < n-1 {
			b.WriteString(" ") // matches tangoColGutter's width between columns
		}
	}
	return b.String()
}

// Compile-time check that tangoAdapter satisfies tui.BoardAdapter.
var _ tui.BoardAdapter = (*tangoAdapter)(nil)
