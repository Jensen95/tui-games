# Game module interface

This is the contract every file in `web/js/games/*.js` implements. `web/js/app.js`
(the shell) is written generically against this contract — it dynamically
`import()`s `./games/<gameId>.js` for whichever game the player picks in the
menu, so a new game (e.g. `zip.js`, `patches.js`) can be dropped in here
**without touching the shell** as long as it follows this doc.

## Module shape

```js
export const id = "tango"; // must exactly match the gameId used by
                            // globalThis.ligEngine / web/js/api.md (and
                            // therefore the filename: games/<id>.js)

export function create(container, api, bundle) {
  // ... build DOM inside `container`, wire up input ...
  return {
    handleKey(event) {
      // Return true if this KeyboardEvent was consumed (and call
      // event.preventDefault() yourself when you consume it, e.g. to stop
      // Space/arrows from scrolling the page). Return false/undefined if
      // you didn't recognize the key -- the shell doesn't currently do
      // anything with a "not consumed" key itself, but returning an
      // accurate answer keeps the contract honest for future shell logic.
      // Every module also recognizes `Shift+H` here and routes it to the
      // same logic `hint()` below exposes -- see "Hints" below.
    },
    destroy() {
      // Undo everything: remove any event listeners you attached outside
      // of what api.bindPointer's returned unbind function already covers
      // (e.g. document-level drag listeners), clear timers/observers. The
      // shell empties `container`'s innerHTML itself after calling this,
      // so you don't need to remove your own DOM nodes.
    },
    async hint() {
      // Required (see "Hints" below): perform exactly one hint move via
      // api.hint()/api.onHint() and apply it to the board. Called by the
      // shell's shared Hint button; also call this same function yourself
      // from handleKey on `h`/`H` so the keyboard shortcut and the button
      // do exactly the same thing.
    },
  };
}
```

Nothing else is required beyond the three methods above. No class, no
default export -- just these named exports. `create` is called exactly once
per puzzle instance; a "New puzzle" or "Menu" action in the shell calls
`destroy()` on the current instance (if any) and, for a new puzzle, calls
`create()` again from scratch with a fresh `bundle`.

## `container`

A plain, empty `HTMLElement` (already in the page, already sized by the
shell's CSS) that belongs entirely to your module for the lifetime of this
instance. Render your board as a DOM grid (CSS Grid) inside it however you
like -- append whatever markup you want, add your own classes. Reuse
`web/css/style.css`'s existing CSS custom properties (`--bg`, `--surface`,
`--border`, `--text`, `--dim`, `--accent`, `--on-accent`, `--sun`, `--moon`,
`--success`, `--warning`, `--error`, `--grid`, `--radius`, `--font-mono`)
for visual consistency; `web/css/play.css` (linked by `play.html` alongside
`style.css`) has shared board/cell/keypad classes (`.cell`, `.given`,
`.cursor`, `.invalid`, `.puzzle-grid`, `.board-shell`, `.keypad`, ...) --
reuse what fits, add your own game-specific classes for anything it
doesn't cover (region colors, edge markers, box borders, etc.).

Never re-implement rule checking. The engine (via `api.violations`/
`api.solved`) is always the referee -- if you find yourself writing "is
this a duplicate in this row" or similar, stop, that belongs in
`internal/games/<game>`, not here.

## `api`

An object the shell builds fresh for this specific puzzle instance
(closed over its `gameId` and `puzzle` token), so you never touch
`globalThis.ligEngine` or `web/js/engine.js` directly:

