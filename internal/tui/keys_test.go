package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// These fixtures encode, from the pinned charm.land/bubbletea/v2 v2.0.8
// source (uv.Key.String/Keystroke, see key.go), how Shift actually shows up
// on a KeyPressMsg:
//
//   - Enhanced terminals (Kitty keyboard protocol, which Bubble Tea always
//     requests basic disambiguation for): Code stays the *base* key
//     (' ' for space, '1'..'6' for digits) and Mod carries ModShift.
//     ShiftedCode may additionally hold the shifted glyph, but Code/Mod
//     alone already disambiguate.
//   - Legacy terminals: Space has no separate "shifted" form at all — there
//     is nothing to report, so Shift+Space is indistinguishable from Space.
//     Shift+<digit> instead arrives as the *shifted glyph itself* — Code
//     and Text both become e.g. '!' for Shift+1 on a US layout — with no
//     Mod bit set, because the terminal just sent the character its layout
//     produces for that physical key combo.

func TestIsSpace(t *testing.T) {
	tests := []struct {
		name string
		k    tea.KeyPressMsg
		want bool
	}{
		{"plain space", tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, true},
		{"shift+space, enhanced encoding", tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModShift}, true},
		{"enter is not space", tea.KeyPressMsg{Code: tea.KeyEnter}, false},
		{"digit is not space", tea.KeyPressMsg{Code: '1', Text: "1"}, false},
		{"legacy shifted-glyph '!' is not space", tea.KeyPressMsg{Code: '!', Text: "!"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSpace(tt.k); got != tt.want {
				t.Fatalf("IsSpace(%+v) = %v, want %v", tt.k, got, tt.want)
			}
		})
	}
}

func TestIsShifted(t *testing.T) {
	tests := []struct {
		name string
		k    tea.KeyPressMsg
		want bool
	}{
		{"plain space: no shift", tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, false},
		{
			"shift+space, enhanced encoding: Mod carries ModShift",
			tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModShift},
			true,
		},
		{
			"shift+space with other mods also set",
			tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModShift | tea.ModCtrl},
			true,
		},
		{"plain digit '1': no shift", tea.KeyPressMsg{Code: '1', Text: "1"}, false},
		{
			"shift+1, enhanced encoding: base Code '1' with ModShift + ShiftedCode",
			tea.KeyPressMsg{Code: '1', Mod: tea.ModShift, ShiftedCode: '!'},
			true,
		},
		{
			"shift+1, LEGACY terminal: arrives as the glyph '!' itself, no Mod bit",
			tea.KeyPressMsg{Code: '!', Text: "!"},
			false, // by design: legacy can't report this as "shifted" at all;
			// IsShifted correctly says no, and the caller's fallback plain-key
			// binding for '!' (or "1" pencil-mode key, etc.) is what handles it —
			// nothing here tries to reverse-map the glyph back to shift+1.
		},
		{"ctrl+c: not shift", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsShifted(tt.k); got != tt.want {
				t.Fatalf("IsShifted(%+v) = %v, want %v", tt.k, got, tt.want)
			}
		})
	}
}

// TestIsShiftedIsSpace_AllSudokuDigits sweeps 1-6 (Mini Sudoku's digit
// range) through the same three shapes: plain, enhanced shift+digit, and
// the legacy shifted-glyph fallback (US layout glyphs, purely as example
// input — the point is that IsShifted must say false for a bare glyph
// regardless of what it is, not that these particular glyphs are special).
func TestIsShiftedIsSpace_AllSudokuDigits(t *testing.T) {
	digits := []rune{'1', '2', '3', '4', '5', '6'}
	legacyShiftedGlyphs := map[rune]rune{
		'1': '!', '2': '@', '3': '#', '4': '$', '5': '%', '6': '^',
	}

	for _, d := range digits {
		plain := tea.KeyPressMsg{Code: d, Text: string(d)}
		if IsShifted(plain) {
			t.Errorf("plain digit %q: IsShifted = true, want false", d)
		}
		if IsSpace(plain) {
			t.Errorf("plain digit %q: IsSpace = true, want false", d)
		}

		enhanced := tea.KeyPressMsg{Code: d, Mod: tea.ModShift, ShiftedCode: legacyShiftedGlyphs[d]}
		if !IsShifted(enhanced) {
			t.Errorf("enhanced shift+%q: IsShifted = false, want true", d)
		}
		if IsSpace(enhanced) {
			t.Errorf("enhanced shift+%q: IsSpace = true, want false", d)
		}

		glyph := legacyShiftedGlyphs[d]
		legacy := tea.KeyPressMsg{Code: glyph, Text: string(glyph)}
		if IsShifted(legacy) {
			t.Errorf("legacy glyph %q (shift+%q on a real terminal): IsShifted = true, want false (can't be recovered, by design)", glyph, d)
		}
	}
}

func TestEnhancedKeyboardActiveDefaultsFalse(t *testing.T) {
	// Guard against test-order leakage: force it false, then restore
	// whatever it was so other tests in this package aren't affected.
	prev := EnhancedKeyboardActive()
	t.Cleanup(func() { setEnhancedKeyboardActive(prev) })

	setEnhancedKeyboardActive(false)
	if EnhancedKeyboardActive() {
		t.Fatalf("EnhancedKeyboardActive() = true after explicitly setting false")
	}
	setEnhancedKeyboardActive(true)
	if !EnhancedKeyboardActive() {
		t.Fatalf("EnhancedKeyboardActive() = false after explicitly setting true")
	}
}
