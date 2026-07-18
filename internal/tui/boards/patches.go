// Package boards holds the per-game BoardAdapter implementations. This file
// is the Patches adapter; see docs/plan/games/patches.md ("TUI interaction")
// and docs/plan/docs/03-tui-design.md (board-adapter pattern, the two-handed
// scheme table) for the design this implements.
//
// Every package-level identifier in this file is prefixed with "patches"
// because sibling adapters for the other four games live in this same
// package and are written concurrently by other agents.
package boards

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	pz "github.com/Jensen95/tui-games/internal/games/patches"
	"github.com/Jensen95/tui-games/internal/tui"
)

func init() {
	tui.Register(pz.GameID, patchesNew)
}

// patchesBox is an axis-aligned rectangle in cell coordinates, inclusive on
// both ends (r0..r1, c0..c1), the shape the adapter's active (uncommitted)
// rectangle takes while the player is anchoring/stretching it.
type patchesBox struct {
	r0, c0, r1, c1 int
}

func (b patchesBox) w() int { return b.c1 - b.c0 + 1 }
func (b patchesBox) h() int { return b.r1 - b.r0 + 1 }

func (b patchesBox) contains(r, c int) bool {
	return r >= b.r0 && r <= b.r1 && c >= b.c0 && c <= b.c1
}

// patchesBoxFromPoints is the bounding box of two cells, inclusive — the
// mouse drag's rectangle model (press corner + current corner).
func patchesBoxFromPoints(a, b engine.Cell) patchesBox {
	r0, r1 := a.Row, b.Row
	if r0 > r1 {
		r0, r1 = r1, r0
	}
	c0, c1 := a.Col, b.Col
	if c0 > c1 {
		c0, c1 = c1, c0
	}
	return patchesBox{r0: r0, c0: c0, r1: r1, c1: c1}
}

// patchesSnapshot is one entry of the undo stack: the board's cell labeling
// and the label counter immediately before a mutating action, so Undo can
// restore it exactly.
type patchesSnapshot struct {
	cells     []int
	nextLabel int
}

// patchesAdapter implements tui.BoardAdapter for Patches. It never
// re-implements the game's rules: Violations/Solved always delegate to
// pz.Validator, and Hint always delegates to the recorded solution (the
// unique tiling the generator produced).
type patchesAdapter struct {
	puzzle    *pz.Puzzle
	solution  *pz.Solution
	board     *pz.Board
	validator *pz.Validator

	rows, cols int
	// cellWidth is the rendered width (in columns) of one cell's body, sized
	// to fit the largest clue number plus one shape-icon column. Computed
	// once at construction so GridGeometry and View never disagree.
	cellWidth int

	// cursor is the keyboard cursor. While hasAnchor is true it stays frozen
	// at the anchor cell (the clue where the active rectangle started); the
	// active rectangle itself is tracked in box.
	cursor engine.Cell

	// hasAnchor/anchor/box implement the two-handed scheme's rectangle
	// interaction: Space on a clue cell anchors box as a 1x1 rectangle
	// there; wasd/arrows "stretch" box outward one edge at a time (see
	// stretch); Space again commits it; Shift+Space/x cancels it.
	hasAnchor bool
	anchor    engine.Cell
	box       patchesBox

	// nextLabel is the rectangle-id counter: each committed rectangle (via
	// keyboard, mouse, or Hint) gets the next integer, monotonically
	// increasing even across Undo so stale labels are never reused for a
	// different rectangle mid-history.
	nextLabel int

	history []patchesSnapshot
}

// patchesNew is the tui.AdapterFactory for Patches, registered against
// pz.GameID.
func patchesNew(gen engine.Generated) tui.BoardAdapter {
	puzzle, ok := gen.Puzzle.(*pz.Puzzle)
	if !ok {
		panic(fmt.Sprintf("boards: patches adapter got unexpected puzzle type %T", gen.Puzzle))
	}
	solution, _ := gen.Solution.(*pz.Solution) // absent solution just disables Hint

	return &patchesAdapter{
		puzzle:    puzzle,
		solution:  solution,
		board:     pz.NewBoard(puzzle),
		validator: pz.NewValidator(puzzle),
		rows:      puzzle.R,
		cols:      puzzle.C,
		cellWidth: patchesCellWidth(puzzle),
	}
}

