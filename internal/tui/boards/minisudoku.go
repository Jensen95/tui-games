// Package boards holds the per-game BoardAdapter implementations. This file
// is the Mini Sudoku adapter; see docs/plan/games/mini-sudoku.md ("TUI
// interaction") and docs/plan/docs/03-tui-design.md (board-adapter pattern,
// the two-handed scheme table) for the design this implements.
//
// Every package-level identifier in this file is prefixed with "minisudoku"
// because sibling adapters for the other four games live in this same
// package and are written concurrently by other agents.
package boards

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	minisudokugame "github.com/Jensen95/tui-games/internal/games/minisudoku"
	"github.com/Jensen95/tui-games/internal/tui"
)

func init() {
	tui.Register(minisudokugame.GameID, minisudokuNew)
}

// minisudokuHintFallbackTechnique labels a Hint reveal that the no-guess
// logic ladder (minisudokugame.Solver.LogicSolve) could not derive on its
// own (e.g. an Expert-tier puzzle needing a guess past the ladder) — the
// cell is still filled from the recorded solution, just without a named
// technique.
const minisudokuHintFallbackTechnique engine.Technique = "solution"

// minisudokuCellWidth/minisudokuCellHeight are derived from the box shape
// (BoxH rows tall, BoxW columns wide) so a cell's interior can lay out
// pencil-mark candidates in the same BoxH×BoxW arrangement as the box itself
// (digit d sits at row (d-1)/BoxW, col (d-1)%BoxW) — see renderCellLines.
// minisudokuColGutter/minisudokuRowGutter are a uniform 1-cell strip between
// every pair of cells; box-boundary strips are drawn bolder than in-box ones
// (docs 03-tui-design.md: "2×3 box borders for Sudoku").
const (
	minisudokuColGutter = 1
	minisudokuRowGutter = 1
)

// minisudokuCellSize returns the rendered interior width/height of one cell
// for a boxH×boxW box shape: width fits boxW single-digit slots separated by
// one space each (2*boxW-1), height is boxH (one line per pencil-mark row).
func minisudokuCellSize(boxH, boxW int) (width, height int) {
	return 2*boxW - 1, boxH
}

// minisudokuSnapshot is one entry of the undo stack: the adapter's full
// mutable state immediately before a mutating action, so Undo can restore it
// byte for byte.
type minisudokuSnapshot struct {
	cells  []int
	notes  []uint8
	cursor engine.Cell
}

// minisudokuAdapter implements tui.BoardAdapter for Mini Sudoku. It never
// re-implements the game's rules: Violations/Solved always delegate to
// minisudokugame.Validator, and Hint always delegates to the recorded
// solution (optionally naming the technique via the engine's logic solver).
type minisudokuAdapter struct {
	puzzle   minisudokugame.Puzzle
	solution minisudokugame.Solution // the complete solved grid; zero value disables Hint

	// cells is the current (partial or complete) player-editable board
	// state, row-major, len N*N: 0 means empty. Givens are pre-seeded here
	// and never mutated. notes holds, per cell, a bitmask of pencil-mark
	// candidates (bit d-1 => digit d); only meaningful while cells[idx]==0.
	cells  []int
	notes  []uint8
	cursor engine.Cell

	// noteMode is the legacy fallback toggle ('e'): while true, a plain
	// digit key toggles a pencil mark instead of placing the digit, letting
	// legacy terminals (which can't distinguish Shift+digit from digit, per
	// tui.IsShifted's doc comment) reach the note-toggle action at all.
	noteMode bool

	// lastHintTechnique records the deepest technique the last Hint call
	// used, for the help/status line; empty until the first Hint.
	lastHintTechnique engine.Technique

	history []minisudokuSnapshot
}

// minisudokuNew is the tui.AdapterFactory for Mini Sudoku, registered against
// minisudokugame.GameID.
func minisudokuNew(gen engine.Generated) tui.BoardAdapter {
	puzzle, ok := gen.Puzzle.(minisudokugame.Puzzle)
	if !ok {
		panic(fmt.Sprintf("boards: minisudoku adapter got unexpected puzzle type %T", gen.Puzzle))
	}
	solution, _ := gen.Solution.(minisudokugame.Solution) // absent solution just disables Hint

	a := &minisudokuAdapter{puzzle: puzzle, solution: solution}
	a.cells = minisudokuInitialCells(puzzle)
	a.notes = make([]uint8, puzzle.N*puzzle.N)
	return a
}

