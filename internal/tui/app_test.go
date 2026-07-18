package tui

import (
	"errors"
	"math/rand/v2"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/Jensen95/tui-games/internal/engine"
)

// --- key message fixtures -------------------------------------------------
//
// Bubble Tea v2 key messages match by KeyPressMsg.String() (see
// charm.land/bubbletea/v2's Key.Keystroke / uv.Key.String): printable keys
// return their Text, named keys fall back to a keystroke built from Code +
// Mod. These helpers build exactly the messages key.Matches expects, per
// the exact v2 symbols confirmed against the pinned v2.0.8 source (not
// guessed from v1 tutorials).

func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func keyNamed(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func keyCtrl(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
}

// --- menuModel unit tests (no engine registry involved) -------------------

func fixtureEntries() []engine.Entry {
	return []engine.Entry{
		{ID: "aaa", Name: "AAA", Generate: fixtureGenerate, Verify: fixtureVerify},
		{ID: "bbb", Name: "BBB", Generate: fixtureGenerate, Verify: fixtureVerify},
		{ID: "ccc", Name: "CCC", Generate: fixtureGenerate, Verify: fixtureVerify},
	}
}

func fixtureGenerate(diff engine.Difficulty, r *rand.Rand) (engine.Generated, error) {
	return engine.Generated{Puzzle: "puzzle", Solution: "solution", Encoded: []byte("{}")}, nil
}

func fixtureVerify(encoded []byte) error { return nil }

func TestMenuModelMoveCursorWraps(t *testing.T) {
	m := menuModel{entries: fixtureEntries()}

	m.moveCursor(-1)
	if m.cursor != 2 {
		t.Fatalf("moveCursor(-1) from 0 = %d, want 2 (wrap to last)", m.cursor)
	}
	m.moveCursor(1)
	if m.cursor != 0 {
		t.Fatalf("moveCursor(1) from 2 = %d, want 0 (wrap to first)", m.cursor)
	}
	m.moveCursor(1)
	if m.cursor != 1 {
		t.Fatalf("moveCursor(1) from 0 = %d, want 1", m.cursor)
	}
}

func TestMenuModelMoveCursorEmptyIsNoop(t *testing.T) {
	m := menuModel{}
	m.moveCursor(1)
	if m.cursor != 0 {
		t.Fatalf("moveCursor on empty menu changed cursor to %d", m.cursor)
	}
	if _, ok := m.selected(); ok {
		t.Fatalf("selected() on empty menu returned ok=true")
	}
	if m.playable() {
		t.Fatalf("playable() on empty menu returned true")
	}
}

func TestMenuModelCycleDifficultyWraps(t *testing.T) {
	m := menuModel{diff: engine.Expert}
	m.cycleDifficulty(1)
	if m.diff != engine.Easy {
		t.Fatalf("cycleDifficulty(1) past Expert = %v, want Easy", m.diff)
	}
	m.cycleDifficulty(-1)
	if m.diff != engine.Expert {
		t.Fatalf("cycleDifficulty(-1) from Easy = %v, want Expert", m.diff)
	}
}

func TestMenuModelPlayableReflectsRegistry(t *testing.T) {
	const fakeID = engine.GameID("menu-playable-fixture")
	Register(fakeID, func(engine.Generated) BoardAdapter { return &fixtureAdapter{} })

	m := menuModel{entries: []engine.Entry{
		{ID: fakeID, Name: "Fixture", Generate: fixtureGenerate, Verify: fixtureVerify},
		{ID: "no-adapter-fixture", Name: "No Adapter", Generate: fixtureGenerate, Verify: fixtureVerify},
	}}

	m.cursor = 0
	if !m.playable() {
		t.Fatalf("playable() = false for a game with a registered adapter")
	}
	m.cursor = 1
	if m.playable() {
		t.Fatalf("playable() = true for a game with no registered adapter")
	}
}

// --- fixtureAdapter: a minimal BoardAdapter for exercising screen transitions ---

type fixtureAdapter struct {
	solved     bool
	undoCalls  int
	resetCalls int
	hintCalls  int
	lastKey    tea.KeyPressMsg
	lastMouse  MouseEvent
	lastCell   CellRef
}

func (a *fixtureAdapter) View(Theme) string { return "board" }
func (a *fixtureAdapter) HandleKey(k tea.KeyPressMsg) bool {
	a.lastKey = k
	return true
}
func (a *fixtureAdapter) HandleMouse(ev MouseEvent, cell CellRef) bool {
	a.lastMouse, a.lastCell = ev, cell
	return true
}
func (a *fixtureAdapter) Violations() []engine.Violation { return nil }
func (a *fixtureAdapter) Solved() bool                   { return a.solved }
func (a *fixtureAdapter) GridGeometry() Geometry {
	return Geometry{CellWidth: 1, CellHeight: 1, Rows: 1, Cols: 1}
}
func (a *fixtureAdapter) Hint()  { a.hintCalls++ }
func (a *fixtureAdapter) Undo()  { a.undoCalls++ }
func (a *fixtureAdapter) Reset() { a.resetCalls++ }

// --- root Model screen-transition tests ------------------------------------

func newTestModel(entries []engine.Entry) Model {
	m := NewModel(Dark())
	m.menu = menuModel{entries: entries, diff: engine.Medium}
	return m
}

func TestUpdateMenuSelectUnregisteredIsNoop(t *testing.T) {
	m := newTestModel([]engine.Entry{
		{ID: "unregistered-fixture", Name: "No Adapter", Generate: fixtureGenerate, Verify: fixtureVerify},
	})

	next, cmd := m.Update(keyNamed(tea.KeyEnter))
	nm := next.(Model)
	if nm.screen != screenMenu {
		t.Fatalf("screen after selecting unregistered game = %v, want screenMenu", nm.screen)
	}
	if cmd != nil {
		t.Fatalf("expected no cmd selecting an unregistered game, got one")
	}
}

func TestScreenTransitionMenuToPlayingToWin(t *testing.T) {
	const fakeID = engine.GameID("transition-fixture")
	adapter := &fixtureAdapter{}
	Register(fakeID, func(engine.Generated) BoardAdapter { return adapter })

	entry := engine.Entry{ID: fakeID, Name: "Transition Fixture", Generate: fixtureGenerate, Verify: fixtureVerify}
	m := newTestModel([]engine.Entry{entry})

	// Menu -> Generating: Select on a playable entry issues a generate Cmd.
	next, cmd := m.Update(keyNamed(tea.KeyEnter))
	m = next.(Model)
	if m.screen != screenGenerating {
		t.Fatalf("screen after Select = %v, want screenGenerating", m.screen)
	}
	if cmd == nil {
		t.Fatalf("Select on a playable entry produced no Cmd")
	}

	// Generating -> Playing: run the Cmd, feed back the puzzleReadyMsg.
	msg := cmd()
	ready, ok := msg.(puzzleReadyMsg)
	if !ok {
		t.Fatalf("generate Cmd produced %T, want puzzleReadyMsg", msg)
	}
	next, _ = m.Update(ready)
	m = next.(Model)
	if m.screen != screenPlaying {
		t.Fatalf("screen after puzzleReadyMsg = %v, want screenPlaying", m.screen)
	}
	if m.game == nil {
		t.Fatalf("game is nil after entering screenPlaying")
	}

	// A key not claimed by the shared keymap reaches the adapter.
	next, _ = m.Update(keyRune('z'))
	m = next.(Model)
	if m.game.adapter.(*fixtureAdapter).lastKey.Code != 'z' {
		t.Fatalf("adapter did not receive the unclaimed key press")
	}

	// Undo/Reset/Hint route to the adapter, not the board's HandleKey.
	next, _ = m.Update(keyRune('u'))
	m = next.(Model)
	next, _ = m.Update(keyCtrl('r'))
	m = next.(Model)
	next, _ = m.Update(keyRune('H'))
	m = next.(Model)
	if adapter.undoCalls != 1 || adapter.resetCalls != 1 || adapter.hintCalls != 1 {
		t.Fatalf("undo/reset/hint calls = %d/%d/%d, want 1/1/1", adapter.undoCalls, adapter.resetCalls, adapter.hintCalls)
	}

	// Playing -> WinSummary: once the adapter reports Solved(), the next
	// Update flips the screen.
	adapter.solved = true
	next, _ = m.Update(keyRune('z'))
	m = next.(Model)
	if m.screen != screenWinSummary {
		t.Fatalf("screen after Solved() = %v, want screenWinSummary", m.screen)
	}

	// WinSummary -> Menu via Back (esc).
	next, _ = m.Update(keyNamed(tea.KeyEscape))
	m = next.(Model)
	if m.screen != screenMenu {
		t.Fatalf("screen after esc from WinSummary = %v, want screenMenu", m.screen)
	}
	if m.game != nil {
		t.Fatalf("game not cleared after returning to menu")
	}
}

func TestQuitIsGlobalAcrossScreens(t *testing.T) {
	for _, sc := range []screen{screenMenu, screenGenerating, screenPlaying, screenWinSummary} {
		m := NewModel(Dark())
		m.screen = sc
		_, cmd := m.Update(keyRune('q'))
		if cmd == nil {
			t.Fatalf("screen %v: q produced no Cmd", sc)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("screen %v: q's Cmd did not produce tea.QuitMsg", sc)
		}
	}
}

func TestWindowSizeMsgUpdatesDimensions(t *testing.T) {
	m := NewModel(Dark())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = next.(Model)
	if m.width != 100 || m.height != 40 {
		t.Fatalf("dimensions after WindowSizeMsg = %dx%d, want 100x40", m.width, m.height)
	}
}

func TestTooSmallMessageBelowMinimum(t *testing.T) {
	m := NewModel(Dark())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 10, Height: 5})
	m = next.(Model)
	view := m.View()
	if view.Content == "" {
		t.Fatalf("expected a non-empty too-small message")
	}
}

