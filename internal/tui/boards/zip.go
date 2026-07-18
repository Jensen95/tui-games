// Package boards holds the per-game BoardAdapter implementations. This file
// is the Zip adapter; see docs/plan/games/zip.md ("TUI interaction") and
// docs/plan/docs/03-tui-design.md (board-adapter pattern, the two-handed
// scheme table) for the design this implements.
//
// Every package-level identifier in this file is prefixed with "zip" because
// sibling adapters for the other four games live in this same package and
// are written concurrently by other agents.
package boards

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	zipgame "github.com/Jensen95/tui-games/internal/games/zip"
	"github.com/Jensen95/tui-games/internal/tui"
)

func init() {
	tui.Register(zipgame.ID, zipNew)
}

// Rendering geometry constants for the Zip grid. Cells are 3 columns wide (to
// fit two-digit waypoint numbers) and 1 row tall; a 1-column/1-row gutter
// between cells carries wall markers (thick edges) or the drawn path's
// connector glyphs, per 03-tui-design.md's "Zip: gutter markers ... walls +
// path glyphs" guidance.
const (
	zipCellWidth  = 3
	zipCellHeight = 1
	zipColGutter  = 1
	zipRowGutter  = 1
)

// zipSnapshot is one entry of the undo stack: the adapter's full mutable
// state immediately before a mutating action, so Undo can restore it byte
// for byte.
type zipSnapshot struct {
	path    []int
	cursor  engine.Cell
	penDown bool
}

// zipAdapter implements tui.BoardAdapter for Zip. It never re-implements the
// game's rules: Violations/Solved always delegate to zipgame.Validator, and
// Hint always delegates to the recorded solution (the engine's LogicSolve is
// what produced it at generation time).
type zipAdapter struct {
	puzzle   zipgame.Puzzle
	solution zipgame.Solution

	// path is the drawn (possibly partial or currently-invalid) path, as
	// row-major cell indices — the same representation zipgame.Board.Path
	// uses, so it hands straight to the validator with no conversion.
	path []int
	// cursor is the keyboard cursor / mouse-drag head. While penDown is
	// true it always equals engine.CellAt(path[len(path)-1], C).
	cursor engine.Cell
	// penDown mirrors Zip's "pen down/up" primary action (Space): while
	// down, wasd/arrow movement (and mouse drag) extends or retracts path.
	penDown bool
	// dragging is the mouse-only half of the pen state machine: true
	// between a valid press and the matching release, per
	// 03-tui-design.md's "press -> motion(while held) -> release" pattern.
	dragging bool

	history []zipSnapshot
}

// zipNew is the tui.AdapterFactory for Zip, registered against zipgame.ID.
func zipNew(gen engine.Generated) tui.BoardAdapter {
	puzzle, ok := gen.Puzzle.(zipgame.Puzzle)
	if !ok {
		panic(fmt.Sprintf("boards: zip adapter got unexpected puzzle type %T", gen.Puzzle))
	}
	solution, _ := gen.Solution.(zipgame.Solution) // absent solution just disables Hint

	a := &zipAdapter{puzzle: puzzle, solution: solution}
	a.resetCursorToStart()
	return a
}

// zipStartCell returns the index of the cell numbered 1 (the path's
// mandatory start), mirroring zipgame's unexported startCell — this is a
// plain lookup of the puzzle's own data, not a rule the adapter is
// re-implementing.
func zipStartCell(p zipgame.Puzzle) (int, bool) {
	for cell, num := range p.Waypoint {
		if num == 1 {
			return cell, true
		}
	}
	return 0, false
}

func (a *zipAdapter) resetCursorToStart() {
	if start, ok := zipStartCell(a.puzzle); ok {
		a.cursor = engine.CellAt(start, a.puzzle.C)
		return
	}
	a.cursor = engine.Cell{}
}

// zipPathIndexOf returns the position of cell index idx within path, if any.
func zipPathIndexOf(path []int, idx int) (int, bool) {
	for i, c := range path {
		if c == idx {
			return i, true
		}
	}
	return 0, false
}