// patchesCellWidth sizes a cell's body to fit the widest clue number (in
// digits) plus one column for the shape icon, with a floor of 3 so small
// puzzles still render with a comfortable gutter.
func patchesCellWidth(p *pz.Puzzle) int {
	digits := 1
	for _, c := range p.Clues {
		if n := len(strconv.Itoa(c.Number)); n > digits {
			digits = n
		}
	}
	w := digits + 1
	if w < 3 {
		w = 3
	}
	return w
}

// patchesShapeIcon returns the accessibility-channel glyph for a shape,
// per docs/plan/games/patches.md's "Rendering" note (□ square, ▭ wide,
// ▯ tall, ◇ free) — the non-color signal so rectangle identity/shape reads
// under NO_COLOR or colorblind vision, not just via the fill color.
func patchesShapeIcon(s pz.Shape) string {
	switch s {
	case pz.Square:
		return "□"
	case pz.Wide:
		return "▭"
	case pz.Tall:
		return "▯"
	default: // pz.Free
		return "◇"
	}
}

// ---------------------------------------------------------------------------
// Keyboard: wasd/arrows/hjkl move the cursor when no rectangle is being
// drawn; Space on a clue anchors one there and the same keys then stretch
// it; Space commits, Shift+Space (or the legacy 'x' fallback) cancels the
// active rectangle or removes the placed one under the cursor. See
// docs/plan/games/patches.md's "TUI interaction" and the two-handed scheme
// table in 03-tui-design.md.
// ---------------------------------------------------------------------------

// patchesDirFromKey maps a key press to a unit movement vector, covering
// arrows, wasd, and hjkl.
func patchesDirFromKey(k tea.KeyPressMsg) (dr, dc int, ok bool) {
	switch k.Code {
	case tea.KeyUp, 'w', 'k':
		return -1, 0, true
	case tea.KeyDown, 's', 'j':
		return 1, 0, true
	case tea.KeyLeft, 'a', 'h':
		return 0, -1, true
	case tea.KeyRight, 'd', 'l':
		return 0, 1, true
	default:
		return 0, 0, false
	}
}

// HandleKey implements tui.BoardAdapter.
func (a *patchesAdapter) HandleKey(k tea.KeyPressMsg) bool {
	if dr, dc, ok := patchesDirFromKey(k); ok {
		if a.hasAnchor {
			a.stretch(dr, dc)
		} else {
			a.moveCursor(dr, dc)
		}
		return false
	}

	switch {
	case tui.IsSpace(k) && tui.IsShifted(k):
		// Enhanced-terminal secondary action.
		return a.secondaryAction()
	case tui.IsSpace(k) || k.Code == tea.KeyEnter:
		// Primary action: Space is the documented key; Enter is accepted too,
		// mirroring KeyMap.PrimaryAction's "space/enter" accommodation.
		return a.primaryAction()
	case k.Code == 'x' || k.Text == "x":
		// Legacy fallback: always works, regardless of keyboard-enhancement
		// support, so both input paths are fully functional per
		// 03-tui-design.md's two-handed scheme table.
		return a.secondaryAction()
	}
	return false
}

func (a *patchesAdapter) moveCursor(dr, dc int) {
	r, c := a.cursor.Row+dr, a.cursor.Col+dc
	if r < 0 {
		r = 0
	}
	if r >= a.rows {
		r = a.rows - 1
	}
	if c < 0 {
		c = 0
	}
	if c >= a.cols {
		c = a.cols - 1
	}
	a.cursor = engine.Cell{Row: r, Col: c}
}