```ts
api = {
  gameId: string,
  difficulty: "easy" | "medium" | "hard" | "expert",
  seed: number,

  // Calls globalThis.ligEngine.violations(gameId, puzzle, JSON.stringify(board))
  // under the hood (via web/js/engine.js) and returns the parsed array
  // documented in web/js/api.md. `board` is your in-memory board object --
  // pass the live object (or a structurally-identical plain object), it
  // gets JSON.stringify'd for you. On an engine-level error, this reports
  // the error to the shell (see onError below) and resolves to `[]` rather
  // than throwing, so you don't need a try/catch at every call site.
  violations(board: object): Promise<Array<{rule: string, message: string, cells: {row:number,col:number}[]}>>,

  // Same idea for globalThis.ligEngine.solved(...); resolves to `false` on
  // an engine-level error (also reported via onError).
  solved(board: object): Promise<boolean>,

  // Call this the moment your own `await api.solved(board)` check first
  // returns true. It's safe to call more than once (or from multiple
  // await chains that raced) -- the shell only reacts to the first call
  // (stops the timer, shows the win banner). You are NOT required to
  // freeze input yourself after calling this; the shell handles the win
  // UI, extra moves after a solved board are harmless.
  onSolved(): void,

  // Report a bridge-level error the shell should surface (a toast/banner)
  // that isn't already funneled through violations()/solved()'s automatic
  // reporting. You'll rarely need this directly.
  onError(err: Error): void,

  // Calls globalThis.ligEngine.hint(gameId, puzzle, JSON.stringify(board),
  // solution) under the hood and returns the parsed {done, message,
  // technique, cells, apply} object documented in web/js/api.md's hint()
  // section. Resolves to `null` on an engine-level error (reported via
  // onError, exactly like violations()/solved()). See "Hints" below for
  // how modules are expected to use this.
  hint(board: object): Promise<{done: boolean, message: string, technique: string, cells: {row:number,col:number}[], apply: object} | null>,

  // Call this with a hint's `message` (see api.hint above) every time your
  // module performs a hint move, so the shell's shared status line shows
  // it -- whether the hint was triggered by the shell's Hint button or by
  // your own `h`/`H` key handling.
  onHint(message: string): void,

  // Keyboard-movement helper shared across games so "wasd + arrows (+
  // hjkl) move a cursor" behaves identically everywhere. Pass your
  // `handleKey`'s KeyboardEvent; returns `{dr, dc}` (a row/col delta, each
  // -1/0/1) if the event was a recognized movement key, or `null`
  // otherwise (i.e. this event wasn't a cursor move -- check it yourself
  // for your game's other keys). Does NOT call preventDefault or mutate
  // anything -- you own clamping the result to your grid's bounds and
  // re-rendering the cursor.
  cursorMove(event: KeyboardEvent): {dr: number, dc: number} | null,

  // Resolves ANY point (viewport/client coordinates, e.g. a PointerEvent's
  // clientX/clientY) to the nearest {row, col} inside a `rows` x `cols` CSS
  // grid element, via getBoundingClientRect + floor division -- never DOM
  // hit-testing (elementFromPoint/event.target). A point outside the
  // element's bounds clamps to the nearest edge cell rather than resolving
  // to nothing. This is the required way for a board's pointer handling to
  // resolve a touch/click/drag point to a cell: DOM hit-testing
  // (elementFromPoint, event.target.closest(...)) can silently miss --
  // between cells wherever a game renders a real gap (Tango's edge-marker
  // gutter tracks), and, more subtly, on the sub-pixel seams `1fr` grid
  // tracks can leave between cells on some viewport widths/zoom levels --
  // either way a tap that lands there hits nothing and is dropped. Wire
  // this into a single pointerdown/pointermove listener on your *grid
  // container* (not per cell); see "Pointer/touch: dead zones" below.
  cellAt(gridEl: Element, rows: number, cols: number, clientX: number, clientY: number): {row: number, col: number},

  // Pointer-input helper for games with NO grid-cell dead-zone risk at all
  // (i.e. nothing rendered on the board *outside* the grid.js/keypad
  // elements this attaches to -- e.g. Mini Sudoku's on-screen keypad
  // buttons). Implements this project's required touch/mouse scheme: a tap
  // (or a plain left-click) fires onPrimary(); a long-press (~500ms, via
  // Pointer Events so it works for touch and mouse both), a right-click
  // (contextmenu, prevented from opening the native menu), OR a
  // shift-click all fire onSecondary(). Attach it once per interactive
  // element; it returns an `unbind()` function -- call it from your
  // `destroy()` (or earlier, if you rebuild that element) to remove the
  // listeners it added.
  //
  // Do NOT use this for board *cells* -- see "Pointer/touch: dead zones"
  // below for why every game's board now resolves pointer input through
  // `cellAt` on a single grid-level listener instead of a per-cell
  // listener. If your game's defining interaction is a drag (Zip's
  // click-drag path, Patches' click-drag rectangle, Queens' drag-to-paint),
  // that grid-level state machine handles taps too (a drag that never
  // moves) -- there is no separate "just a tap" case to also wire up with
  // this helper.
  bindPointer(el: Element, handlers: {onPrimary: () => void, onSecondary: () => void}): () => void,
};
```

