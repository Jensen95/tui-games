# 03 — TUI Design (Bubble Tea v2)

## App shape

A single Bubble Tea v2 program with a **screen state machine** as the root model. Screens:

```
Menu ──pick game+difficulty──▶ Generating(spinner) ──ready──▶ Playing ──win──▶ WinSummary
  ▲                                                              │                 │
  └───────────────────── back ◀─────────────────────────────────┴──── new/back ───┘
```

Root model holds `screen Screen`, the active `Game`, the current board state, a shared theme, and shared key bindings. `Update` dispatches by screen; `View` composes the active screen. This keeps each game's logic isolated in a **board adapter** while the shell (menu, help bar, resize handling, quit) is written once.

> v2 reminder: `Update` switches on `tea.KeyPressMsg` / mouse message types; `View()` returns a `tea.View`. Enable mouse reporting via the program options when constructing `tea.NewProgram`. Confirm exact v2 symbols against current docs (see `00-overview.md`).

## The board-adapter pattern

The shell is game-agnostic; each game supplies a small adapter implementing:

```go
type BoardAdapter interface {
    // Render the current board to a string (Lip Gloss styled), given theme + focus.
    View(theme Theme) string
    // Handle a key press; return whether the board changed.
    HandleKey(k tea.KeyPressMsg) (changed bool)
    // Handle a mouse event already resolved to a grid cell (or -1,-1 if outside).
    HandleMouse(ev MouseEvent, cell CellRef) (changed bool)
    // Live feedback + win detection delegate to the engine validator.
    Violations() []Violation
    Solved() bool
    // Layout metadata so the shell's mouse code can map screen coords -> cells.
    GridGeometry() Geometry // origin (x,y), cell width/height, rows, cols, gutters
    // Hints / reset / undo.
    Hint() ; Undo() ; Reset()
}
```

The shell owns the frame (title, timer, help line, borders); the adapter owns the grid. Adding a game = one adapter (~a few hundred lines), no shell changes.

## Navigation — keyboard

Shared bindings (via `bubbles/key`, shown in a `bubbles/help` bar):

| Key | Action |
|---|---|
| `↑↓←→` / `hjkl` | Move cursor |
| `Space` / `Enter` | Primary action (cycle symbol / place / commit rectangle / extend path) |
| game-specific | `s`/`m` (Tango), `1`–`6` (Sudoku), `x` (mark/clear), etc. |
| `u` | Undo |
| `Ctrl+r` | Reset puzzle |
| `H` | Hint (reveal one forced move + which rule/technique) |
| `n` | New puzzle (regenerate, same game/difficulty) |
| `?` | Toggle help |
| `Esc` | Back to menu |
| `Ctrl+c` / `q` | Quit |

Per-game key nuances are defined in each `games/*.md` under "TUI interaction."

## Navigation — mouse (first-class, required)

Bubble Tea v2 delivers mouse events as messages (click, motion, release, wheel), with coordinates. The shell converts a raw coordinate to a **`CellRef`** using the active adapter's `GridGeometry`, then calls `HandleMouse`. **Do the hit-testing manually** — for a grid it's just:

```
col = (mouseX - originX) / (cellWidth + colGutter)
row = (mouseY - originY) / (cellHeight + rowGutter)
if in bounds and not in a gutter -> CellRef{row,col} else Outside
```

This is robust, dependency-free, and easy to unit-test (see below). `bubblezone` is an option if you later want declarative zones for buttons/menus, but confirm its v2 compatibility first; for the game grids, manual math is cheaper and clearer.

Mouse interactions per game (these are why mouse support matters, not just a nicety):

- **Tango / Queens / Mini Sudoku:** click a cell = primary action (cycle symbol / cycle mark→queen / focus-then-type). Click-**drag** in Queens paints "X" marks across cells (mirrors LinkedIn). Right-click clears.
- **Zip:** **click-drag from `1`** to draw the path through cells; drag back to erase. This is the defining Zip interaction — prioritize smooth drag handling (motion events while a button is held).
- **Patches:** **click a clue and drag to the opposite corner** to define a rectangle; release commits; click a placed rectangle to remove. Also the defining interaction for that game.

Handle press → motion(while held) → release as a small per-adapter drag state machine. Wheel events can adjust difficulty on the menu or scroll help.

## Rendering & theming (Lip Gloss v2)

- A central `Theme` (in `tui/theme.go`) defines: base bg/fg, cursor highlight, given-cell style, error/violation style (red), success style, and **per-game palettes** (Queens needs N distinguishable region colors; Tango needs sun/moon colors; Patches needs several cosmetic rectangle fills). Concrete starting values for the three shipped themes (grey/dark/light) — token tables, region palette, and provenance — live in `08-theme-style-guide.md` (a provisional starting point, to be finalized once the TUI renders real boards).
- **Accessibility:** use adaptive colors (light/dark terminal) and make regions/rectangles distinguishable without relying on color alone — add a secondary channel (region border characters, rectangle glyph patterns, or a letter/index tag) so colorblind users and low-color terminals still work. This matters most for Queens.
- Compose grids with Lip Gloss borders/joins; draw inter-cell markers (Tango `=`/`×`, Zip walls) in the gutters between cells. Zip's path is a thick line through cell centers with corner-aware glyphs (`─ │ ┌ ┐ └ ┘`) or block shading.
- Keep a fixed, centered layout that adapts to `tea.WindowSizeMsg`; refuse to render (with a friendly "make the terminal bigger" message) below a minimum size.

## Live feedback & win

- After every change, the adapter calls `Violations()` and styles offending cells/lines/edges in the error style. This uses the *engine's* validator — same referee as the tests, so UI feedback and correctness can't drift.
- `Solved()` (also the engine's) triggers the win transition: freeze the board, show elapsed time and difficulty, and offer `n` (new) / `Esc` (menu).
- **Hints** call `LogicSolve`/forced-move logic to reveal exactly one next move and name the technique — genuinely educational and reuses the engine.

## Generation without blocking

Generation runs in a Bubble Tea `Cmd` (goroutine) that returns a `puzzleReadyMsg`; the shell shows a `bubbles/spinner` on the `Generating` screen meanwhile. For four of the games this is instant; for Zip at larger sizes it keeps the UI responsive.

## TUI testing hooks (see `04-testing-strategy.md`)

- Keep adapters' `HandleKey`/`HandleMouse` pure w.r.t. the model (no direct terminal writes) so they're unit-testable.
- Expose `GridGeometry` so mouse mapping is testable without a terminal.
- Use `teatest` for end-to-end flows (start → play a scripted solve → assert win banner) and golden files for stable View snapshots.