// stretch extends one edge of the active rectangle outward by one cell,
// clamped to the grid. Growing is monotonic (there is no shrink key) —
// overshooting is corrected by canceling (Shift+Space/x) and re-anchoring,
// which keeps the interaction unambiguous with only four directional keys.
func (a *patchesAdapter) stretch(dr, dc int) {
	switch {
	case dr < 0:
		if a.box.r0 > 0 {
			a.box.r0--
		}
	case dr > 0:
		if a.box.r1 < a.rows-1 {
			a.box.r1++
		}
	case dc < 0:
		if a.box.c0 > 0 {
			a.box.c0--
		}
	case dc > 0:
		if a.box.c1 < a.cols-1 {
			a.box.c1++
		}
	}
}

// primaryAction implements Space: anchor a new rectangle at the cursor if
// it sits on an uncovered clue, otherwise commit the active rectangle.
func (a *patchesAdapter) primaryAction() bool {
	if !a.hasAnchor {
		idx := engine.Index(a.cursor, a.cols)
		if _, isClue := a.puzzle.Clues[idx]; !isClue {
			return false
		}
		if a.board.Cells[idx] != -1 {
			return false // already covered by a placed rectangle
		}
		a.hasAnchor = true
		a.anchor = a.cursor
		a.box = patchesBox{r0: a.cursor.Row, c0: a.cursor.Col, r1: a.cursor.Row, c1: a.cursor.Col}
		return false
	}
	return a.commitActive()
}

// secondaryAction implements Shift+Space/x: cancel the active rectangle if
// one is being drawn, otherwise remove the placed rectangle under the
// cursor (if any).
func (a *patchesAdapter) secondaryAction() bool {
	if a.hasAnchor {
		a.hasAnchor = false
		return false
	}
	return a.removeRectAt(a.cursor)
}

// commitActive places the active rectangle if every cell it covers is
// currently uncovered. It rejects (no-op, keeping the rectangle active so
// the player can adjust and retry) rather than overwriting another
// rectangle's cells, since the board's one-label-per-cell representation
// can't otherwise distinguish "overlap" from "replace." Whether the
// committed rectangle actually satisfies its clue's area/shape is left
// entirely to the engine validator's live feedback — this only enforces the
// bookkeeping the data structure requires.
func (a *patchesAdapter) commitActive() bool {
	if !a.hasAnchor {
		return false
	}
	b := a.box
	for r := b.r0; r <= b.r1; r++ {
		for c := b.c0; c <= b.c1; c++ {
			if a.board.Cells[r*a.cols+c] != -1 {
				return false
			}
		}
	}
	a.pushHistory()
	label := a.nextLabel
	a.nextLabel++
	for r := b.r0; r <= b.r1; r++ {
		for c := b.c0; c <= b.c1; c++ {
			a.board.Cells[r*a.cols+c] = label
		}
	}
	a.hasAnchor = false
	a.cursor = a.anchor
	return true
}

// removeRectAt clears the placed rectangle owning cell, if any.
func (a *patchesAdapter) removeRectAt(cell engine.Cell) bool {
	idx := engine.Index(cell, a.cols)
	label := a.board.Cells[idx]
	if label == -1 {
		return false
	}
	a.pushHistory()
	for i, l := range a.board.Cells {
		if l == label {
			a.board.Cells[i] = -1
		}
	}
	return true
}

func (a *patchesAdapter) pushHistory() {
	a.history = append(a.history, patchesSnapshot{
		cells:     append([]int(nil), a.board.Cells...),
		nextLabel: a.nextLabel,
	})
}

// ---------------------------------------------------------------------------
// Mouse: press on a clue and drag to the opposite corner to define a
// rectangle; release commits; click a placed rectangle to remove it. This is
// Patches' defining interaction (03-tui-design.md, "Navigation — mouse").
// ---------------------------------------------------------------------------

