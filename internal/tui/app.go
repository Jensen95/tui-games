package tui

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jensen95/tui-games/internal/engine"
)

// screen is the root model's state machine per 03-tui-design.md:
//
//	Menu -> Generating(spinner) -> Playing -> WinSummary
//	 ^                                            |
//	 '-------------------- back/new --------------'
type screen int

const (
	screenMenu screen = iota
	screenGenerating
	screenPlaying
	screenWinSummary
)

// Minimum terminal size the shell will render a screen at; below this it
// shows a friendly "make the terminal bigger" message instead
// (03-tui-design.md: "refuse to render... below a minimum size").
const (
	minWidth  = 60
	minHeight = 20
)

// Model is the root Bubble Tea model. It holds the active screen, the
// shared theme/keymap/help bar, and (once playing) the active game's view.
// Update dispatches by screen; View composes whichever screen is active.
// Each game's logic stays isolated behind BoardAdapter — this file never
// imports a game package.
type Model struct {
	theme Theme
	keys  KeyMap
	help  help.Model

	width, height int

	screen screen
	menu   menuModel
	spin   spinner.Model
	game   *gameView

	genErr   error // surfaced on the Menu screen if the last generation failed
	nextSeed int64
}

// NewModel builds the root model with theme as the active palette (see
// DefaultTheme for the selection order).
func NewModel(theme Theme) Model {
	return Model{
		theme:    theme,
		keys:     DefaultKeyMap(),
		help:     help.New(),
		spin:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		menu:     newMenuModel(),
		screen:   screenMenu,
		nextSeed: time.Now().UnixNano(),
	}
}

// Init starts the spinner ticking (harmless before it's shown; the
// Generating screen is the only one that renders it).
func (m Model) Init() tea.Cmd {
	return m.spin.Tick
}

// puzzleReadyMsg is delivered by generateCmd once a background Generate
// call finishes, per "Generation without blocking" in 03-tui-design.md.
type puzzleReadyMsg struct {
	entry engine.Entry
	diff  engine.Difficulty
	seed  int64
	gen   engine.Generated
	err   error
}

// generateCmd runs entry.Generate in a Bubble Tea Cmd (its own goroutine) so
// the UI never blocks, even for the slower generators.
func generateCmd(entry engine.Entry, diff engine.Difficulty, seed int64) tea.Cmd {
	return func() tea.Msg {
		gen, err := entry.Generate(diff, engine.NewRand(seed))
		return puzzleReadyMsg{entry: entry, diff: diff, seed: seed, gen: gen, err: err}
	}
}

// Update dispatches by message first (window size and generation results are
// screen-independent), then by the active screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		return m, nil
	case puzzleReadyMsg:
		return m.onPuzzleReady(msg)
	case tea.KeyboardEnhancementsMsg:
		// The terminal acked our keyboard-enhancement request (see View,
		// which sets v.KeyboardEnhancements — though v2.0.8 always requests
		// basic Kitty disambiguation regardless of that field's value; see
		// its source comment "always enable basic key disambiguation" in
		// cursed_renderer.go). SupportsKeyDisambiguation() is true whenever
		// this message arrives at all, since a terminal that didn't support
		// it wouldn't reply. This is exactly what makes Shift+Space /
		// Shift+digit distinguishable from their unshifted forms (IsShifted,
		// IsSpace) instead of legacy terminals' fallback-only behavior.
		enhanced := msg.SupportsKeyDisambiguation()
		setEnhancedKeyboardActive(enhanced)
		m.keys.SecondaryAction.SetEnabled(enhanced)
		return m, nil
	case tea.KeyPressMsg:
		// Quit is global: it must work from every screen, including mid-play.
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
	}

	switch m.screen {
	case screenMenu:
		return m.updateMenu(msg)
	case screenGenerating:
		return m.updateGenerating(msg)
	case screenPlaying, screenWinSummary:
		return m.updatePlaying(msg)
	default:
		return m, nil
	}
}

func (m Model) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch {
	case key.Matches(km, m.keys.Up):
		m.menu.moveCursor(-1)
	case key.Matches(km, m.keys.Down):
		m.menu.moveCursor(1)
	case key.Matches(km, m.keys.Left):
		m.menu.cycleDifficulty(-1)
	case key.Matches(km, m.keys.Right):
		m.menu.cycleDifficulty(1)
	case key.Matches(km, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
	case key.Matches(km, m.keys.PrimaryAction):
		if entry, ok := m.menu.selected(); ok && m.menu.playable() {
			seed := m.nextSeed
			m.nextSeed++
			diff := m.menu.diff
			m.genErr = nil
			m.screen = screenGenerating
			return m, generateCmd(entry, diff, seed)
		}
	}
	return m, nil
}

