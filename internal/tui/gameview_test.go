package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

	"github.com/Jensen95/tui-games/internal/engine"
)

// TestGameView_BoardFrameOffset pins boardOriginX/Y to the actual frame
// composition: the adapter's view must start exactly at (boardOriginX,
// boardOriginY) inside gameView.View's output, or mouse hit-testing drifts
// off the grid.
func TestGameView_BoardFrameOffset(t *testing.T) {
	g := &gameView{
		entry:   engine.Entry{ID: "fixture", Name: "Fixture", Generate: fixtureGenerate, Verify: fixtureVerify},
		diff:    engine.Easy,
		adapter: &fixtureAdapter{},
	}
	out := g.View(Dark(), DefaultKeyMap(), help.New(), 80)
	lines := strings.Split(out, "\n")
	if len(lines) <= boardOriginY {
		t.Fatalf("frame has %d lines, expected board at line %d", len(lines), boardOriginY)
	}
	// fixtureAdapter.View returns "board"; it must appear at the pinned row,
	// flush against the pinned column.
	row := lines[boardOriginY]
	if idx := strings.Index(row, "board"); idx != boardOriginX {
		t.Errorf("board rendered at column %d of row %d, want %d (row: %q)", idx, boardOriginY, boardOriginX, row)
	}
	for i := 0; i < boardOriginY; i++ {
		if strings.Contains(lines[i], "board") {
			t.Errorf("board content leaked above its pinned row, at line %d", i)
		}
	}
}

// TestGameView_MouseTranslation verifies handleMouse shifts screen
// coordinates into adapter view space before hit-testing.
func TestGameView_MouseTranslation(t *testing.T) {
	a := &fixtureAdapter{}
	g := &gameView{adapter: a}
	// fixtureAdapter geometry: 1x1 grid at origin (0,0), cell 1x1, no gutters
	// (see app_test.go). A click at screen (0, boardOriginY) lands in the
	// adapter's only cell (0,0).
	g.handleMouse(tea.MouseClickMsg{X: boardOriginX, Y: boardOriginY})
	if a.lastCell.Row != 0 || a.lastCell.Col != 0 || !a.lastCell.Valid {
		t.Errorf("mouse at screen (%d,%d) resolved to %+v, want valid (0,0)", boardOriginX, boardOriginY, a.lastCell)
	}
	// A click on the header row must resolve off-grid.
	g.handleMouse(tea.MouseClickMsg{X: 0, Y: 0})
	if a.lastCell.Valid {
		t.Errorf("mouse on header row resolved on-grid: %+v", a.lastCell)
	}
}