// HandleMouse implements tui.BoardAdapter.
func (a *patchesAdapter) HandleMouse(ev tui.MouseEvent, cell tui.CellRef) bool {
	switch ev.Type {
	case tui.MouseEventPress:
		return a.mousePress(ev, cell)
	case tui.MouseEventMotion:
		return a.mouseMotion(cell)
	case tui.MouseEventRelease:
		return a.mouseRelease(cell)
	default:
		return false
	}
}

func (a *patchesAdapter) mousePress(ev tui.MouseEvent, cell tui.CellRef) bool {
	if ev.Button != tea.MouseLeft || !cell.Valid {
		return false
	}
	a.cursor = cell.Cell
	idx := engine.Index(cell.Cell, a.cols)
	if a.board.Cells[idx] != -1 {
		a.hasAnchor = false
		return a.removeRectAt(cell.Cell)
	}
	if _, isClue := a.puzzle.Clues[idx]; isClue {
		a.hasAnchor = true
		a.anchor = cell.Cell
		a.box = patchesBox{r0: cell.Cell.Row, c0: cell.Cell.Col, r1: cell.Cell.Row, c1: cell.Cell.Col}
	}
	return false
}

func (a *patchesAdapter) mouseMotion(cell tui.CellRef) bool {
	if !a.hasAnchor || !cell.Valid {
		return false
	}
	a.cursor = cell.Cell
	a.box = patchesBoxFromPoints(a.anchor, cell.Cell)
	return false
}

func (a *patchesAdapter) mouseRelease(cell tui.CellRef) bool {
	if !a.hasAnchor {
		return false
	}
	if cell.Valid {
		a.cursor = cell.Cell
		a.box = patchesBoxFromPoints(a.anchor, cell.Cell)
	}
	changed := a.commitActive()
	a.hasAnchor = false // the drag always ends on release, whether or not it committed
	return changed
}

// ---------------------------------------------------------------------------
// Undo / Reset / Hint.
// ---------------------------------------------------------------------------

// Undo implements tui.BoardAdapter.
func (a *patchesAdapter) Undo() {
	if len(a.history) == 0 {
		return
	}
	last := a.history[len(a.history)-1]
	a.history = a.history[:len(a.history)-1]
	a.board.Cells = last.cells
	a.nextLabel = last.nextLabel
	a.hasAnchor = false
}

// Reset implements tui.BoardAdapter: back to the ungenerated-move state (an
// entirely uncovered board). The puzzle's clues are never mutated by play,
// so there is nothing to restore there.
func (a *patchesAdapter) Reset() {
	a.board = pz.NewBoard(a.puzzle)
	a.history = nil
	a.nextLabel = 0
	a.hasAnchor = false
	a.cursor = engine.Cell{}
}

// patchesClueForRect returns the anchor-cell index of the one clue rect
// contains, per the generation invariant that every solution rectangle
// contains exactly one clue.
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

// patchesHintCandidate pairs a solution rectangle with its clue's anchor
// index, so hints can be offered in a stable, deterministic order.
type patchesHintCandidate struct {
	clueIdx int
	rect    pz.Rect
}

func (a *patchesAdapter) sortedSolutionRects() []patchesHintCandidate {
	cands := make([]patchesHintCandidate, 0, len(a.solution.Rects))
	for _, rect := range a.solution.Rects {
		idx, ok := patchesClueForRect(a.puzzle, rect)
		if !ok {
			continue
		}
		cands = append(cands, patchesHintCandidate{clueIdx: idx, rect: rect})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].clueIdx < cands[j].clueIdx })
	return cands
}

// rectMatchesBoard reports whether rect is already exactly reflected on the
// board: every one of its cells shares a single label, and that label
// doesn't leak outside rect's bounds.
func (a *patchesAdapter) rectMatchesBoard(rect pz.Rect) bool {
	label := -2
	for r := rect.R0; r < rect.R0+rect.H; r++ {
		for c := rect.C0; c < rect.C0+rect.W; c++ {
			l := a.board.Cells[r*a.cols+c]
			if label == -2 {
				label = l
			} else if l != label {
				return false
			}
		}
	}
	if label < 0 {
		return false
	}
	for i, l := range a.board.Cells {
		if l != label {
			continue
		}
		cell := engine.CellAt(i, a.cols)
		if cell.Row < rect.R0 || cell.Row >= rect.R0+rect.H || cell.Col < rect.C0 || cell.Col >= rect.C0+rect.W {
			return false
		}
	}
	return true
}