func (m Model) updateGenerating(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.Back) {
			m.screen = screenMenu
		}
	}
	return m, nil
}

func (m Model) onPuzzleReady(msg puzzleReadyMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.genErr = fmt.Errorf("generate %s: %w", msg.entry.ID, msg.err)
		m.screen = screenMenu
		return m, nil
	}
	gv, err := newGameView(msg.entry, msg.diff, msg.seed, msg.gen)
	if err != nil {
		m.genErr = err
		m.screen = screenMenu
		return m, nil
	}
	m.game = gv
	m.screen = screenPlaying
	return m, nil
}

func (m Model) updatePlaying(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.game == nil {
		m.screen = screenMenu
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Back):
			m.screen = screenMenu
			m.game = nil
			return m, nil
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		case key.Matches(msg, m.keys.New):
			entry, diff := m.game.entry, m.game.diff
			seed := m.nextSeed
			m.nextSeed++
			m.game = nil
			m.screen = screenGenerating
			return m, generateCmd(entry, diff, seed)
		case key.Matches(msg, m.keys.Undo):
			m.game.adapter.Undo()
		case key.Matches(msg, m.keys.Reset):
			m.game.adapter.Reset()
		case key.Matches(msg, m.keys.Hint):
			m.game.adapter.Hint()
		default:
			m.game.handleKey(msg)
		}
	case tea.MouseMsg:
		m.game.handleMouse(msg)
	}

	if m.game != nil && m.game.solved() {
		m.screen = screenWinSummary
	}
	return m, nil
}

// View composes the active screen and declares the terminal features this
// program needs (alt screen + mouse cell motion + keyboard enhancements),
// per v2's declarative View fields (00-overview.md: "Mouse mode is now a
// View field").
//
// Keyboard enhancements: charm.land/bubbletea/v2 v2.0.8 always requests
// basic Kitty keyboard protocol disambiguation regardless of this field
// (cursed_renderer.go's keyboardEnhancementsFlags: "flags := 1 // always
// enable basic key disambiguation") — that base request is what lets
// supporting terminals report Shift+Space / Shift+digit with a real Mod bit
// instead of collapsing them into plain Space or a shifted glyph (see
// keys.go: IsShifted/IsSpace, and EnhancedKeyboardActive). We additionally
// opt into ReportAlternateKeys so ShiftedCode is populated too, for any
// adapter that wants it later. Whether the terminal actually supports any
// of this arrives asynchronously as a tea.KeyboardEnhancementsMsg, handled
// in Update.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.KeyboardEnhancements = tea.KeyboardEnhancements{ReportAlternateKeys: true}
	return v
}

func (m Model) render() string {
	if m.width > 0 && (m.width < minWidth || m.height < minHeight) {
		return tooSmallMessage(m.theme, m.width, m.height)
	}

	switch m.screen {
	case screenMenu:
		body := m.menu.View(m.theme, m.width)
		if m.genErr != nil {
			errStyle := lipgloss.NewStyle().Foreground(m.theme.Error)
			body = lipgloss.JoinVertical(lipgloss.Left, body, "", errStyle.Render("generation failed: "+m.genErr.Error()))
		}
		return body
	case screenGenerating:
		accent := lipgloss.NewStyle().Foreground(m.theme.Accent)
		dim := lipgloss.NewStyle().Foreground(m.theme.Dim)
		return lipgloss.JoinVertical(lipgloss.Left,
			accent.Render(m.spin.View()+" generating puzzle..."),
			"",
			dim.Render("esc to cancel"))
	case screenPlaying, screenWinSummary:
		if m.game == nil {
			return ""
		}
		return m.game.View(m.theme, m.keys, m.help, m.width)
	default:
		return ""
	}
}

func tooSmallMessage(theme Theme, w, h int) string {
	msg := fmt.Sprintf("terminal too small (%dx%d)\nneed at least %dx%d — resize to keep playing", w, h, minWidth, minHeight)
	return lipgloss.NewStyle().Foreground(theme.Warning).Padding(1, 2).Render(msg)
}

// Run starts the interactive Bubble Tea program and blocks until it exits.
func Run() error {
	m := NewModel(DefaultTheme(""))
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