// minisudokuInitialCells builds the givens-only starting board for p: every
// given index seeded with its locked digit, everything else 0 (empty).
func minisudokuInitialCells(p minisudokugame.Puzzle) []int {
	cells := make([]int, p.N*p.N)
	for idx, val := range p.Givens {
		cells[idx] = val
	}
	return cells
}

// isGiven reports whether cell idx is a locked given, immutable to play.
func (a *minisudokuAdapter) isGiven(idx int) bool {
	_, ok := a.puzzle.Givens[idx]
	return ok
}

func (a *minisudokuAdapter) pushHistory() {
	a.history = append(a.history, minisudokuSnapshot{
		cells:  append([]int(nil), a.cells...),
		notes:  append([]uint8(nil), a.notes...),
		cursor: a.cursor,
	})
}

// setDigit is the single mutation path for every digit-changing action
// (place, clear, mouse-wheel cycle): it refuses to touch a given cell, is a
// no-op if the value and notes would be unchanged (so undo history never
// gains an empty entry), always clears any pencil marks once a digit lands
// (marks are meaningless once a cell is resolved), and otherwise records
// history before mutating.
func (a *minisudokuAdapter) setDigit(idx, value int) bool {
	if a.isGiven(idx) {
		return false
	}
	if a.cells[idx] == value && a.notes[idx] == 0 {
		return false
	}
	a.pushHistory()
	a.cells[idx] = value
	a.notes[idx] = 0
	return true
}

// clearCell implements the shared 0/Backspace/Delete/right-click action: if
// the cell holds a digit, clear it; otherwise clear any pencil marks. A
// completely empty, note-free cell is left alone (no-op).
func (a *minisudokuAdapter) clearCell(idx int) bool {
	if a.isGiven(idx) {
		return false
	}
	if a.cells[idx] != 0 {
		return a.setDigit(idx, 0)
	}
	if a.notes[idx] != 0 {
		a.pushHistory()
		a.notes[idx] = 0
		return true
	}
	return false
}

// toggleNote toggles pencil-mark candidate digit on cell idx. Notes only
// apply to still-empty cells: a cell already holding a digit ignores note
// toggles (there is nothing to mark).
func (a *minisudokuAdapter) toggleNote(idx, digit int) bool {
	if a.isGiven(idx) || a.cells[idx] != 0 {
		return false
	}
	a.pushHistory()
	a.notes[idx] ^= uint8(1) << uint(digit-1)
	return true
}

// cycleDigit steps cell idx's value by delta, wrapping through 0 (empty) and
// 1..N. Used by the mouse wheel.
func (a *minisudokuAdapter) cycleDigit(idx, delta int) bool {
	n := a.puzzle.N
	next := ((a.cells[idx]+delta)%(n+1) + (n + 1)) % (n + 1)
	return a.setDigit(idx, next)
}

// ---------------------------------------------------------------------------
// Keyboard: wasd/arrows/hjkl move the cursor. Digits 1-6 place the digit at
// the cursor (primary action); Shift+1-6 toggles a pencil-mark candidate for
// that digit (secondary action) whenever the key press actually carries a
// real Shift modifier (true only on terminals that ack Bubble Tea's keyboard
// enhancements — tui.IsShifted). On a legacy terminal, Shift+digit arrives as
// the layout-dependent shifted glyph with no Mod bit at all and cannot be
// recovered (see tui.IsShifted's doc comment), so 'e' instead toggles a
// note-entry mode: while active, a plain digit key toggles a note instead of
// placing it. 0/Backspace/Delete clears the focused cell, in both modes, per
// docs/plan/docs/03-tui-design.md's two-handed scheme table.
// ---------------------------------------------------------------------------

