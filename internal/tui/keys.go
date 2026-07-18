package tui

import (
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
)

// enhancedKeyboard tracks whether the terminal acknowledged Bubble Tea's
// keyboard-enhancement request. Bubble Tea v2 always asks for basic Kitty
// keyboard protocol disambiguation (see the v2.0.8 source note in app.go);
// legacy terminals never reply, so this stays false until a
// tea.KeyboardEnhancementsMsg arrives. It's a package-level atomic rather
// than a Model field because board adapters (which only see a Theme and a
// tea.KeyPressMsg, not the root Model) need to read it too, e.g. to decide
// whether their help text should advertise "Shift+Space: ..." or fall back
// to a plain-key binding.
var enhancedKeyboard atomic.Bool

// EnhancedKeyboardActive reports whether the terminal has confirmed support
// for Bubble Tea's requested keyboard enhancements (Kitty keyboard
// protocol). When true, Shift+Space and Shift+<digit> arrive with a real
// Shift modifier (see IsShifted) instead of collapsing into plain Space or a
// layout-dependent shifted glyph. Board adapters call this — alongside
// IsShifted/IsSpace — to decide which half of the two-handed scheme's
// "Primary (Space) / Secondary (Shift+...)" column to show in their help
// text (03-tui-design.md, "The two-handed scheme").
func EnhancedKeyboardActive() bool { return enhancedKeyboard.Load() }

// setEnhancedKeyboardActive is called from Model.Update on
// tea.KeyboardEnhancementsMsg. It's unexported: only the root model updates
// this; adapters only ever read it via EnhancedKeyboardActive.
func setEnhancedKeyboardActive(v bool) { enhancedKeyboard.Store(v) }

// IsShifted reports whether k was pressed with the Shift modifier held.
//
// This only carries real information on terminals that acknowledged Bubble
// Tea's keyboard-enhancement request (see EnhancedKeyboardActive) — that's
// exactly what lets Shift+Space (the two-handed scheme's universal
// secondary-action modifier) and Shift+1..6 (Sudoku notes) be told apart
// from their unshifted counterparts. On a legacy terminal:
//   - Shift+Space is physically indistinguishable from Space (there's no
//     separate keycode for a shifted space bar), so IsShifted is always
//     false for it — there is no way to recover the intent, which is why
//     every game keeps a plain-key fallback for its secondary action.
//   - Shift+<digit> arrives as the shifted glyph for that key on the
//     current keyboard layout (e.g. "!" for US Shift+1), not as Mod+ModShift
//     on the base digit. Do not try to reverse-map the glyph back to a
//     digit+shift — it's layout-dependent and lossy; that's exactly why the
//     fallback column in 03-tui-design.md exists (Sudoku's plain "e" toggles
//     note-entry mode instead).
func IsShifted(k tea.KeyPressMsg) bool {
	return k.Mod.Contains(tea.ModShift)
}

// IsSpace reports whether k is the space key, independent of Shift. Pair it
// with IsShifted to detect the two-handed scheme's Secondary action
// (Shift+Space): IsSpace(k) && IsShifted(k).
func IsSpace(k tea.KeyPressMsg) bool {
	return k.Code == tea.KeySpace
}
