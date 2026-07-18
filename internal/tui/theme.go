// Package tui is the Bubble Tea v2 shell: screen state machine, shared
// keymap, theming, mouse hit-testing, and the board-adapter registry that
// per-game packages plug into. Nothing here is imported by internal/engine
// or internal/games/* — the dependency arrow points one way (01-architecture.md).
package tui

import (
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// Theme is the central palette every board adapter renders through. Concrete
// token values come from docs/plan/docs/08-theme-style-guide.md; per-game
// adapters read only from Theme so a palette tweak never touches game logic.
//
// v2 API note: unlike v1's lipgloss.Color type, charm.land/lipgloss/v2's
// lipgloss.Color is a function (func(string) color.Color) that constructs a
// stdlib image/color.Color; that's the type Style.Foreground/Background
// (and everything else color-typed) actually take, so Theme's fields are
// color.Color, populated via lipgloss.Color("#hex").
type Theme struct {
	// Name is the theme's identifier, e.g. "grey", "dark", "light".
	Name string

	Bg, Surface, Grid, Border   color.Color
	Text, Dim, Accent, OnAccent color.Color
	Piece, Elim                 color.Color
	Success, Warning, Error     color.Color

	// Regions is the categorical palette for Queens regions / Patches
	// rectangle fills (index 1-6 in the style guide; here 0-indexed).
	// Callers cycle with RegionColor for boards larger than len(Regions).
	Regions []color.Color

	// Sun, Moon are Tango's two symbol colors.
	Sun, Moon color.Color
}

// RegionColor returns the categorical fill for region index idx, cycling
// through the palette for boards with more regions than swatches.
func (t Theme) RegionColor(idx int) color.Color {
	if len(t.Regions) == 0 {
		return t.Accent
	}
	if idx < 0 {
		idx = -idx
	}
	return t.Regions[idx%len(t.Regions)]
}

// RegionLabel returns a stable, colorblind-safe letter tag for a region
// index: A, B, C, ... Z, AA, AB, .... This is the secondary, non-color
// channel mandated by 08-theme-style-guide.md ("never rely on color alone")
// — adapters overlay it (or a heavier border glyph) on region fills so
// Queens stays legible under NO_COLOR, 16-color downsampling, or colorblind
// vision.
func RegionLabel(idx int) string {
	if idx < 0 {
		idx = -idx
	}
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if idx < len(letters) {
		return string(letters[idx])
	}
	// Two-letter fallback for boards with >26 regions (never expected in
	// practice, but keep it total rather than panicking).
	first := idx/len(letters) - 1
	second := idx % len(letters)
	return string(letters[first]) + string(letters[second])
}

// Grey is the low-chroma neutral theme (Nostromo neutrals + Sulaco steel
// accent). It is the most fragile on 16-color terminals, which is why region
// borders (the non-color channel) are mandatory rather than optional.
func Grey() Theme {
	return Theme{
		Name:     "grey",
		Bg:       lipgloss.Color("#26282D"),
		Surface:  lipgloss.Color("#2E3137"),
		Grid:     lipgloss.Color("#3B3E46"),
		Border:   lipgloss.Color("#4A4E57"),
		Text:     lipgloss.Color("#E6E8EC"),
		Dim:      lipgloss.Color("#9096A0"),
		Accent:   lipgloss.Color("#86A6C8"),
		OnAccent: lipgloss.Color("#111318"),
		Piece:    lipgloss.Color("#F1F3F6"),
		Elim:     lipgloss.Color("#5B616B"),
		Success:  lipgloss.Color("#6FBF73"),
		Warning:  lipgloss.Color("#E0A45C"),
		Error:    lipgloss.Color("#E5736B"),
		Regions: []color.Color{
			lipgloss.Color("#4A463F"),
			lipgloss.Color("#3D4650"),
			lipgloss.Color("#3F4A45"),
			lipgloss.Color("#3E414E"),
			lipgloss.Color("#4A4340"),
			lipgloss.Color("#47414C"),
		},
		Sun:  lipgloss.Color("#C9A15E"),
		Moon: lipgloss.Color("#7E8CA8"),
	}
}

// Dark is the iamjarl dark mode: near-black surfaces, a lime accent.
func Dark() Theme {
	return Theme{
		Name:     "dark",
		Bg:       lipgloss.Color("#0A0A0B"),
		Surface:  lipgloss.Color("#15161A"),
		Grid:     lipgloss.Color("#2A2C33"),
		Border:   lipgloss.Color("#33363F"),
		Text:     lipgloss.Color("#F2F3F5"),
		Dim:      lipgloss.Color("#9AA0AB"),
		Accent:   lipgloss.Color("#D0FF00"),
		OnAccent: lipgloss.Color("#000000"),
		Piece:    lipgloss.Color("#FFFFFF"),
		Elim:     lipgloss.Color("#565A63"),
		Success:  lipgloss.Color("#4CAF50"),
		Warning:  lipgloss.Color("#FF6B35"),
		Error:    lipgloss.Color("#FF453A"),
		Regions: []color.Color{
			lipgloss.Color("#8A6D2F"),
			lipgloss.Color("#2F6E86"),
			lipgloss.Color("#2C7A63"),
			lipgloss.Color("#2E5C86"),
			lipgloss.Color("#8A4A2E"),
			lipgloss.Color("#7A4E68"),
		},
		Sun:  lipgloss.Color("#FFB454"),
		Moon: lipgloss.Color("#7AA2F7"),
	}
}

// Light is the iamjarl light mode: white surfaces, a purple accent.
func Light() Theme {
	return Theme{
		Name:     "light",
		Bg:       lipgloss.Color("#FFFFFF"),
		Surface:  lipgloss.Color("#F5F5F7"),
		Grid:     lipgloss.Color("#DCDCE1"),
		Border:   lipgloss.Color("#C9C9CE"),
		Text:     lipgloss.Color("#111111"),
		Dim:      lipgloss.Color("#6B6B72"),
		Accent:   lipgloss.Color("#A435D2"),
		OnAccent: lipgloss.Color("#FFFFFF"),
		Piece:    lipgloss.Color("#111111"),
		Elim:     lipgloss.Color("#B3B3BA"),
		Success:  lipgloss.Color("#2E7D32"),
		Warning:  lipgloss.Color("#C2410C"),
		Error:    lipgloss.Color("#D70015"),
		Regions: []color.Color{
			lipgloss.Color("#FBE3B8"),
			lipgloss.Color("#CDE7F5"),
			lipgloss.Color("#C7E9DA"),
			lipgloss.Color("#C9D8EC"),
			lipgloss.Color("#F3D2C0"),
			lipgloss.Color("#EBD3E2"),
		},
		Sun:  lipgloss.Color("#F5A623"),
		Moon: lipgloss.Color("#5B5BD6"),
	}
}

// Themes returns all three shipped themes in a stable order, e.g. for a
// theme picker.
func Themes() []Theme { return []Theme{Grey(), Dark(), Light()} }

// ThemeByName looks up a shipped theme by name (case-insensitive).
func ThemeByName(name string) (Theme, bool) {
	for _, t := range Themes() {
		if strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return Theme{}, false
}

// DefaultTheme picks a theme per the selection order in
// 08-theme-style-guide.md: explicit --theme flag (flagTheme, empty if
// unset) > $LIG_THEME env > default (Dark). Terminal light/dark background
// detection is intentionally not wired in yet — Bubble Tea v2 reports it via
// tea.BackgroundColorMsg at runtime, which arrives after the first Theme is
// already needed for Init/View; a future pass can re-select on that message.
func DefaultTheme(flagTheme string) Theme {
	if flagTheme != "" {
		if t, ok := ThemeByName(flagTheme); ok {
			return t
		}
	}
	if env := os.Getenv("LIG_THEME"); env != "" {
		if t, ok := ThemeByName(env); ok {
			return t
		}
	}
	return Dark()
}
