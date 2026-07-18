package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jensen95/tui-games/internal/engine"
)

// menuModel is the Menu screen: pick a game + difficulty from the frozen
// engine registry (engine.All()). Games without a registered board adapter
// are listed but cannot be entered — they show "engine ready — adapter
// coming soon" so the shell stays honest about what's playable while other
// agents' adapters land independently.
type menuModel struct {
	entries []engine.Entry
	cursor  int
	diff    engine.Difficulty
}

func newMenuModel() menuModel {
	return menuModel{
		entries: engine.All(),
		diff:    engine.Medium,
	}
}

// selected returns the currently highlighted entry and whether the menu has
// any entries at all (it's empty until at least one game track lands).
func (m menuModel) selected() (engine.Entry, bool) {
	if len(m.entries) == 0 {
		return engine.Entry{}, false
	}
	return m.entries[m.cursor], true
}

// playable reports whether the currently highlighted entry has a registered
// board adapter and can be entered.
func (m menuModel) playable() bool {
	e, ok := m.selected()
	if !ok {
		return false
	}
	_, ok = Lookup(e.ID)
	return ok
}

func (m *menuModel) moveCursor(delta int) {
	if len(m.entries) == 0 {
		return
	}
	n := len(m.entries)
	m.cursor = ((m.cursor+delta)%n + n) % n
}

func (m *menuModel) cycleDifficulty(delta int) {
	const n = int(engine.Expert) + 1
	m.diff = engine.Difficulty(((int(m.diff)+delta)%n + n) % n)
}

// View renders the game picker.
func (m menuModel) View(theme Theme, width int) string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("lig — pick a game")

	if len(m.entries) == 0 {
		empty := lipgloss.NewStyle().Foreground(theme.Dim).Render(
			"No games registered yet. Board adapters land as each game track finishes.")
		return lipgloss.JoinVertical(lipgloss.Left, title, "", empty)
	}

	var rows []string
	for i, e := range m.entries {
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(theme.Text)
		if i == m.cursor {
			cursor = "> "
			style = style.Foreground(theme.Accent).Bold(true)
		}

		_, ok := Lookup(e.ID)
		status := lipgloss.NewStyle().Foreground(theme.Success).Render("ready")
		if !ok {
			status = lipgloss.NewStyle().Foreground(theme.Dim).Italic(true).Render("engine ready — adapter coming soon")
		}

		row := fmt.Sprintf("%s%-14s %s", cursor, e.Name, status)
		rows = append(rows, style.Render(row))
	}

	diffLine := lipgloss.NewStyle().Foreground(theme.Dim).Render(
		fmt.Sprintf("difficulty: < %s >  (←/→ to change)", m.diff))

	var help string
	if m.playable() {
		help = lipgloss.NewStyle().Foreground(theme.Dim).Render("enter/space to play  ·  ?/q help/quit")
	} else {
		help = lipgloss.NewStyle().Foreground(theme.Dim).Render("(adapter not ready — pick another game)  ·  ?/q help/quit")
	}

	body := strings.Join(rows, "\n")
	return lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", diffLine, "", help)
}
