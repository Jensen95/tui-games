package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jensen95/tui-games/internal/engine"
)

// gameView hosts one active BoardAdapter: it routes keys/mouse the shared
// keymap doesn't claim, tracks elapsed time, and renders the frame (title,
// timer, violations, help) around whatever the adapter draws for its grid.
// The shell (app.go) owns screen transitions (Playing -> WinSummary -> Menu);
// gameView only reports whether the adapter says the puzzle is solved.
type gameView struct {
	entry   engine.Entry
	diff    engine.Difficulty
	adapter BoardAdapter
	seed    int64

	startTime time.Time
	solvedAt  time.Time // zero until Solved() first true
}

func newGameView(entry engine.Entry, diff engine.Difficulty, seed int64, gen engine.Generated) (*gameView, error) {
	factory, ok := Lookup(entry.ID)
	if !ok {
		return nil, fmt.Errorf("tui: no board adapter registered for %q", entry.ID)
	}
	return &gameView{
		entry:     entry,
		diff:      diff,
		seed:      seed,
		adapter:   factory(gen),
		startTime: time.Now(),
	}, nil
}

// solved reports whether the adapter's win condition has fired, and latches
// solvedAt the first time it does so the WinSummary timer freezes.
func (g *gameView) solved() bool {
	ok := g.adapter.Solved()
	if ok && g.solvedAt.IsZero() {
		g.solvedAt = time.Now()
	}
	return ok
}

func (g *gameView) elapsed() time.Duration {
	if !g.solvedAt.IsZero() {
		return g.solvedAt.Sub(g.startTime)
	}
	return time.Since(g.startTime)
}

// handleKey routes a key press not claimed by the shared keymap to the
// adapter. It reports whether the board changed.
func (g *gameView) handleKey(msg tea.KeyPressMsg) bool {
	return g.adapter.HandleKey(msg)
}

// mouseEventFromMsg classifies a tea mouse message into the adapter-facing
// MouseEventType + button/mod, independent of screen coordinates.
func mouseEventFromMsg(msg tea.MouseMsg) MouseEvent {
	m := msg.Mouse()
	ev := MouseEvent{Button: m.Button, Mod: m.Mod}
	switch msg.(type) {
	case tea.MouseClickMsg:
		ev.Type = MouseEventPress
	case tea.MouseMotionMsg:
		ev.Type = MouseEventMotion
	case tea.MouseReleaseMsg:
		ev.Type = MouseEventRelease
	case tea.MouseWheelMsg:
		ev.Type = MouseEventWheel
	}
	return ev
}

// boardOriginX/Y locate the adapter's View() output inside the frame that
// gameView.View composes: the board is rendered flush-left after the
// title/timer header line and one blank line. Adapter GridGeometry origins
// are relative to the adapter's own view, so mouse coordinates must be
// shifted by this frame offset before hit-testing. Keep in sync with View()
// (pinned by TestGameView_BoardFrameOffset).
const (
	boardOriginX = 0
	boardOriginY = 2
)

// handleMouse resolves a raw mouse message's coordinates to a grid cell via
// the adapter's GridGeometry (translating from screen space into the
// adapter's view space first), then dispatches. It reports whether the board
// changed.
func (g *gameView) handleMouse(msg tea.MouseMsg) bool {
	m := msg.Mouse()
	geo := g.adapter.GridGeometry()
	cell := CellRefFromPoint(geo, m.X-boardOriginX, m.Y-boardOriginY)
	return g.adapter.HandleMouse(mouseEventFromMsg(msg), cell)
}

// formatDuration renders a duration as M:SS (or H:MM:SS past an hour).
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// View renders the frame (title/timer, board, violations, help) around the
// adapter's own grid rendering.
func (g *gameView) View(theme Theme, keys KeyMap, helpModel help.Model, width int) string {
	titleStyle := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(theme.Dim)

	title := titleStyle.Render(fmt.Sprintf("%s · %s", g.entry.Name, g.diff))
	timer := dimStyle.Render(formatDuration(g.elapsed()))
	header := lipgloss.JoinHorizontal(lipgloss.Top, title, "  ", timer)

	board := g.adapter.View(theme)

	var sections []string
	sections = append(sections, header, "", board)

	if violations := g.adapter.Violations(); len(violations) > 0 {
		errStyle := lipgloss.NewStyle().Foreground(theme.Error)
		var lines []string
		for _, v := range violations {
			lines = append(lines, errStyle.Render("! "+v.Message))
		}
		sections = append(sections, "", strings.Join(lines, "\n"))
	}

	if g.solved() {
		winStyle := lipgloss.NewStyle().Foreground(theme.Success).Bold(true)
		sections = append(sections, "",
			winStyle.Render(fmt.Sprintf("Solved in %s! (n) new puzzle   (esc) menu", formatDuration(g.elapsed()))))
	}

	sections = append(sections, "", helpModel.View(keys))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}