// minisudokuDirFromKey maps a key press to a unit movement vector, covering
// arrows, wasd, and hjkl.
func minisudokuDirFromKey(k tea.KeyPressMsg) (engine.Cell, bool) {
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

// minisudokuDigitFromKey reports the digit 1..6 a plain (unshifted-glyph) key
// press encodes, if any. This only matches the base digit runes '1'..'6' —
// never a legacy shifted glyph like '!' — per tui.IsShifted's guidance that
// such glyphs must not be reverse-mapped.
func minisudokuDigitFromKey(k tea.KeyPressMsg) (int, bool) {
	switch k.Code {
	case '1', '2', '3', '4', '5', '6':
		return int(k.Code - '0'), true
	default:
		return 0, false
	}
}

// HandleKey implements tui.BoardAdapter.
func (a *minisudokuAdapter) HandleKey(k tea.KeyPressMsg) bool {
	if dir, ok := minisudokuDirFromKey(k); ok {
		return a.move(dir)
	}

	idx := engine.Index(a.cursor, a.puzzle.N)

	if d, ok := minisudokuDigitFromKey(k); ok {
		if tui.IsShifted(k) || a.noteMode {
			return a.toggleNote(idx, d)
		}
		return a.setDigit(idx, d)
	}

	switch k.Code {
	case '0', tea.KeyBackspace, tea.KeyDelete:
		return a.clearCell(idx)
	case 'e':
		a.noteMode = !a.noteMode
		return true
	}
	return false
}

// move steps the cursor one cell in dir, clamped to the grid.
func (a *minisudokuAdapter) move(dir engine.Cell) bool {
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

// ---------------------------------------------------------------------------
// Mouse: left-click focuses a cell (click-then-type, per
// docs/plan/games/mini-sudoku.md's "Mouse" section); right-click clears it;
// the wheel cycles the focused cell's digit (0 -> 1 -> ... -> N -> 0), an
// optional extra per 03-tui-design.md ("wheel events can adjust... cycle
// candidates").
// ---------------------------------------------------------------------------

// HandleMouse implements tui.BoardAdapter.
func (a *minisudokuAdapter) HandleMouse(ev tui.MouseEvent, cell tui.CellRef) bool {
	if !cell.Valid {
		return false
	}
	idx := engine.Index(cell.Cell, a.puzzle.N)

	switch ev.Type {
	case tui.MouseEventPress:
		switch ev.Button {
		case tea.MouseLeft:
			moved := a.cursor != cell.Cell
			a.cursor = cell.Cell
			return moved
		case tea.MouseRight:
			a.cursor = cell.Cell
			return a.clearCell(idx)
		default:
			return false
		}
	case tui.MouseEventWheel:
		a.cursor = cell.Cell
		switch ev.Button {
		case tea.MouseWheelUp:
			return a.cycleDigit(idx, 1)
		case tea.MouseWheelDown:
			return a.cycleDigit(idx, -1)
		default:
			return false
		}
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Undo / Reset / Hint.
// ---------------------------------------------------------------------------

// Undo implements tui.BoardAdapter.
func (a *minisudokuAdapter) Undo() {
	if len(a.history) == 0 {
		return
	}
	last := a.history[len(a.history)-1]
	a.history = a.history[:len(a.history)-1]
	a.cells = last.cells
	a.notes = last.notes
	a.cursor = last.cursor
}

// Reset implements tui.BoardAdapter: back to the ungenerated-move state
// (givens only, no notes, cursor at the origin, note mode off, undo history
// cleared).
func (a *minisudokuAdapter) Reset() {
	a.cells = minisudokuInitialCells(a.puzzle)
	a.notes = make([]uint8, a.puzzle.N*a.puzzle.N)
	a.cursor = engine.Cell{}
	a.noteMode = false
	a.lastHintTechnique = ""
	a.history = nil
}

// minisudokuNextHint finds the next cell Hint should reveal: it first asks
// the engine's no-guess logic solver (seeded with the current board, givens
// and player-entered digits alike, as its "givens") to close as much as it
// can, and reveals the first still-empty cell the ladder managed to fill —
// genuinely educational, per 03-tui-design.md ("Hints call
// LogicSolve/forced-move logic ... names the technique"). If the ladder made
// no progress at all (e.g. an Expert-tier puzzle needing a guess beyond it),
// it falls back to the first still-empty cell, labelled with
// minisudokuHintFallbackTechnique instead of a real technique name.
func (a *minisudokuAdapter) minisudokuNextHint() (idx int, technique engine.Technique) {
	givens := make(map[int]int, len(a.cells))
	for i, v := range a.cells {
		if v != 0 {
			givens[i] = v
		}
	}
	temp := minisudokugame.Puzzle{N: a.puzzle.N, BoxH: a.puzzle.BoxH, BoxW: a.puzzle.BoxW, Givens: givens}
	ladderSol, _, tech := minisudokugame.Solver{}.LogicSolve(temp)

	for i, v := range a.cells {
		if v == 0 && i < len(ladderSol.Cells) && ladderSol.Cells[i] != 0 {
			return i, tech
		}
	}
	for i, v := range a.cells {
		if v == 0 {
			return i, minisudokuHintFallbackTechnique
		}
	}
	return -1, ""
}

// Hint implements tui.BoardAdapter: reveals one forced move (see
// minisudokuNextHint) from the recorded solution, which is always the
// authoritative value regardless of which path found the cell.
func (a *minisudokuAdapter) Hint() {
	if len(a.solution.Cells) != len(a.cells) {
		return // no recorded solution to hint from
	}
	idx, technique := a.minisudokuNextHint()
	if idx < 0 {
		return // board already full
	}
	a.pushHistory()
	a.cells[idx] = a.solution.Cells[idx]
	a.notes[idx] = 0
	a.cursor = engine.CellAt(idx, a.puzzle.N)
	a.lastHintTechnique = technique
}

// ---------------------------------------------------------------------------
// Validator delegation.
// ---------------------------------------------------------------------------

func (a *minisudokuAdapter) board() minisudokugame.Board {
	return minisudokugame.Board{Cells: a.cells}
}

// Violations implements tui.BoardAdapter by delegating to the engine
// validator — the adapter never referees the rules itself.
func (a *minisudokuAdapter) Violations() []engine.Violation {
	return minisudokugame.Validator{}.Violations(a.board())
}

// Solved implements tui.BoardAdapter by delegating to the engine validator.
func (a *minisudokuAdapter) Solved() bool {
	return minisudokugame.Validator{}.Solved(a.board())
}

// violationCells returns the set of cell indices any current violation
// names, so the grid renderer can style them in the error style.
func (a *minisudokuAdapter) violationCells() map[int]bool {
	set := make(map[int]bool)
	for _, v := range a.Violations() {
		for _, c := range v.Cells {
			set[engine.Index(c, a.puzzle.N)] = true
		}
	}
	return set
}

// GridGeometry implements tui.BoardAdapter. Origin is (0,0): the grid is the
// first thing View renders, so its top-left cell sits at the top-left of the
// string this adapter returns.
func (a *minisudokuAdapter) GridGeometry() tui.Geometry {
	w, h := minisudokuCellSize(a.puzzle.BoxH, a.puzzle.BoxW)
	return tui.Geometry{
		OriginX:    0,
		OriginY:    0,
		CellWidth:  w,
		CellHeight: h,
		Rows:       a.puzzle.N,
		Cols:       a.puzzle.N,
		ColGutter:  minisudokuColGutter,
		RowGutter:  minisudokuRowGutter,
	}
}

// ---------------------------------------------------------------------------
// Rendering.
// ---------------------------------------------------------------------------

// gridWidth is the total rendered width of one grid line (cells + column
// gutters), used to size the full-width row-gutter divider lines.
func (a *minisudokuAdapter) gridWidth() int {
	geo := a.GridGeometry()
	return geo.Cols*geo.CellWidth + (geo.Cols-1)*geo.ColGutter
}

// minisudokuCenterDigit renders val centered in a boxW-derived cell width
// (2*boxW-1), e.g. "  4  " for boxW==3.
func minisudokuCenterDigit(val, boxW int) string {
	width := 2*boxW - 1
	s := strconv.Itoa(val)
	total := width - len(s)
	if total < 0 {
		return s
	}
	left := total / 2
	right := total - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// cellLines returns the cellHeight (BoxH) lines of unstyled text for the
// cell at idx: a placed digit centered on the first line (blank lines
// beneath), or — for an empty cell — a BoxH×BoxW grid of pencil-mark digits
// (blank where the candidate is not set), digit d sitting at row (d-1)/BoxW,
// col (d-1)%BoxW so its on-screen position never moves as marks come and go.
func (a *minisudokuAdapter) cellLines(idx int) []string {
	boxH, boxW := a.puzzle.BoxH, a.puzzle.BoxW
	width, _ := minisudokuCellSize(boxH, boxW)
	lines := make([]string, boxH)

	if val := a.cells[idx]; val != 0 {
		lines[0] = minisudokuCenterDigit(val, boxW)
		for r := 1; r < boxH; r++ {
			lines[r] = strings.Repeat(" ", width)
		}
		return lines
	}

	notes := a.notes[idx]
	for r := 0; r < boxH; r++ {
		var b strings.Builder
		for c := 0; c < boxW; c++ {
			d := r*boxW + c + 1
			if d <= a.puzzle.N && notes&(uint8(1)<<uint(d-1)) != 0 {
				b.WriteString(strconv.Itoa(d))
			} else {
				b.WriteString(" ")
			}
			if c < boxW-1 {
				b.WriteString(" ")
			}
		}
		lines[r] = b.String()
	}
	return lines
}

// cellStyle returns the style applied uniformly to every rendered line of
// the cell at (row, col): given digits are bold, the cursor cell gets an
// accent background, and cells named by a live violation are styled in the
// theme's error color.
func (a *minisudokuAdapter) cellStyle(theme tui.Theme, row, col, idx int, violCells map[int]bool) lipgloss.Style {
	style := lipgloss.NewStyle()
	given := a.isGiven(idx)
	isCursor := a.cursor == (engine.Cell{Row: row, Col: col})
	isViol := violCells[idx]

	if given {
		style = style.Bold(true)
	}
	if isCursor {
		style = style.Background(theme.Accent)
	}
	switch {
	case isViol:
		style = style.Foreground(theme.Error).Bold(true)
	case isCursor:
		style = style.Foreground(theme.OnAccent)
	case given:
		style = style.Foreground(theme.Text)
	case a.cells[idx] != 0:
		style = style.Foreground(theme.Piece)
	default:
		style = style.Foreground(theme.Dim)
	}
	return style
}

// colGutterGlyph renders the single-column strip between the cell at leftCol
// and leftCol+1 (both in the same row): a bold accent bar at a box boundary
// (every BoxW columns), otherwise blank.
func (a *minisudokuAdapter) colGutterGlyph(theme tui.Theme, leftCol int) string {
	if (leftCol+1)%a.puzzle.BoxW == 0 {
		return lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("│")
	}
	return " "
}

// rowGutterLine renders the full-width strip between logical row r and r+1:
// a bold horizontal rule at a box boundary (every BoxH rows), otherwise
// blank, per 03-tui-design.md's "2×3 box borders for Sudoku" guidance.
func (a *minisudokuAdapter) rowGutterLine(theme tui.Theme, r int) string {
	width := a.gridWidth()
	if (r+1)%a.puzzle.BoxH == 0 {
		return lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render(strings.Repeat("─", width))
	}
	return strings.Repeat(" ", width)
}

// renderGrid renders the full board: for every logical row, BoxH lines of
// cell interiors (each column separated by colGutterGlyph), then — between
// logical rows — one rowGutterLine strip.
func (a *minisudokuAdapter) renderGrid(theme tui.Theme) string {
	n, boxH := a.puzzle.N, a.puzzle.BoxH
	violCells := a.violationCells()

	var rows []string
	for r := 0; r < n; r++ {
		cellLinesPerCol := make([][]string, n)
		for c := 0; c < n; c++ {
			cellLinesPerCol[c] = a.cellLines(r*n + c)
		}
		for sub := 0; sub < boxH; sub++ {
			var b strings.Builder
			for c := 0; c < n; c++ {
				idx := r*n + c
				style := a.cellStyle(theme, r, c, idx, violCells)
				b.WriteString(style.Render(cellLinesPerCol[c][sub]))
				if c < n-1 {
					b.WriteString(a.colGutterGlyph(theme, c))
				}
			}
			rows = append(rows, b.String())
		}
		if r < n-1 {
			rows = append(rows, a.rowGutterLine(theme, r))
		}
	}
	// Deliberately joined with plain "\n" rather than lipgloss.JoinVertical:
	// JoinVertical pads every line to the widest block's width (here, the
	// help line), which would silently stretch the grid lines beyond
	// GridGeometry's reported width and break the shell's mouse hit-testing
	// (see TestMiniSudoku_GeometryMatchesRenderedView).
	return strings.Join(rows, "\n")
}

// View implements tui.BoardAdapter.
func (a *minisudokuAdapter) View(theme tui.Theme) string {
	grid := a.renderGrid(theme)
	dim := lipgloss.NewStyle().Foreground(theme.Dim)

	sections := []string{grid, "", dim.Render(a.helpLine())}
	if a.lastHintTechnique != "" {
		sections = append(sections, dim.Render("Hint used: "+string(a.lastHintTechnique)))
	}
	return strings.Join(sections, "\n")
}

// helpLine advertises the bindings that are actually active for the current
// keyboard mode, per 03-tui-design.md's "show what's actually active".
func (a *minisudokuAdapter) helpLine() string {
	if tui.EnhancedKeyboardActive() {
		return "1-6: place digit   Shift+1-6: toggle note   0/Backspace: clear   u: undo   Ctrl+R: reset   H: hint"
	}
	mode := "off"
	if a.noteMode {
		mode = "ON"
	}
	return fmt.Sprintf("1-6: place digit   e: toggle note mode [%s]   0/Backspace: clear   u: undo   Ctrl+R: reset   H: hint", mode)
}

// Compile-time check that minisudokuAdapter satisfies tui.BoardAdapter.
var _ tui.BoardAdapter = (*minisudokuAdapter)(nil)