// zipAdjacentIdx reports whether two row-major cell indices are orthogonally
// adjacent in a grid with cols columns. Pure geometry, not a rule check.
func zipAdjacentIdx(a, b, cols int) bool {
	ra, ca := a/cols, a%cols
	rb, cb := b/cols, b%cols
	dr := ra - rb
	if dr < 0 {
		dr = -dr
	}
	dc := ca - cb
	if dc < 0 {
		dc = -dc
	}
	return dr+dc == 1
}

func (a *zipAdapter) pushHistory() {
	a.history = append(a.history, zipSnapshot{
		path:    append([]int(nil), a.path...),
		cursor:  a.cursor,
		penDown: a.penDown,
	})
}

// ---------------------------------------------------------------------------
// Keyboard: wasd/arrows/hjkl move the cursor; while the pen is down, movement
// extends the path (or retracts it when moving back onto the previous
// cell). Space toggles the pen; Shift+Space (or the legacy Backspace
// fallback) erases the last segment. See docs/plan/games/zip.md's "TUI
// interaction" and the two-handed scheme table in 03-tui-design.md.
// ---------------------------------------------------------------------------

// zipDirFromKey maps a key press to a unit movement vector, covering
// arrows, wasd, and hjkl (03-tui-design.md: "wasd (draws while pen down)").
func zipDirFromKey(k tea.KeyPressMsg) (engine.Cell, bool) {
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
func (a *zipAdapter) HandleKey(k tea.KeyPressMsg) bool {
	if dir, ok := zipDirFromKey(k); ok {
		return a.move(dir)
	}
	if tui.IsSpace(k) {
		if tui.IsShifted(k) {
			return a.eraseLast()
		}
		return a.togglePen()
	}
	if k.Code == tea.KeyBackspace {
		return a.eraseLast()
	}
	return false
}

// move steps the cursor one cell in dir. While the pen is down this extends
// the path into the target cell — unless the target is the cell immediately
// before the current head, in which case it retracts (LinkedIn's backtrack
// behavior, per the spec). It never validates legality beyond adjacency and
// grid bounds: illegal moves (walls, revisits, wrong waypoint order) are
// still recorded and surface through Violations(), same as any other partial
// state — the adapter is not the referee.
func (a *zipAdapter) move(dir engine.Cell) bool {
	target := engine.Cell{Row: a.cursor.Row + dir.Row, Col: a.cursor.Col + dir.Col}
	if !engine.InBounds(target, a.puzzle.R, a.puzzle.C) {
		return false
	}
	if !a.penDown {
		a.cursor = target
		return true
	}
	idx := engine.Index(target, a.puzzle.C)
	n := len(a.path)
	if n >= 2 && a.path[n-2] == idx {
		a.pushHistory()
		a.path = a.path[:n-1]
		a.cursor = target
		return true
	}
	if n >= 1 && a.path[n-1] == idx {
		return false // target is the current head; nothing to do
	}
	a.pushHistory()
	a.path = append(a.path, idx)
	a.cursor = target
	return true
}

// togglePen implements the primary action (Space): pen down starts (or
// resumes) drawing from the cursor; pen up stops. Resuming mid-path is only
// allowed from the path's current tail — anywhere else, a bare Space is a
// no-op (the player must retract with Shift+Space/Backspace first).
func (a *zipAdapter) togglePen() bool {
	if a.penDown {
		a.penDown = false
		return true
	}
	if len(a.path) == 0 {
		a.pushHistory()
		a.path = []int{engine.Index(a.cursor, a.puzzle.C)}
		a.penDown = true
		return true
	}
	if a.path[len(a.path)-1] != engine.Index(a.cursor, a.puzzle.C) {
		return false
	}
	a.penDown = true
	return true
}

// eraseLast implements the secondary action (Shift+Space) and its legacy
// Backspace fallback: both drop the path's last cell.
func (a *zipAdapter) eraseLast() bool {
	if len(a.path) == 0 {
		return false
	}
	a.pushHistory()
	a.path = a.path[:len(a.path)-1]
	if len(a.path) > 0 {
		a.cursor = engine.CellAt(a.path[len(a.path)-1], a.puzzle.C)
	} else {
		a.penDown = false
	}
	return true
}

// ---------------------------------------------------------------------------
// Mouse: click-drag from cell 1 draws the path; dragging backwards over an
// already-drawn cell retracts to it. This is Zip's defining interaction
// (03-tui-design.md, "Navigation — mouse").
// ---------------------------------------------------------------------------

// HandleMouse implements tui.BoardAdapter.
func (a *zipAdapter) HandleMouse(ev tui.MouseEvent, cell tui.CellRef) bool {
	switch ev.Type {
	case tui.MouseEventPress:
		return a.mousePress(cell)
	case tui.MouseEventMotion:
		return a.mouseMotion(cell)
	case tui.MouseEventRelease:
		a.dragging = false
		return false
	default:
		return false
	}
}

func (a *zipAdapter) mousePress(cell tui.CellRef) bool {
	if !cell.Valid {
		a.dragging = false
		return false
	}
	idx := engine.Index(cell.Cell, a.puzzle.C)

	if pos, ok := zipPathIndexOf(a.path, idx); ok {
		a.dragging = true
		a.penDown = true
		a.cursor = cell.Cell
		if pos+1 == len(a.path) {
			return false // pressed the current head: resume, nothing to retract
		}
		a.pushHistory()
		a.path = a.path[:pos+1]
		return true
	}

	if len(a.path) == 0 {
		a.pushHistory()
		a.path = []int{idx}
		a.dragging = true
		a.penDown = true
		a.cursor = cell.Cell
		return true
	}

	// Pressed somewhere off the current path with a path already in
	// progress: not a valid drag start (Zip's mouse interaction only draws
	// from an existing path endpoint), so ignore rather than clobber
	// progress.
	a.dragging = false
	return false
}

func (a *zipAdapter) mouseMotion(cell tui.CellRef) bool {
	if !a.dragging || !cell.Valid || len(a.path) == 0 {
		return false
	}
	idx := engine.Index(cell.Cell, a.puzzle.C)
	n := len(a.path)

	if a.path[n-1] == idx {
		return false // still over the current head
	}
	if n >= 2 && a.path[n-2] == idx {
		a.pushHistory()
		a.path = a.path[:n-1]
		a.cursor = cell.Cell
		return true
	}
	if pos, ok := zipPathIndexOf(a.path, idx); ok {
		a.pushHistory()
		a.path = a.path[:pos+1]
		a.cursor = cell.Cell
		return true
	}
	head := a.path[n-1]
	if !zipAdjacentIdx(head, idx, a.puzzle.C) {
		return false // drag skipped a cell; ignore rather than teleport
	}
	a.pushHistory()
	a.path = append(a.path, idx)
	a.cursor = cell.Cell
	return true
}

// ---------------------------------------------------------------------------
// Undo / Reset / Hint.
// ---------------------------------------------------------------------------

// Undo implements tui.BoardAdapter.
func (a *zipAdapter) Undo() {
	if len(a.history) == 0 {
		return
	}
	last := a.history[len(a.history)-1]
	a.history = a.history[:len(a.history)-1]
	a.path = last.path
	a.cursor = last.cursor
	a.penDown = last.penDown
}

// Reset implements tui.BoardAdapter: back to the ungenerated-move state (no
// drawn path, pen up, cursor back on the start cell). The puzzle itself
// (waypoints, walls) is never mutated by play, so there is nothing to
// restore there.
func (a *zipAdapter) Reset() {
	a.path = nil
	a.penDown = false
	a.dragging = false
	a.history = nil
	a.resetCursorToStart()
}

// Hint implements tui.BoardAdapter: reveals one forced move by walking the
// recorded solution. It finds the longest prefix of the solution the current
// path still agrees with (which also self-corrects a wrong turn) and plants
// the next solution cell, pen down.
func (a *zipAdapter) Hint() {
	if len(a.solution.Path) == 0 {
		return
	}
	i := 0
	for i < len(a.path) && i < len(a.solution.Path) && a.path[i] == a.solution.Path[i] {
		i++
	}
	if i >= len(a.solution.Path) {
		return // already complete
	}
	a.pushHistory()
	next := a.solution.Path[i]
	a.path = append(append([]int(nil), a.solution.Path[:i]...), next)
	a.cursor = engine.CellAt(next, a.puzzle.C)
	a.penDown = true
}

// ---------------------------------------------------------------------------
// Validator delegation.
// ---------------------------------------------------------------------------

func (a *zipAdapter) board() zipgame.Board {
	return zipgame.Board{Puzzle: a.puzzle, Path: a.path}
}

// Violations implements tui.BoardAdapter by delegating to the engine
// validator — the adapter never referees the rules itself.
func (a *zipAdapter) Violations() []engine.Violation {
	return zipgame.Validator{}.Violations(a.board())
}

// Solved implements tui.BoardAdapter by delegating to the engine validator.
func (a *zipAdapter) Solved() bool {
	return zipgame.Validator{}.Solved(a.board())
}

// GridGeometry implements tui.BoardAdapter. Origin is (0,0): the grid is the
// first thing View renders, so its top-left cell sits at the top-left of the
// string this adapter returns.
func (a *zipAdapter) GridGeometry() tui.Geometry {
	return tui.Geometry{
		OriginX:    0,
		OriginY:    0,
		CellWidth:  zipCellWidth,
		CellHeight: zipCellHeight,
		Rows:       a.puzzle.R,
		Cols:       a.puzzle.C,
		ColGutter:  zipColGutter,
		RowGutter:  zipRowGutter,
	}
}

// ---------------------------------------------------------------------------
// Rendering.
// ---------------------------------------------------------------------------

// violationCells returns the set of cell indices any current violation
// names, so the grid renderer can style them in the error style.
func (a *zipAdapter) violationCells() map[int]bool {
	set := make(map[int]bool)
	for _, v := range a.Violations() {
		for _, c := range v.Cells {
			set[engine.Index(c, a.puzzle.C)] = true
		}
	}
	return set
}

// View implements tui.BoardAdapter.
func (a *zipAdapter) View(theme tui.Theme) string {
	pathPos := make(map[int]int, len(a.path))
	for i, idx := range a.path {
		pathPos[idx] = i
	}
	violCells := a.violationCells()

	R, C := a.puzzle.R, a.puzzle.C
	lines := make([]string, 0, 2*R-1)
	for r := 0; r < R; r++ {
		lines = append(lines, a.renderCellRowImpl(theme, r, pathPos, violCells))
		if r < R-1 {
			lines = append(lines, a.renderGutterRow(theme, r, pathPos))
		}
	}
	grid := strings.Join(lines, "\n")

	remaining := R*C - len(a.path)
	pen := "up"
	if a.penDown {
		pen = "down"
	}
	dimStyle := lipgloss.NewStyle().Foreground(theme.Dim)
	status := dimStyle.Render(fmt.Sprintf("pen: %s   remaining: %d/%d", pen, remaining, R*C))

	help := "Space: pen down/up   Backspace: erase last segment   u: undo   Ctrl+R: reset   H: hint"
	if tui.EnhancedKeyboardActive() {
		help = "Space: pen down/up   Shift+Space: erase last segment   u: undo   Ctrl+R: reset   H: hint"
	}

	// Deliberately not lipgloss.JoinVertical: it pads every line to the
	// block's widest line (here, the help line), which would corrupt the
	// grid's per-cell width that GridGeometry promises to callers doing
	// mouse hit-testing (see TestZip_GeometryMatchesRenderedView).
	return grid + "\n\n" + status + "\n" + dimStyle.Render(help)
}

// renderCellRowImpl renders one grid row of cells, joined with styled
// col-gutter glyphs (wall or path connector) between them.
func (a *zipAdapter) renderCellRowImpl(theme tui.Theme, r int, pathPos map[int]int, violCells map[int]bool) string {
	C := a.puzzle.C
	var b strings.Builder
	for c := 0; c < C; c++ {
		idx := engine.Index(engine.Cell{Row: r, Col: c}, C)
		b.WriteString(a.renderCell(theme, r, c, idx, pathPos, violCells))
		if c < C-1 {
			b.WriteString(a.renderColGutter(theme, r, c, pathPos))
		}
	}
	return b.String()
}

func (a *zipAdapter) renderCell(theme tui.Theme, r, c, idx int, pathPos map[int]int, violCells map[int]bool) string {
	num, hasNum := a.puzzle.Waypoint[idx]
	_, onPath := pathPos[idx]
	isCursor := a.cursor == (engine.Cell{Row: r, Col: c})
	isViolation := violCells[idx]

	var text string
	switch {
	case hasNum:
		text = fmt.Sprintf("%2d ", num)
	case onPath:
		text = " ● " // ●
	default:
		text = " · " // ·
	}

	style := lipgloss.NewStyle()
	if isCursor {
		style = style.Background(theme.Accent)
	}
	switch {
	case isViolation:
		style = style.Foreground(theme.Error).Bold(true)
	case isCursor:
		style = style.Foreground(theme.OnAccent).Bold(true)
	case hasNum:
		style = style.Foreground(theme.Piece).Bold(true)
	case onPath:
		style = style.Foreground(theme.Accent)
	default:
		style = style.Foreground(theme.Dim)
	}
	return style.Render(text)
}

func (a *zipAdapter) renderColGutter(theme tui.Theme, r, c int, pathPos map[int]int) string {
	C := a.puzzle.C
	idxA := engine.Index(engine.Cell{Row: r, Col: c}, C)
	idxB := engine.Index(engine.Cell{Row: r, Col: c + 1}, C)
	hasWall := a.puzzle.Walls[zipgame.WallKey(idxA, idxB)]

	posA, okA := pathPos[idxA]
	posB, okB := pathPos[idxB]
	connected := okA && okB && zipAbsDiff(posA, posB) == 1

	switch {
	case hasWall:
		return lipgloss.NewStyle().Foreground(theme.Warning).Bold(true).Render("┃") // ┃
	case connected:
		return lipgloss.NewStyle().Foreground(theme.Accent).Render("─") // ─
	default:
		return " "
	}
}

func (a *zipAdapter) renderGutterRow(theme tui.Theme, r int, pathPos map[int]int) string {
	C := a.puzzle.C
	var b strings.Builder
	for c := 0; c < C; c++ {
		idxA := engine.Index(engine.Cell{Row: r, Col: c}, C)
		idxB := engine.Index(engine.Cell{Row: r + 1, Col: c}, C)
		hasWall := a.puzzle.Walls[zipgame.WallKey(idxA, idxB)]

		posA, okA := pathPos[idxA]
		posB, okB := pathPos[idxB]
		connected := okA && okB && zipAbsDiff(posA, posB) == 1

		switch {
		case hasWall:
			b.WriteString(lipgloss.NewStyle().Foreground(theme.Warning).Bold(true).Render("━━━")) // ━━━
		case connected:
			b.WriteString(lipgloss.NewStyle().Foreground(theme.Accent).Render(" │ ")) // │
		default:
			b.WriteString("   ")
		}
		if c < C-1 {
			b.WriteString(" ")
		}
	}
	return b.String()
}

func zipAbsDiff(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}

// Compile-time check that zipAdapter satisfies tui.BoardAdapter.
var _ tui.BoardAdapter = (*zipAdapter)(nil)