// TestKeyboardEnhancementsMsgEnablesSecondaryAction exercises the two-handed
// scheme's terminal-capability detection end to end at the Update level:
// SecondaryAction (Shift+Space) starts disabled (so it's hidden from help
// until proven real, per 03-tui-design.md's "show what's actually active"),
// and a tea.KeyboardEnhancementsMsg with a non-zero Flags — what v2.0.8
// actually sends when a terminal acks the request (see keyboard.go:
// SupportsKeyDisambiguation returns k.Flags > 0) — flips it on and updates
// the shared EnhancedKeyboardActive() flag adapters read.
func TestKeyboardEnhancementsMsgEnablesSecondaryAction(t *testing.T) {
	t.Cleanup(func() { setEnhancedKeyboardActive(false) })

	m := NewModel(Dark())
	if m.keys.SecondaryAction.Enabled() {
		t.Fatalf("SecondaryAction starts enabled, want disabled until the terminal confirms support")
	}
	if EnhancedKeyboardActive() {
		t.Fatalf("EnhancedKeyboardActive() = true before any KeyboardEnhancementsMsg")
	}

	next, cmd := m.Update(tea.KeyboardEnhancementsMsg{Flags: 1})
	m = next.(Model)
	if cmd != nil {
		t.Fatalf("KeyboardEnhancementsMsg handling produced an unexpected Cmd")
	}
	if !m.keys.SecondaryAction.Enabled() {
		t.Fatalf("SecondaryAction still disabled after a KeyboardEnhancementsMsg with Flags=1")
	}
	if !EnhancedKeyboardActive() {
		t.Fatalf("EnhancedKeyboardActive() = false after a KeyboardEnhancementsMsg with Flags=1")
	}

	// A zero-Flags message (defensive: v2.0.8 shouldn't send one, since it
	// only replies when the terminal supports at least basic disambiguation)
	// must not report support.
	next, _ = m.Update(tea.KeyboardEnhancementsMsg{Flags: 0})
	m = next.(Model)
	if m.keys.SecondaryAction.Enabled() {
		t.Fatalf("SecondaryAction enabled after a KeyboardEnhancementsMsg with Flags=0")
	}
	if EnhancedKeyboardActive() {
		t.Fatalf("EnhancedKeyboardActive() = true after a KeyboardEnhancementsMsg with Flags=0")
	}
}