## `bundle`

The specific puzzle instance to render, straight from `web/js/engine.js`'s
`generate(gameId, difficulty, seed)` plus the params that produced it:

```ts
bundle = {
  puzzle: <opaque>,          // do not read fields from this -- see api.md
  solution: <game-specific>, // documented per-game in api.md; only useful
                              // if you build a hint/reveal feature later
  board: <game-specific>,    // the INITIAL board JSON from generate() --
                              // givens/clues pre-filled, everything else
                              // empty. This is your seed board: mutate it
                              // in place as the player plays (see below).
  difficulty: string,
  seed: number,
  gameId: string,
};
```

**Board ownership:** `bundle.board` is the one piece of mutable state your
module owns for its whole lifetime. Mutate it directly in response to
input (set/clear a cell, flip a queen, push a path step, etc. -- per the
"Move semantics" section of your game in `web/js/api.md`), then call
`await api.violations(board)` and `await api.solved(board)` with that same
object to re-check and re-render. Never mutate `givens`/`regions`/`clues`/
etc. (the immutable fields) -- they're only there for you to render from,
the bridge ignores them on the way back in anyway (see api.md's "Shared
conventions").

UI-only state that has **no engine representation** (Queens' X marks,
Sudoku's pencil notes) does not belong in `board` at all -- keep it in your
module's own local variables/closures, and never write it into the fields
`violations`/`solved` read (`cells` for Queens/Sudoku/Tango). See the
per-game sections of `web/js/api.md` for exactly which UI concepts are
engine-invisible.

## The two-handed key scheme (from `docs/plan/docs/03-tui-design.md`)

`wasd` / arrow keys / `hjkl` move a per-game keyboard cursor (use
`api.cursorMove`). `Space` is primary, `Shift+Space` is secondary. Browsers
report `event.shiftKey` reliably, so implement the real scheme first (no
detection fallback needed) -- but per the shell's ground rules, also keep
the legacy single-key fallbacks alive for muscle-memory parity:

| Game | Primary | Secondary | Fallback keys |
|---|---|---|---|
| Tango | `Space` = place sun (again = clear) | `Shift+Space` = place moon | `m` = moon |
| Queens | `Space` = place X (again = clear) | `Shift+Space` = place queen | `x` = X |
| Mini Sudoku | `1`-`6` = place digit (again = clear) | `Shift+1`-`6` = toggle pencil note | `e` = toggle note-entry mode; `0`/`Backspace` = clear |

Mouse/touch: the pointer (mouse *and* touch) interaction is **not** always a
literal mirror of the keyboard's primary/secondary split -- a touch user has
no Shift key and no reliable right-click, so a couple of games depart from
the keyboard mapping on purpose to keep every action reachable by tap alone:

- **Tango:** its board pointer handling (see "Pointer/touch: dead zones"
  below) does *not* mirror Space/Shift+Space. Tap/click **cycles**
  `empty -> sun -> moon -> empty` (LinkedIn's mobile model) -- this is what
  makes "moon" reachable with a single tap, where the old
  tap=sun/shift-or-long-press=moon split left moon unreachable by touch.
  Long-press (~500ms)/right-click/shift-click clears the cell outright
  instead of placing moon. The keyboard mapping (Space toggles sun,
  Shift+Space toggles moon, `m` fallback) is unchanged.
- **Queens:** also departs from a literal primary/secondary mirror. Tap
  cycles `empty -> X -> queen -> empty`; long-press/right-click/shift-click
  clears the cell (mark + queen) outright. Queens' *defining* mouse/touch
  interaction is a drag, though (see `03-tui-design.md`: "click-drag paints
  X marks, mirrors LinkedIn"), so it wires its own pointerdown/move/up state
  machine on the grid (see "Pointer/touch: dead zones" below): pressing and
  dragging paints an X mark on every non-given, non-queen cell the pointer
  crosses (a tap that never leaves its starting cell falls through to the
  tap-cycle above instead of painting). The keyboard mapping (Space toggles
  X, Shift+Space toggles queen, `x` fallback) is unchanged.
- **Mini Sudoku:** primary action is inherently digit-based (there's no
  single "the" primary symbol to tap-cycle), so its cell tap only moves the
  cursor/selection -- render an on-screen `1..N` keypad (`N` read from the
  board JSON's `rows`/`cols`, never hardcoded, so a differently-sized clone
  keeps working) plus a Notes toggle (reflect its state visually, e.g. an
  `.active` class/`aria-pressed`) and an Erase button, so the game is fully
  playable with no physical keyboard, per the "must be fully playable on a
  phone" requirement. Keep the keypad visible at least whenever
  `matchMedia("(pointer: coarse)")` matches or the viewport is narrow; it's
  fine (and simplest) to leave it visible on desktop too. The keypad/tools
  buttons themselves are exactly the "no dead-zone risk" case `api.bindPointer`
  is still for (see below) -- only the board's own cell tap-to-move-cursor
  goes through the grid-level `api.cellAt` handling.
- **Zip:** the defining interaction is a drag from the path's current end
  (see below), wired as a hand-rolled pointerdown/move/up state machine. A
  tap is just the degenerate case of that same state machine (a
  pointerdown/up with no intervening move): tapping a cell orthogonally
  adjacent to the path's head extends the path to it; tapping a cell already
  on the path truncates the path back to (and including) that cell --
  tapping the second-to-last cell is therefore how a tap retracts one step.
- **Patches:** the defining interaction is a drag from an uncovered cell to
  the opposite corner (see below), which always commits immediately on
  release and remains the primary gesture. As a touch-friendly alternative,
  a plain tap-and-release with **no** movement anchors a rectangle without
  committing it (its 1-cell preview stays on screen); a second, separate tap
  elsewhere names the opposite corner and commits, or re-tapping the same
  anchor cell confirms a 1x1 rectangle. Tapping a placed rectangle removes
  it -- this takes priority even while a two-tap rectangle is pending (the
  pending anchor is simply abandoned).

Every pointer state machine above (Tango/Queens/Zip/Patches on the board;
Mini Sudoku's cursor-move tap) must still play nicely with touch: set CSS
`touch-action: none` on the drag/tap surface **only** (not the whole page --
everywhere else must keep scrolling normally), and prefer
`Element.setPointerCapture(event.pointerId)` on `pointerdown` so drags stay
tracked reliably even once the finger drifts off the element that started
them.

### Pointer/touch: dead zones

**Every game's board resolves pointer input through a single
pointerdown/pointermove listener on the *grid container*, using
`api.cellAt(gridEl, rows, cols, clientX, clientY)` to turn the raw point into
a `{row, col}` -- never a listener attached per cell button, and never DOM
hit-testing (`elementFromPoint`, `event.target.closest(...)`).** This was a
real, reported bug (Queens on a phone: "a bit dinky with tapping between"):
`api.cellAt`'s doc comment above explains exactly why DOM-hit-testing-based
resolution can drop a point on the floor -- a per-cell listener can only ever
react to points landing squarely on *its own* element, so it inherits every
gap between elements for free (a real rendered gutter, like Tango's
edge-marker tracks; or an invisible one, like the sub-pixel seams `1fr` grid
tracks can round to between cells at some viewport widths). Grid-level
`cellAt`-based resolution has no such gap: every point in (or even just
outside) the grid's bounds maps to *some* cell, full stop.

Concretely: Tango and Mini Sudoku's board cells (formerly wired with
`api.bindPointer` per cell button) and Queens/Zip/Patches' drag surfaces (already
grid-level, formerly resolving hits via `elementFromPoint`) all go through
`api.cellAt` now. `api.bindPointer` itself is unchanged and still correct for
interactive elements that carry no such risk (nothing else is ever rendered
between them) -- Mini Sudoku's on-screen keypad/tools buttons are exactly
that case.

## Hints

Every module implements `hint()` (see "Module shape" above) mirroring the
TUI's `H` key: it reveals exactly one forced move toward the puzzle's
recorded solution. The heavy lifting -- deciding *which* move, and naming a
technique where one applies -- is entirely `globalThis.ligEngine.hint()`'s
job (via `api.hint`, see web/js/api.md's `hint()` section); a module's
`hint()` just:

1. Calls `const result = await api.hint(board);` and bails if it's `null`
   (an engine-level error -- `api.hint` already reported it via `onError`).
2. If `result.done`, calls `api.onHint(result.message)` and stops -- there's
   nothing to apply.
3. Otherwise, applies `result.apply` to `board` per its game-specific shape
   (see api.md's `hint()` section: a `{cells: [...]}` write-list for
   Tango/Queens/Mini Sudoku, `{path: [...]}` for Zip, `{r0,c0,r1,c1}` for
   Patches), exactly the same way any other move mutates `board` --
   including any UI-only bookkeeping a normal move would also need (Mini
   Sudoku: clear that cell's pencil notes; Queens: nothing extra, X marks
   are untouched by hints; Patches: fully clear any rectangle(s) currently
   overlapping the revealed box before writing the new label, mirroring
   `internal/tui/boards/patches.go`'s `applyHintRect`).
4. Moves the keyboard cursor to (the first of) `result.cells`.
5. Re-renders, adds a `hint-pulse` class (see `web/css/play.css`) to every
   cell in `result.cells` and removes it again after ~900ms (a `setTimeout`
   -- no need to track/cancel it in `destroy()`, a stale timeout clearing a
   class off a detached DOM node is harmless).
6. Calls `await api.violations(board)` / `await api.solved(board)` exactly
   like any other move (a hint can complete the puzzle).
7. Calls `api.onHint(result.message)`.

Wire **`Shift+H`** in `handleKey` (checked *before* calling `api.cursorMove`)
to call the exact same internal function your `hint()` returns -- the shared
toolbar's Hint button and the keyboard shortcut must behave identically.
Plain `h` (no Shift) is deliberately left alone: it's the `hjkl` vim-motion
fallback for "move left", and `api.cursorMove` lowercases before its lookup
-- so it matches *both* `h` and `H` as a cursor move and would swallow the
hint shortcut first if checked afterward or checked as bare `h`/`H` without
requiring `event.shiftKey`. `hint()` itself takes no arguments and its
return value is not read by the shell (status display already happened via
`api.onHint` in step 7) -- returning the raw engine result is harmless and
convenient for the module's own `handleKey` to reuse, but not required.

## Never re-derive win/violations yourself

`api.violations`/`api.solved` are the *only* source of truth. Call
`violations` after every mutation to drive live red error styling on the
offending cells (each violation's `cells` array tells you exactly which
ones), and call `solved` after every mutation to detect a win -- the moment
it resolves `true`, call `api.onSolved()`. Do not assume "no violations"
means solved (an empty board has no violations either -- see api.md).
