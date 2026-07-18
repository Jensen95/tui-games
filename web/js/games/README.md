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
    },
    destroy() {
      // Undo everything: remove any event listeners you attached outside
      // of what api.bindPointer's returned unbind function already covers
      // (e.g. document-level drag listeners), clear timers/observers. The
      // shell empties `container`'s innerHTML itself after calling this,
      // so you don't need to remove your own DOM nodes.
    },
  };
}
```

Nothing else is required. No class, no default export -- just these two
named exports. `create` is called exactly once per puzzle instance; a "New
puzzle" or "Menu" action in the shell calls `destroy()` on the current
instance (if any) and, for a new puzzle, calls `create()` again from
scratch with a fresh `bundle`.

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

  // Keyboard-movement helper shared across games so "wasd + arrows (+
  // hjkl) move a cursor" behaves identically everywhere. Pass your
  // `handleKey`'s KeyboardEvent; returns `{dr, dc}` (a row/col delta, each
  // -1/0/1) if the event was a recognized movement key, or `null`
  // otherwise (i.e. this event wasn't a cursor move -- check it yourself
  // for your game's other keys). Does NOT call preventDefault or mutate
  // anything -- you own clamping the result to your grid's bounds and
  // re-rendering the cursor.
  cursorMove(event: KeyboardEvent): {dr: number, dc: number} | null,

  // Pointer-input helper implementing this project's required touch/mouse
  // scheme: a tap (or a plain left-click) fires onPrimary(); a long-press
  // (~500ms, via Pointer Events so it works for touch and mouse both), a
  // right-click (contextmenu, prevented from opening the native menu), OR
  // a shift-click all fire onSecondary(). Attach it once per interactive
  // element (typically once per cell button); it returns an `unbind()`
  // function -- call it from your `destroy()` (or earlier, if you rebuild
  // that element) to remove the listeners it added.
  //
  // This assumes your game's mouse/touch model is "click a single cell to
  // act on it". If your game's defining interaction is a drag (Zip's
  // click-drag path, Patches' click-drag rectangle), don't use this helper
  // for the drag surface itself -- wire up your own pointerdown/move/up
  // state machine, exactly like `03-tui-design.md`'s mouse section
  // describes, and clean up whatever listeners you add in `destroy()`.
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

- **Tango:** `api.bindPointer` per cell, but its `onPrimary`/`onSecondary`
  callbacks do *not* mirror Space/Shift+Space. Tap/click **cycles**
  `empty -> sun -> moon -> empty` (LinkedIn's mobile model) -- this is what
  makes "moon" reachable with a single tap, where the old
  tap=sun/shift-or-long-press=moon split left moon unreachable by touch.
  Long-press (~500ms)/right-click/shift-click (`onSecondary`) clears the
  cell outright instead of placing moon. The keyboard mapping (Space toggles
  sun, Shift+Space toggles moon, `m` fallback) is unchanged.
- **Queens:** also departs from a literal primary/secondary mirror. Tap
  cycles `empty -> X -> queen -> empty`; long-press/right-click/shift-click
  clears the cell (mark + queen) outright. Queens' *defining* mouse/touch
  interaction is a drag, though (see `03-tui-design.md`: "click-drag paints
  X marks, mirrors LinkedIn"), so it does **not** use `api.bindPointer` --
  like Zip/Patches below, it wires its own pointerdown/move/up state machine
  on the grid: pressing and dragging paints an X mark on every non-given,
  non-queen cell the pointer crosses (a tap that never leaves its starting
  cell falls through to the tap-cycle above instead of painting). The
  keyboard mapping (Space toggles X, Shift+Space toggles queen, `x`
  fallback) is unchanged.
- **Mini Sudoku:** primary action is inherently digit-based (there's no
  single "the" primary symbol to tap-cycle), so its cell tap only moves the
  cursor/selection -- render an on-screen `1..N` keypad (`N` read from the
  board JSON's `rows`/`cols`, never hardcoded, so a differently-sized clone
  keeps working) plus a Notes toggle (reflect its state visually, e.g. an
  `.active` class/`aria-pressed`) and an Erase button, so the game is fully
  playable with no physical keyboard, per the "must be fully playable on a
  phone" requirement. Keep the keypad visible at least whenever
  `matchMedia("(pointer: coarse)")` matches or the viewport is narrow; it's
  fine (and simplest) to leave it visible on desktop too.
- **Zip:** the defining interaction is a drag from the path's current end
  (see below), wired as a hand-rolled pointerdown/move/up state machine, not
  `api.bindPointer`. A tap is just the degenerate case of that same state
  machine (a pointerdown/up with no intervening move): tapping a cell
  orthogonally adjacent to the path's head extends the path to it; tapping
  a cell already on the path truncates the path back to (and including)
  that cell -- tapping the second-to-last cell is therefore how a tap
  retracts one step.
- **Patches:** the defining interaction is a drag from an uncovered cell to
  the opposite corner (see below), which always commits immediately on
  release and remains the primary gesture. As a touch-friendly alternative,
  a plain tap-and-release with **no** movement anchors a rectangle without
  committing it (its 1-cell preview stays on screen); a second, separate tap
  elsewhere names the opposite corner and commits, or re-tapping the same
  anchor cell confirms a 1x1 rectangle. Tapping a placed rectangle removes
  it -- this takes priority even while a two-tap rectangle is pending (the
  pending anchor is simply abandoned).

Any pointer state machine that isn't `api.bindPointer` (Queens/Zip/Patches
above) must still play nicely with touch: set CSS `touch-action: none` on
the drag surface **only** (not the whole page -- everywhere else must keep
scrolling normally), and prefer `Element.setPointerCapture(event.pointerId)`
on `pointerdown` so drags stay tracked reliably even once the finger drifts
off the element that started them.

## Never re-derive win/violations yourself

`api.violations`/`api.solved` are the *only* source of truth. Call
`violations` after every mutation to drive live red error styling on the
offending cells (each violation's `cells` array tells you exactly which
ones), and call `solved` after every mutation to detect a win -- the moment
it resolves `true`, call `api.onSolved()`. Do not assume "no violations"
means solved (an empty board has no violations either -- see api.md).