// TestTwoHandedMovementBindingsMatchWASD checks that w/a/s/d move the cursor
// exactly like hjkl/arrows already do (03-tui-design.md's two-handed
// scheme), by exercising the same public surface real input would: the
// shared KeyMap via key.Matches.
func TestTwoHandedMovementBindingsMatchWASD(t *testing.T) {
	keys := DefaultKeyMap()

	tests := []struct {
		name    string
		k       tea.KeyPressMsg
		binding key.Binding
	}{
		{"w = up", keyRune('w'), keys.Up},
		{"a = left", keyRune('a'), keys.Left},
		{"s = down", keyRune('s'), keys.Down},
		{"d = right", keyRune('d'), keys.Right},
		{"up arrow still = up", keyNamed(tea.KeyUp), keys.Up},
		{"h still = left (vi keys unaffected)", keyRune('h'), keys.Left},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !key.Matches(tt.k, tt.binding) {
				t.Fatalf("key.Matches(%v, ...) = false, want true", tt.k)
			}
		})
	}

	// wasd/hjkl must never collide with any other shared binding
	// (03-tui-design.md: "wasd/hjkl letters are therefore reserved").
	reserved := []rune{'w', 'a', 's', 'd', 'h', 'j', 'k', 'l'}
	others := map[string]key.Binding{
		"Undo":            keys.Undo,
		"Reset":           keys.Reset,
		"Hint":            keys.Hint,
		"New":             keys.New,
		"Help":            keys.Help,
		"Back":            keys.Back,
		"Quit":            keys.Quit,
		"PrimaryAction":   keys.PrimaryAction,
		"SecondaryAction": keys.SecondaryAction,
	}
	for _, r := range reserved {
		for name, b := range others {
			if key.Matches(keyRune(r), b) {
				t.Fatalf("shared binding %s collides with reserved movement key %q", name, r)
			}
		}
	}
}

func TestPuzzleReadyErrorReturnsToMenu(t *testing.T) {
	m := NewModel(Dark())
	m.screen = screenGenerating
	next, _ := m.Update(puzzleReadyMsg{
		entry: engine.Entry{ID: "err-fixture", Name: "Err"},
		err:   errors.New("boom"),
	})
	m = next.(Model)
	if m.screen != screenMenu {
		t.Fatalf("screen after generate error = %v, want screenMenu", m.screen)
	}
	if m.genErr == nil {
		t.Fatalf("genErr not set after a failed generation")
	}
}
