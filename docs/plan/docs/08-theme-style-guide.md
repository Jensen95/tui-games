# 08 — Theme Style Guide (starting point)

> **Status: starting point, not final.** These tokens are a first pass distilled from two
> existing design systems (below), good enough to build the TUI against. **Lock them only
> after the TUI renders real boards in a real terminal** — colors read very differently
> there than in a browser swatch, and the region palette in particular needs eyes-on tuning
> at actual cell size and against 256/16-color downsampling. Until then, treat every value
> here as a default to adjust, not a spec to defend. The finalization checklist is at the end.

Three themes ship: **grey**, **dark**, **light**. They plug into the central `Theme`
(`internal/tui/theme.go`) described in `03-tui-design.md`; nothing in the engine ever sees a
color.

## Sources & provenance

- **[nostromo-ui](https://github.com/JarlLyng/nostromo-ui)** — React/Tailwind component library, 4 Alien-themed token sets (Nostromo/Mother/Sulaco/LV-426), each with light+dark. We borrow its **pure-grey neutral ramp** and **Sulaco steel-blue** for the grey theme, and its token *architecture* (brand/neutral/state ramps → semantic tokens).
- **[iamjarl-design](https://github.com/JarlLyng/iamjarl-design)** — a token source-of-truth (Swift/CSS/TS), two modes with a bold identity. We borrow its **light** (purple `#A435D2` on white) and **dark** (lime `#D0FF00` on black) modes wholesale, plus its rules: state colors green/orange/red, no pure black/white except as deliberate tokens, always pair a colored surface with its on-color, radius/AA discipline.

Mapping: **grey ← Nostromo neutrals + Sulaco steel · dark ← iamjarl dark · light ← iamjarl light.**

## Semantic tokens

HSL/hex as reviewed. Accent is currently **mode-tuned** (see Open decisions).

| Token | Role | Grey | Dark | Light |
|---|---|---|---|---|
| `bg` | app background | `#26282D` | `#0A0A0B` | `#FFFFFF` |
| `surface` | panels, help bar, cards | `#2E3137` | `#15161A` | `#F5F5F7` |
| `grid` | inter-cell grid lines | `#3B3E46` | `#2A2C33` | `#DCDCE1` |
| `border` | outer frame / box borders | `#4A4E57` | `#33363F` | `#C9C9CE` |
| `text` | primary foreground | `#E6E8EC` | `#F2F3F5` | `#111111` |
| `dim` | secondary / help text | `#9096A0` | `#9AA0AB` | `#6B6B72` |
| `accent` | cursor, selection, focus, title | `#86A6C8` | `#D0FF00` | `#A435D2` |
| `onAccent` | text on an accent fill | `#111318` | `#000000` | `#FFFFFF` |
| `queen`/`piece` | placed-piece glyph | `#F1F3F6` | `#FFFFFF` | `#111111` |
| `elim` | eliminated/marked cell (`·`) | `#5B616B` | `#565A63` | `#B3B3BA` |
| `success` | solved / valid | `#6FBF73` | `#4CAF50` | `#2E7D32` |
| `warning` | soft constraint hint | `#E0A45C` | `#FF6B35` | `#C2410C` |
| `error` | violation (red) | `#E5736B` | `#FF453A` | `#D70015` |

Notes: the `success`/`warning`/`error` values are tuned to sit on `bg` as **text/glyph** colors
(the iamjarl "state text" idea), not as fills. If we ever fill a cell with a state color, pair
it with an on-color and re-check contrast.

## Categorical palette (regions, rectangles, pieces)

The hardest part: several fills that stay distinct **and** readable behind a glyph. Built on an
**Okabe–Ito colorblind-safe** base, re-toned per theme (pale tints on light, muted mid-tones on
dark, low-chroma greys separated mainly by lightness on grey).

**Queens regions / Patches rectangle fills** (index 1–6; extend for larger boards by rotating hue):

| # | Grey | Dark | Light |
|---|---|---|---|
| 1 | `#4A463F` | `#8A6D2F` | `#FBE3B8` |
| 2 | `#3D4650` | `#2F6E86` | `#CDE7F5` |
| 3 | `#3F4A45` | `#2C7A63` | `#C7E9DA` |
| 4 | `#3E414E` | `#2E5C86` | `#C9D8EC` |
| 5 | `#4A4340` | `#8A4A2E` | `#F3D2C0` |
| 6 | `#47414C` | `#7A4E68` | `#EBD3E2` |

**Tango sun/moon** — *not yet reviewed on screen; starting suggestions, expect to tune*:

| | Grey | Dark | Light |
|---|---|---|---|
| sun ☀ | `#C9A15E` | `#FFB454` | `#F5A623` |
| moon ☾ | `#7E8CA8` | `#7AA2F7` | `#5B5BD6` |

Patches treats fills as cosmetic, so it reuses the region palette above. Queens must draw
**region borders** (heavier glyphs between differing regions) on top of the fills — this is the
non-color channel that makes the grey theme's low-chroma regions usable and keeps everything
legible for colorblind users; see Accessibility below.

## Mapping to `Theme` (Go sketch)

Aligns with the `Theme` in `03-tui-design.md`. Values are `lipgloss.Color` (v2); Lip Gloss owns
the downsampling to the terminal's color profile.

```go
// internal/tui/theme.go
type Theme struct {
    Bg, Surface, Grid, Border      lipgloss.Color
    Text, Dim, Accent, OnAccent    lipgloss.Color
    Piece, Elim                    lipgloss.Color
    Success, Warning, Error        lipgloss.Color
    Regions                        []lipgloss.Color // categorical; len >= max board size
    Sun, Moon                      lipgloss.Color   // Tango
}

func Grey() Theme  { /* table above */ }
func Dark() Theme  { /* table above */ }
func Light() Theme { /* table above */ }

// Selection order (starting point): explicit --theme flag > $LIG_THEME env >
// detected terminal light/dark background > default = Dark.
```

Per-game adapters read only from `Theme` (e.g. `theme.Regions[regionID % len(theme.Regions)]`,
`theme.Sun`), so a token change is one edit, no game logic touched.

## Terminal rendering notes

- **Assume truecolor (24-bit).** Provide the hex above; Lip Gloss v2 auto-detects
  `COLORTERM`/profile and **downsamples** to 256- or 16-color automatically. Don't hand-pick
  256-color indices — let the library approximate, then spot-check.
- **Grey is the fragile one on 16 colors:** low-chroma regions collapse toward the same ANSI
  slot. That's exactly why region borders (non-color channel) are mandatory, not optional.
- **`NO_COLOR` / dumb terminals:** fall back to the secondary channel entirely — region-index
  letters/borders, `·`/`✕` marks, piece glyphs — so the game is still playable with zero color.
- Radius/shadows from the source systems don't translate to a grid of cells; ignore them. The
  terminal equivalents are box-drawing borders and the gutter markers already in
  `03-tui-design.md` (Tango `=`/`×`, Zip walls, Zip path glyphs `─ │ ┌ ┐ └ ┘`).

## Accessibility (carried from `03-tui-design.md`)

Never rely on color alone. Every color-coded distinction needs a second channel:
- **Queens regions:** heavier region-boundary glyphs **and** an optional per-region index letter.
- **Tango:** distinct `☀`/`☾` glyphs already carry the state; color is reinforcement.
- **Patches:** rectangle fills are cosmetic — the numbered clue + shape type carry the meaning.
- Target WCAG-style contrast for text-on-bg; verify `dim` on `bg` and `text` on each region fill.

## Open decisions

1. **Accent: mode-tuned vs. constant.** Current tokens flip the accent per theme
   (grey steel / dark lime / light purple), following iamjarl's mode-flip DNA. The alternative
   is **one constant purple accent** across all three for a tighter single identity. Undecided —
   revisit at finalization. If we go constant, only the `accent`/`onAccent` rows change.
2. **Whether grey's regions need slightly more chroma** to survive 16-color terminals, at the
   cost of some of the "calm grey" feel. Decide with a real 16-color test.

## How to finalize (do this after the TUI renders real boards)

1. Run each game in a real terminal (truecolor, then a forced 256- and 16-color run, then
   `NO_COLOR`) and screenshot all three themes.
2. Check region distinguishability at real cell size — especially grey — and adjust the
   categorical palette until 6+ regions are unmistakable with borders on.
3. Verify `text`/`dim` contrast on every surface and every region fill; nudge failing values.
4. Settle the two open decisions above.
5. Only then freeze the tables here and delete this "starting point" banner.
