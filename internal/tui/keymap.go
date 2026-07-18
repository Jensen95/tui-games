package tui

import "charm.land/bubbles/v2/key"

// KeyMap holds the bindings shared by the whole shell, per the table in
// 03-tui-design.md and its "two-handed scheme (WASD + Space/Shift)"
// subsection. Per-game keys (m for Tango moon, 1-6 for Sudoku, x for
// mark, etc.) are not here — they're handled inside each BoardAdapter's
// HandleKey and never claimed by the shell.
//
// wasd/hjkl are reserved for movement across the whole shell: no other
// shared binding may use those letters, and per-game fallback keys must
// avoid them too (03-tui-design.md calls this out explicitly — Tango's
// moon fallback is 'm', not 's', for exactly this reason).
type KeyMap struct {
	Up    key.Binding // up / w / k
	Down  key.Binding // down / s / j
	Left  key.Binding // left / a / h
	Right key.Binding // right / d / l

	// PrimaryAction is the two-handed scheme's right-hand primary action:
	// Space (or Enter to accommodate the menu / non-two-handed play).
	PrimaryAction key.Binding
	// SecondaryAction is Shift+Space, the universal secondary-action
	// modifier across all five games (Shift+digit in Sudoku is the same
	// idea but game-specific, so it isn't a shared binding). It only
	// carries real information on terminals that ack Bubble Tea's keyboard
	// enhancement request — see EnhancedKeyboardActive — so it starts
	// disabled and the shell enables it once a KeyboardEnhancementsMsg
	// confirms support. Disabled bindings are omitted from the help view
	// automatically, which is how the help bar "shows what's actually
	// active" per the design doc.
	SecondaryAction key.Binding

	Undo  key.Binding
	Reset key.Binding // ctrl+r
	Hint  key.Binding // H
	New   key.Binding // n: regenerate, same game/difficulty

	Help key.Binding // ?
	Back key.Binding // esc: back to menu
	Quit key.Binding // q / ctrl+c
}

// DefaultKeyMap returns the shell's shared bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "w", "k"),
			key.WithHelp("↑/w/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "s", "j"),
			key.WithHelp("↓/s/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "a", "h"),
			key.WithHelp("←/a/h", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "d", "l"),
			key.WithHelp("→/d/l", "right"),
		),
		PrimaryAction: key.NewBinding(
			key.WithKeys("space", "enter"),
			key.WithHelp("space/enter", "primary action"),
		),
		SecondaryAction: key.NewBinding(
			key.WithKeys("shift+space"),
			key.WithHelp("shift+space", "secondary action"),
			key.WithDisabled(), // enabled once the terminal acks keyboard enhancements
		),
		Undo: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "undo"),
		),
		Reset: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "reset"),
		),
		Hint: key.NewBinding(
			key.WithKeys("H"),
			key.WithHelp("H", "hint"),
		),
		New: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new puzzle"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// ShortHelp implements help.KeyMap for the compact single-line help bar.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.PrimaryAction, k.SecondaryAction, k.Undo, k.Hint, k.Help, k.Back, k.Quit}
}

// FullHelp implements help.KeyMap for the expanded, multi-column help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.PrimaryAction, k.SecondaryAction},
		{k.Undo, k.Reset},
		{k.Hint, k.New},
		{k.Help, k.Back, k.Quit},
	}
}