// applyHintRect clears any rectangle currently overlapping rect's cells
// (in full, so no fragment is left behind) and then places rect under a
// fresh label.
func (a *patchesAdapter) applyHintRect(rect pz.Rect) {
	stale := map[int]bool{}
	for r := rect.R0; r < rect.R0+rect.H; r++ {
		for c := rect.C0; c < rect.C0+rect.W; c++ {
			if l := a.board.Cells[r*a.cols+c]; l != -1 {
				stale[l] = true
			}
		}
	}
	for l := range stale {
		for i, cl := range a.board.Cells {
			if cl == l {
				a.board.Cells[i] = -1
			}
		}
	}
	label := a.nextLabel
	a.nextLabel++
	for r := rect.R0; r < rect.R0+rect.H; r++ {
		for c := rect.C0; c < rect.C0+rect.W; c++ {
			a.board.Cells[r*a.cols+c] = label
		}
	}
}

// Hint implements tui.BoardAdapter: reveals one forced rectangle by walking
// the recorded solution (in clue-index order) for the first one not yet
// exactly placed, clearing anything wrongly occupying its cells and
// planting it. A no-op if there's no recorded solution or every clue is
// already correctly placed.
func (a *patchesAdapter) Hint() {
	if a.solution == nil {
		return
	}
	for _, cand := range a.sortedSolutionRects() {
		if a.rectMatchesBoard(cand.rect) {
			continue
		}
		a.pushHistory()
		a.applyHintRect(cand.rect)
		a.hasAnchor = false
		a.cursor = engine.Cell{Row: cand.rect.R0, Col: cand.rect.C0}
		return
	}
}

// ---------------------------------------------------------------------------
// Validator delegation.
// ---------------------------------------------------------------------------

// Violations implements tui.BoardAdapter by delegating to the engine
// validator — the adapter never referees the rules itself.
func (a *patchesAdapter) Violations() []engine.Violation {
	return a.validator.Violations(a.board)
}

// Solved implements tui.BoardAdapter by delegating to the engine validator.
func (a *patchesAdapter) Solved() bool {
	return a.validator.Solved(a.board)
}

// GridGeometry implements tui.BoardAdapter. Origin is (0,0): the grid is the
// first thing View renders, so its top-left cell sits at the top-left of the
// string this adapter returns.
func (a *patchesAdapter) GridGeometry() tui.Geometry {
	return tui.Geometry{
		OriginX:    0,
		OriginY:    0,
		CellWidth:  a.cellWidth,
		CellHeight: 1,
		Rows:       a.rows,
		Cols:       a.cols,
		ColGutter:  1,
		RowGutter:  1,
	}
}

// ---------------------------------------------------------------------------
// Rendering.
// ---------------------------------------------------------------------------

// patchesViolationCells returns the set of cell indices any current
// area/shape violation names, so the grid renderer can style them in the
// error style. Global violations (exact-cover gaps, one-clue mismatches)
// have no specific Cells and are surfaced only in the shell's violations
// list, not as per-cell highlighting — highlighting every still-uncovered
// cell red for the ever-present "not fully covered yet" signal would be
// noise, not feedback.
func (a *patchesAdapter) patchesViolationCells() map[int]bool {
	set := make(map[int]bool)
	for _, v := range a.Violations() {
		for _, c := range v.Cells {
			set[engine.Index(c, a.cols)] = true
		}
	}
	return set
}

// labelShape returns the shape of the clue owning label, so cells covered by
// a rectangle but not themselves the clue cell can still render the
// shape-glyph fill (the colorblind-safe channel, alongside the fill color).
func (a *patchesAdapter) labelShape(label int) pz.Shape {
	for idx, clue := range a.puzzle.Clues {
		if a.board.Cells[idx] == label {
			return clue.Shape
		}
	}
	return pz.Free
}

// View implements tui.BoardAdapter.
func (a *patchesAdapter) View(theme tui.Theme) string {
	violCells := a.patchesViolationCells()
	cursorIdx := engine.Index(a.cursor, a.cols)

	lines := make([]string, 0, 2*a.rows-1)
	for r := 0; r < a.rows; r++ {
		lines = append(lines, a.renderRow(theme, r, cursorIdx, violCells))
		if r < a.rows-1 {
			lines = append(lines, a.renderRowGutter(theme))
		}
	}
	grid := strings.Join(lines, "\n")

	// Deliberately joined with plain "\n" rather than lipgloss.JoinVertical:
	// JoinVertical pads every line to the widest block's width (here, the
	// legend line), which would silently stretch the grid lines beyond
	// GridGeometry's reported width and break the shell's mouse hit-testing.
	dim := lipgloss.NewStyle().Foreground(theme.Dim)
	return grid + "\n\n" + dim.Render(a.legend())
}

func (a *patchesAdapter) renderRow(theme tui.Theme, r, cursorIdx int, violCells map[int]bool) string {
	var b strings.Builder
	for c := 0; c < a.cols; c++ {
		idx := r*a.cols + c
		b.WriteString(a.renderCell(theme, r, c, idx, cursorIdx, violCells))
		if c < a.cols-1 {
			b.WriteString(lipgloss.NewStyle().Foreground(theme.Grid).Render("│"))
		}
	}
	return b.String()
}

func (a *patchesAdapter) renderRowGutter(theme tui.Theme) string {
	width := a.cols*a.cellWidth + (a.cols - 1)
	return lipgloss.NewStyle().Foreground(theme.Grid).Render(strings.Repeat("─", width))
}

func (a *patchesAdapter) renderCell(theme tui.Theme, r, c, idx, cursorIdx int, violCells map[int]bool) string {
	label := a.board.Cells[idx]
	clue, isClue := a.puzzle.Clues[idx]
	covered := label != -1

	digits := a.cellWidth - 1
	var text string
	switch {
	case isClue:
		text = fmt.Sprintf("%*d%s", digits, clue.Number, patchesShapeIcon(clue.Shape))
	case covered:
		text = strings.Repeat(patchesShapeIcon(a.labelShape(label)), a.cellWidth)
	default:
		text = strings.Repeat(" ", a.cellWidth)
	}

	st := lipgloss.NewStyle()
	switch {
	case covered:
		st = st.Background(theme.RegionColor(label)).Foreground(theme.OnAccent)
	default:
		st = st.Background(theme.Surface).Foreground(theme.Dim)
	}
	if isClue {
		st = st.Bold(true)
		if !covered {
			st = st.Foreground(theme.Piece)
		}
	}
	if violCells[idx] {
		st = st.Foreground(theme.Error).Bold(true)
	}
	if a.hasAnchor && !covered && a.box.contains(r, c) {
		st = st.Background(theme.Accent).Foreground(theme.OnAccent)
	}
	if idx == cursorIdx {
		st = st.Reverse(true)
	}
	return st.Render(text)
}

func (a *patchesAdapter) legend() string {
	secondary := "x: cancel/remove"
	if tui.EnhancedKeyboardActive() {
		secondary = "Shift+Space: cancel/remove"
	}
	return fmt.Sprintf(
		"wasd/arrows move/stretch  ·  Space: anchor/commit  ·  %s  ·  %s square  %s wide  %s tall  %s free",
		secondary,
		patchesShapeIcon(pz.Square), patchesShapeIcon(pz.Wide), patchesShapeIcon(pz.Tall), patchesShapeIcon(pz.Free),
	)
}

// Compile-time check that patchesAdapter satisfies tui.BoardAdapter.
var _ tui.BoardAdapter = (*patchesAdapter)(nil)
