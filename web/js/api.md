# `globalThis.ligEngine` — WASM engine bridge API

This is the contract two UI agents build against. It is produced by
`web/wasm/*.go` (built with `GOOS=js GOARCH=wasm go build -o lig.wasm
./web/wasm`) and loaded into the page alongside `wasm_exec.js`. It exposes
**one** global object, `globalThis.ligEngine`, with five synchronous
functions. Every function takes and returns **JSON-encoded strings** (never
raw JS objects) unless noted otherwise. `JSON.parse`/`JSON.stringify` at the
call site.

No exposed function ever panics or throws across the JS boundary: every
call recovers internally and returns `{"error": "<message>"}` (a JSON
string) instead. Always check for an `error` key before reading any other
field of a response.

## Loading / readiness

The wasm program does real work in `main()` (registering the five
functions) and then blocks forever (`select {}`) so the Go runtime stays
alive to service calls. Because instantiation is asynchronous, **do not**
call `ligEngine.*` until it signals readiness:

```html
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch("lig.wasm"), go.importObject)
    .then((result) => go.run(result.instance));

  globalThis.onLigEngineReady = () => {
    // globalThis.ligEngine is now safe to call.
    const games = JSON.parse(globalThis.ligEngine.games());
  };
</script>
```

The bridge sets `globalThis.ligEngineReady = true` and, if
`globalThis.onLigEngineReady` is already defined as a function at that
point, calls it. If your loader script runs after the wasm module (unlikely
but possible), poll `globalThis.ligEngineReady` instead.

## Functions

### `games()`

Returns every registered game, sorted by id.

```
games() -> string  // JSON: [{"id": string, "name": string}, ...]
```

```json
[
  {"id":"minisudoku","name":"Mini Sudoku"},
  {"id":"patches","name":"Patches"},
  {"id":"queens","name":"Queens"},
  {"id":"tango","name":"Tango"},
  {"id":"zip","name":"Zip"}
]
```

### `generate(gameId, difficulty, seed)`

```
generate(gameId: string, difficulty: string, seed: number) -> string
  // JSON: {"puzzle": <opaque>, "solution": <game-specific>, "board": <game-specific>}
  // or:   {"error": string}
```

- `gameId` — one of the ids from `games()`.
- `difficulty` — one of `"easy"`, `"medium"`, `"hard"`, `"expert"`
  (case-insensitive).
- `seed` — a JS number. The same `(gameId, difficulty, seed)` triple always
  produces the exact same puzzle (verified: `generate("tango","easy",7)`
  called twice yields byte-identical output) — it is fed straight into
  `engine.NewRand(seed)`.

The response has three top-level fields:

- **`puzzle`** — the game's own opaque clue encoding (exactly what that
  game's `Encode` function produces — never the solution). Treat it as a
  token: decode it only by passing it back into `violations`/`solved`
  unchanged. Its shape is internal and NOT part of this contract; UIs
  should never read fields out of it directly, since it's the same wire
  format the CLI's `lig generate`/`lig verify` commands use and may grow
  fields.
- **`solution`** — a game-specific JSON value, documented per game below.
  Not shown to the player during play; used for hints/reveals and for
  testing "does the board equal the solution" if you want an extra check
  beyond `solved()`.
- **`board`** — the game-specific **initial board JSON** the player starts
  from: every given/clue cell is pre-filled and locked, everything else is
  empty. This is the format documented in detail below, and the format
  `violations`/`solved` expect back.

### `violations(gameId, puzzleJSON, boardJSON)`

```
violations(gameId: string, puzzleJSON: string, boardJSON: string) -> string
  // JSON: [{"rule": string, "message": string, "cells": [{"row":int,"col":int}, ...]}, ...]
  // or:   {"error": string}
```

`puzzleJSON` is the `puzzle` string from `generate()` (pass it through
unchanged — `JSON.stringify` it if you parsed it, or keep the raw string
around). `boardJSON` is the current, possibly partial, board state in the
per-game shape documented below (`JSON.stringify` your in-memory board
object).

This is **partial-board aware**: it reports only rules that are *already*
broken by cells the player has filled in. An unfilled cell is never flagged,
and an incomplete-but-not-yet-invalid board yields `[]`. Call this after
every move to drive live error styling. `rule` is a stable,
machine-checkable identifier (e.g. `"three-in-a-row"`); the per-game
sections below list every rule string each game can emit. `cells` lists the
offending cells (may be `[]` for a whole-board rule with no natural anchor
cell, e.g. Patches' `"exact-cover"` "some cells uncovered" case).

### `solved(gameId, puzzleJSON, boardJSON)`

```
solved(gameId: string, puzzleJSON: string, boardJSON: string) -> string
  // JSON: {"solved": bool}
  // or:   {"error": string}
```

Same inputs as `violations`. Reports whether the board is a **complete**,
fully valid solution (this is the engine's own `Validator.Solved`, not a
"same as the recorded solution" check — a puzzle can have exactly one valid
solution, so in practice they coincide, but `solved()` is the authoritative
win check; never re-derive it from `violations()` being empty, since an
empty board also has no violations).

### `hint(gameId, puzzleJSON, boardJSON, solutionJSON)`

```
hint(gameId: string, puzzleJSON: string, boardJSON: string, solutionJSON: string) -> string
  // JSON: {
  //   "done": bool,
  //   "message": string,
  //   "technique": string,
  //   "cells": [{"row":int,"col":int}, ...],
  //   "apply": <game-specific, see below>
  // }
  // or: {"error": string}
```

Same `puzzleJSON`/`boardJSON` as `violations`/`solved`. `solutionJSON` is the
`solution` value `generate()` returned for this puzzle (`JSON.stringify` it
if you parsed it, or keep the raw string around — exactly like `puzzleJSON`).
This reveals **exactly one** forced move toward that recorded solution, the
same move each game's TUI `H` hint key reveals (see
`internal/tui/boards/*.go`'s `Hint()` methods) — it never re-derives a move
from `violations`/`solved` itself, and it never invents a move `solved()`
wouldn't accept.

- **`done`** — `true` when there is nothing left to hint (the board is
  already a complete solution, or — Zip/Patches/Mini Sudoku only, see
  below — no solution was recorded to hint from). When `done` is `true`,
  `cells` is `[]` and `apply` is absent; don't try to mutate anything.
  `message` still explains why (e.g. `"already solved"`).
- **`message`** — always a short, human-readable line describing the move,
  safe to show verbatim in a status line (e.g.
  `"hint: r2c4 = 3 (hidden single)"`, `"hint: queen at r3c5"`,
  `"hint: extend path to r1c1"`, `"hint: rectangle r0c1..r0c2"`). Row/column
  numbers in `message` are **1-indexed** for human display — everywhere else
  in this bridge (including this same response's `cells`/`apply`) stays
  0-indexed, per this doc's shared conventions.
- **`technique`** — the deepest logic technique the move needed, when the
  game can name one. Only Mini Sudoku currently populates this (one of
  `"given"`, `"naked-single"`, `"hidden-single"`, `"naked-pair"`,
  `"hidden-pair"`, `"pointing-pair"`, or the fallback `"solution"` when even
  the no-guessing solver stalled and the cell had to be revealed directly —
  see `internal/games/minisudoku/logicsolve.go`). Every other game always
  returns `""` — the key is always present, never omitted, so callers don't
  need an existence check, only an emptiness one.
- **`cells`** — every cell the move touches, for highlighting (e.g. Queens'
  hint may list both the cleared old queen cell and the newly placed one).
- **`apply`** — the mutation the UI should perform, **before** calling
  `violations`/`solved` again. Its shape is per-game, exactly like every
  other board JSON in this doc:
  - **Tango, Queens, Mini Sudoku**: `{"cells": [{"row":r,"col":c,"value":v}, ...]}`
    — a short list of absolute cell writes into `board.cells`. Queens'
    "move the queen" is a clear (`value: 0`) of the wrong cell (only if one
    is present and not a given) followed by a set (`value: 1`) of the
    correct one (only if it's not a given); Tango/Mini Sudoku are always a
    single write. Apply every entry in order; each is a plain
    `board.cells[row][col] = value` assignment (Queens: 0/1, Tango: 0/1/2,
    Mini Sudoku: 0-6).
  - **Zip**: `{"path": [{"row":r,"col":c}, ...]}` — the full replacement
    path (`board.path` always round-trips as a whole array, never
    incrementally, per this doc's Zip section). Replace `board.path`'s
    contents with this array.
  - **Patches**: `{"r0":int,"c0":int,"r1":int,"c1":int}` — one rectangle's
    bounding box (inclusive on both ends) to reveal. Before writing it,
    clear every cell of any rectangle(s) currently overlapping this box **in
    full** (reset their whole label to `-1`, not just the overlapping
    cells — mirrors `internal/tui/boards/patches.go`'s `applyHintRect`),
    then write a single fresh label across every cell in the box.

Worked example (Mini Sudoku, one empty cell left, solvable by a hidden
single):

```json
{
  "done": false,
  "message": "hint: r6c6 = 4 (hidden-single)",
  "technique": "hidden-single",
  "cells": [{"row":5,"col":5}],
  "apply": {"cells": [{"row":5,"col":5,"value":4}]}
}
```

Worked example (any game, nothing left to hint):

```json
{"done": true, "message": "already solved", "cells": [], "apply": null}
```

### Errors

Every function returns `{"error": "<message>"}` (and nothing else) when:

- `gameId` is not a registered game id,
- `difficulty` is not one of the four valid names,
- `puzzleJSON`/`boardJSON`/`solutionJSON` fails to parse or fails a
  shape/bounds check (wrong dimensions, out-of-range indices, malformed
  JSON), or
- anything else internally goes wrong (including a recovered panic).

Examples (captured from a real build):

```
generate("nope", "easy", 1)
  -> {"error":"unknown game \"nope\""}

generate("tango", "impossible", 1)
  -> {"error":"unknown difficulty \"impossible\" (want one of easy, medium, hard, expert)"}

violations("tango", "{not json", "{}")
  -> {"error":"tango: decode puzzle: tango: decode: invalid character 'n' looking for beginning of object key string"}

violations("tango", "<valid 6x6 puzzle>", "{\"cells\":[[0,0]]}")
  -> {"error":"tango: board has 1 rows, want 6"}
```

Never rely on the exact wording of an error message — only on the presence
of the `error` key.

## Shared conventions

- All grids are **row-major, zero-indexed**: `cells[row][col]`, `row` grows
  down, `col` grows right — identical to the engine's own
  `engine.Cell{Row, Col}`.
- A cell reference in JSON is always `{"row": int, "col": int}`.
- Every board format below distinguishes **immutable clue data** (echoed
  from the puzzle, needed for rendering, but the WASM bridge *never trusts
  it back* from an incoming `boardJSON` — it always re-derives it from the
  decoded `puzzleJSON`) from **mutable player state** (the only part the
  bridge actually reads back out of `boardJSON`). This means you can freely
  mutate and resend the exact object `generate()` gave you — the extra
  fields are ignored, not re-validated, on the way in.
- Two-handed move scheme: per `docs/plan/docs/03-tui-design.md`, every game
  has a **primary** action (`Space`/click) and a **secondary** action
  (`Shift+Space`/shift-click or drag). The per-game sections below map both
  to the exact board-JSON mutation the UI should perform before calling
  `violations`/`solved` again. The bridge has no notion of "primary" vs
  "secondary" itself — it only ever sees the resulting board state — so
  this mapping is UI-side convention, documented here so both UI agents
  implement the same behavior.

---

## Tango

6×6 grid, every cell holds Sun or Moon, subject to row/column balance,
no-three-in-a-row, and optional `=`/`×` edge constraints between
orthogonally adjacent cells. Grid size is always fixed at 6×6.

### Board JSON

```ts
{
  rows: 6, cols: 6,
  cells:  number[6][6],   // 0 = empty, 1 = sun, 2 = moon
  givens: boolean[6][6],  // true = pre-filled clue cell (immutable, locked)
  hEdges: number[6][5],   // hEdges[r][c]: relation between (r,c)-(r,c+1)
  vEdges: number[5][6],   // vEdges[r][c]: relation between (r,c)-(r+1,c)
}
```

`hEdges`/`vEdges` values: `0` = no constraint, `1` = `=` (equal — both
cells must end up the same symbol), `2` = `×` (cross — both cells must end
up different symbols). Edges are immutable puzzle data (never move); they
are included in the board JSON purely so the UI can render the `=`/`×`
gutter glyphs without separately decoding the opaque `puzzle` string. When
`violations`/`solved` decode an incoming `boardJSON`, only `cells` is read
back — `hEdges`/`vEdges`/`givens`/`rows`/`cols` are ignored and re-derived
from the puzzle.

### Move semantics (two-handed scheme)

Per the TUI spec's Tango row: primary places **sun** (pressing it again on
a sun clears it back to empty), secondary places **moon**. Mapped to clicks:
click a non-given cell to cycle `0 -> 1 -> 0`; shift-click to cycle
`0 -> 2 -> 0`. Never mutate a cell where `givens[row][col] === true`.

### Solution JSON

```ts
{ cells: number[6][6] }   // the fully solved grid, same 0/1/2 encoding as board.cells
```

(Tango's engine solution type is itself a full solved `Board`, so this is
literally that board's `cells`, reshaped to 2D — see
`internal/games/tango/tango.go`'s `Board` and
`internal/games/tango/generator.go`'s `Generate`.)

### Worked examples

**1. Freshly generated puzzle** (`generate("tango","easy",1)`, truncated):

```json
{
  "board": {
    "rows": 6, "cols": 6,
    "cells":  [[0,0,0,0,0,0],[0,0,0,2,1,1],[0,2,0,1,0,0],[0,0,0,0,0,0],[0,0,0,0,2,0],[0,1,0,0,0,2]],
    "givens": [[false,false,false,false,false,false],[false,false,false,true,true,true],
               [false,true,false,true,false,false],[false,false,false,false,false,false],
               [false,false,false,false,true,false],[false,true,false,false,false,true]],
    "hEdges": [[0,0,0,0,0],[2,2,0,0,0],[0,0,0,0,1],[2,0,0,0,2],[0,2,0,0,2],[0,0,0,0,0]],
    "vEdges": [[0,2,0,0,0,0],[0,0,0,2,0,2],[1,0,0,0,0,1],[0,2,0,2,0,0],[0,1,0,0,1,0]]
  }
}
```
`violations(...)` on this exact board returns `[]` (nothing filled by the
player yet — the givens themselves are always consistent). `solved(...)`
returns `{"solved": false}` (many cells still `0`).

**2. Player breaks two rules at once.** Starting from example 1, set
`cells[3][0]=cells[3][1]=cells[3][2]=1` (three suns in a row) and
`cells[1][0]=cells[1][1]=1` (both sun, but `hEdges[1][0]` above is a `2`
i.e. `×`/cross, which requires them to differ):

```json
[
  {"rule":"balance","message":"row 1 has more than 3 of one symbol",
   "cells":[{"row":1,"col":0},{"row":1,"col":1},{"row":1,"col":2},{"row":1,"col":3},{"row":1,"col":4},{"row":1,"col":5}]},
  {"rule":"three-in-a-row","message":"row 3 has three consecutive identical symbols starting at column 0",
   "cells":[{"row":3,"col":0},{"row":3,"col":1},{"row":3,"col":2}]},
  {"rule":"edge-constraint","message":"edge between cells 6 and 7 violates its constraint",
   "cells":[{"row":1,"col":0},{"row":1,"col":1}]},
  {"rule":"edge-constraint","message":"edge between cells 18 and 19 violates its constraint",
   "cells":[{"row":3,"col":0},{"row":3,"col":1}]}
]
```

(Rule strings this game can emit: `"balance"`, `"three-in-a-row"`,
`"edge-constraint"`.)

**3. Solved.** Copy `solution.cells` into `board.cells` unchanged (edges and
givens are already consistent with the solution by construction):
`violations(...)` returns `[]` and `solved(...)` returns
`{"solved": true}`.

---

## Queens

N×N grid (N varies puzzle-to-puzzle, 5..11, independent of difficulty)
divided into N connected colored regions. Exactly one queen per row, per
column, per region, with no two queens 8-adjacent (touching, including
diagonally — only local adjacency, never the full chess diagonal).

### Board JSON

```ts
{
  n: number,
  regions: number[n][n],   // region id 0..n-1 per cell (immutable)
  cells:   number[n][n],   // 0 = empty, 1 = queen
  givens:  boolean[n][n],  // true = pre-placed, locked queen cell
}
```

**Important:** the engine's board model only ever tracks `Empty`/`Queen`.
The player's "X" mark (used to note "no queen can go here") is explicitly a
TUI/UI-only concept with **no engine representation** — see the doc comment
on `Cell` in `internal/games/queens/queens.go`. Track X marks entirely in
UI-local state; never send them through `cells` (only `0`/`1` are valid
there), and don't expect `violations`/`solved` to know or care about them.
As with Tango, `regions`/`givens`/`n` are ignored on the way back in —
only `cells` round-trips.

### Move semantics (two-handed scheme)

Primary (`Space`/click) places an **X mark** (UI-only, again = clear — does
not touch `cells` at all); secondary (`Shift+Space`/shift-click) places a
**queen** (again = clear): shift-click toggles `cells[row][col]` between `0`
and `1`. Never mutate a cell where `givens[row][col] === true`.

### Solution JSON

```ts
{ cells: number[n][n] }   // exactly one 1 per row/col/region, rest 0
```

(Queens' engine `Solution` is `{N, QueenAt []int}`, one column-per-row; this
wire form expands it to the same full-grid shape as `board.cells` so hints
can diff the two directly.)

### Worked examples

**1. Freshly generated 7×7 puzzle** (`generate("queens","easy",42)`,
`board.cells` all zero, `board.givens` all false — Queens puzzles from this
generator carry no pre-placed givens in practice, though the format
supports them):

```json
{"n":7,
 "regions":[[0,3,3,3,1,1,2],[3,3,3,3,1,2,2],[3,3,3,3,2,2,2],[3,3,3,3,5,4,2],
            [6,3,6,5,5,4,2],[6,3,6,5,5,5,5],[6,6,6,6,5,5,5]]}
```

**2. Player places three mutually-conflicting queens**
(`cells[0][0]=cells[0][1]=1` on the board above — same row, and region 0
happens to cover both cells, and they're 8-adjacent):

```json
[
  {"rule":"same-row","message":"two queens share a row","cells":[{"row":0,"col":0},{"row":0,"col":1}]},
  {"rule":"same-region","message":"two queens share a region","cells":[{"row":0,"col":0},{"row":0,"col":1}]},
  {"rule":"adjacent","message":"two queens touch (including diagonally)","cells":[{"row":0,"col":0},{"row":0,"col":1}]}
]
```

(Rule strings this game can emit: `"same-row"`, `"same-col"`,
`"same-region"`, `"adjacent"`.)

**3. Solved.** Copy `solution.cells` into `board.cells`: `violations`
returns `[]`, `solved` returns `{"solved": true}`.

---

## Mini Sudoku

6×6 grid, digits 1..6, every row/column/2×3 box contains each digit exactly
once. Boxes are 2 rows tall × 3 columns wide (not 3×2, not 2×2). Grid size
is always fixed at 6×6.

### Board JSON

```ts
{
  rows: 6, cols: 6, boxRows: 2, boxCols: 3,
  cells:  number[6][6],   // 0 = empty, else 1..6
  givens: boolean[6][6],  // true = pre-filled clue cell (immutable, locked)
}
```

`boxRows`/`boxCols` are included for forward-compatibility/self-description
(a future clone could vary box geometry) but are currently always `2`/`3`.
As with the other grids, `givens`/dimensions round-trip for rendering but
are ignored on the way back in — only `cells` matters to
`violations`/`solved`.

### Move semantics (two-handed scheme)

Primary (`Space`+digit, or click-then-type): pressing digit key `1`-`6`
(after selecting/clicking a non-given cell) sets `cells[row][col]` to that
digit; pressing the same digit again (or `0`/`Backspace`) clears it back to
`0`. Secondary (`Shift+1`-`6`) toggles a **pencil note** — pencil notes are
UI-only scratch marks with no engine representation (exactly like Queens'
X marks); never send them through `cells`.

### Solution JSON

```ts
{ cells: number[6][6] }   // fully solved grid, digits 1..6, same shape as board.cells
```

### Worked examples

**1. Freshly generated puzzle** (`generate("minisudoku","easy",3)`,
truncated to the first two rows):

```json
{
  "cells":  [[5,0,4,6,3,1],[6,3,0,2,0,5], "...4 more rows..."],
  "givens": [[true,false,true,true,true,true],[true,true,false,true,false,true], "..."]
}
```
`violations` on the untouched board is `[]`; `solved` is `{"solved":false}`.

**2. Player creates a duplicate.** Set `cells[0][1] = 5` (row 0 already has
a `5` at column 0; column 1 already has a `5` at row 5; and both cells
share box 0):

```json
[
  {"rule":"row","message":"digit 5 repeated in row 0","cells":[{"row":0,"col":0},{"row":0,"col":1}]},
  {"rule":"column","message":"digit 5 repeated in column 1","cells":[{"row":0,"col":1},{"row":5,"col":1}]},
  {"rule":"box","message":"digit 5 repeated in box 0","cells":[{"row":0,"col":0},{"row":0,"col":1}]}
]
```

(Rule strings this game can emit: `"value"` (an out-of-1..6 digit — can
only happen from a malformed/tampered board, since the UI's own input
should never produce one), `"row"`, `"column"`, `"box"`.)

**3. Solved.** Copy `solution.cells` into `board.cells`: `violations`
returns `[]`, `solved` returns `{"solved": true}`.

---

## Zip

R×C grid (5×5 for Easy, 6×6 for Medium/Hard/Expert). Draw one continuous
path through every cell, passing through numbered waypoints 1..K in
ascending order, moving only orthogonally, never crossing a wall.

### Board JSON

```ts
{
  rows: number, cols: number,
  waypoints: number[rows][cols],  // 0 = no waypoint; else the 1..K number (immutable)
  hWalls: boolean[rows][cols-1],  // hWalls[r][c]: wall on edge (r,c)-(r,c+1) (immutable)
  vWalls: boolean[rows-1][cols],  // vWalls[r][c]: wall on edge (r,c)-(r+1,c) (immutable)
  path: {row:number, col:number}[], // the player's drawn path so far, in visiting order
}
```

Deviation from the task brief's suggested single `walls` field: walls are
split into `hWalls`/`vWalls` (mirroring Tango's `hEdges`/`vEdges`) so every
game's edge data lives in the same row/col-indexed grid shape rather than
an edge-list. `waypoints`/`hWalls`/`vWalls`/`rows`/`cols` are echoed for
rendering but ignored on the way back in — only `path` round-trips.

`path` starts as `[]` (nothing drawn). It need not be complete or even
end at a legal cell to call `violations` — a path with 0, 1, or many cells
is fine; only *already-broken* rules are reported (e.g. a path that hasn't
reached the far side yet is not "wrong", it's just incomplete). A
dead-ended/stranded-but-not-yet-illegal path is *not* flagged either — per
the engine spec, "trapping" is a UX hint concern, not a hard validator
rule.

### Move semantics (two-handed scheme)

Per the TUI spec, Zip's primary action is "pen down/up" while `wasd`
extends the path cell-by-cell; secondary erases the last segment. Mapped to
the mouse interaction (the spec calls this "the defining Zip interaction"):
click-drag starting from the cell numbered `1` appends each cell the
pointer enters to `path` (only if orthogonally adjacent to `path`'s current
last cell and not already in `path`, mirroring the fallback `Backspace`
"erase" by dragging back over the immediately-preceding cell, which should
pop it off the end of `path`). Always resend the *entire* `path` array —
the bridge has no incremental/append API.

### Solution JSON

```ts
{ path: {row:number, col:number}[] }   // the full Hamiltonian path, start (waypoint 1) to end (max waypoint)
```

### Worked examples

**1. Freshly generated 5×5 puzzle** (`generate("zip","easy",3)`):

```json
{
  "rows": 5, "cols": 5,
  "waypoints": [[9,8,7,6,5],[10,1,2,3,4],[11,16,17,18,25],[12,15,20,19,24],[13,14,21,22,23]],
  "hWalls": [[false,false,false,false],[false,false,false,false],[false,false,false,false],[false,false,false,false],[false,false,false,false]],
  "vWalls": [[false,false,false,false,false],[false,false,false,false,false],[false,false,false,false,false],[false,false,false,false,false]],
  "path": []
}
```
(This particular puzzle happens to have no walls — `hWalls`/`vWalls` are
all `false`. Cell `(1,1)` carries waypoint `1`, so a legal path must start
there.) `violations` on the empty path is `[]`; `solved` is
`{"solved": false}`.

**2. A puzzle with walls** (`generate("zip","easy",1)`, a different seed):
`walls` decode to e.g. `hWalls[2][1] = true` (a wall on the edge between
`(2,1)`-`(2,2)`) and `vWalls[1][2] = true`, `vWalls[2][3] = true`. If the
player's `path` steps directly from `(2,1)` to `(2,2)`, `violations` reports:

```json
[{"rule":"wall-crossing","message":"a step crosses a wall","cells":[{"row":2,"col":1},{"row":2,"col":2}]}]
```

(Rule strings this game can emit: `"revisit"`, `"non-adjacent-step"`,
`"wall-crossing"`, `"waypoint-order"`, `"wrong-start"`.)

**3. Solved.** Set `board.path = solution.path` unchanged: `violations`
returns `[]`, `solved` returns `{"solved": true}`.

---

## Patches

Fixed 5×5 grid. Partition it into one axis-aligned rectangle per clue, each
rectangle's cell count matching its clue's number and its aspect ratio
matching its clue's shape (`square`: width == height; `wide`: width >
height; `tall`: height > width; `free`: any).

### Board JSON

```ts
{
  rows: 5, cols: 5,
  clues: {row:number, col:number, area:number, shape:string}[],  // shape: "square"|"wide"|"tall"|"free" (immutable)
  labels: number[5][5],  // -1 = uncovered; else an opaque, UI-assigned rectangle id
}
```

`labels` is the one genuinely UI-owned piece of board state: to place a
rectangle, choose **any** integer not currently used by another rectangle
and write it into every cell of the rectangle's bounding box (all `-1`
before that). The exact numeric values never matter, only the *grouping*
does — the validator recomputes each label's bounding box and checks the
group's cells exactly fill it (a "ragged" group — the label's cells not
forming a solid rectangle — is how an overlap between two attempted
rectangles surfaces; see `RuleExactCover`'s doc comment in
`internal/games/patches/validator.go`). To remove a placed rectangle, reset
all of its cells back to `-1`. `clues`/`rows`/`cols` are echoed for
rendering but ignored on the way back in — only `labels` round-trips.

### Move semantics (two-handed scheme)

Per the TUI spec ("the defining interaction for that game"): click a clue
cell and drag to the opposite corner to define a rectangle; releasing
commits it — on release, pick a fresh label id and write it across the
bounding box in `labels`. Secondary (`Shift+Space`/shift-click on a placed
rectangle) removes it — reset every cell carrying that rectangle's label
back to `-1`.

### Solution JSON

```ts
{ labels: number[5][5] }  // a complete tiling; label i is Solution.Rects[i]'s cells (solver order — NOT meaningful to compare against a UI-chosen labeling)
```

Because `labels` values are opaque/UI-chosen on the board but
solver-ordered in the solution, do not compare `board.labels ===
solution.labels` cell-by-cell to check correctness — use `solved()`.

### Worked examples

**1. Freshly generated puzzle** (`generate("patches","easy",3)`):

```json
{
  "rows": 5, "cols": 5,
  "clues": [
    {"row":0,"col":1,"area":4,"shape":"wide"},
    {"row":2,"col":4,"area":4,"shape":"tall"},
    {"row":3,"col":2,"area":16,"shape":"square"},
    {"row":4,"col":4,"area":1,"shape":"square"}
  ],
  "labels": [[-1,-1,-1,-1,-1],[-1,-1,-1,-1,-1],[-1,-1,-1,-1,-1],[-1,-1,-1,-1,-1],[-1,-1,-1,-1,-1]]
}
```
`violations` on the fully-uncovered board reports only the blanket
coverage rule (never a per-cell one, since nothing is placed yet):
`[{"rule":"exact-cover","message":"some cells are not covered by any rectangle","cells":[]}]`.
`solved` is `{"solved": false}`.

**2. Player commits a wrong-area rectangle.** Cover just `(0,0)` and
`(0,1)` with label `0` (2 cells), but the clue at `(0,1)` needs area 4:

```json
[
  {"rule":"exact-cover","message":"some cells are not covered by any rectangle","cells":[]},
  {"rule":"area","message":"a rectangle's area does not match its clue's number","cells":[{"row":0,"col":1}]}
]
```

Extending that same rectangle to the full 2×2 block `(0,0)-(1,1)` (4 cells,
matching the clue's `area:4`, but the clue's `shape` is `"wide"` and 2×2 is
square, not wide) instead reports:

```json
[
  {"rule":"exact-cover","message":"some cells are not covered by any rectangle","cells":[]},
  {"rule":"shape","message":"a rectangle's shape does not match its clue's shape","cells":[{"row":0,"col":1}]}
]
```

(Rule strings this game can emit: `"exact-cover"`, `"one-clue"`, `"area"`,
`"shape"`.)

**3. Solved.** Copy `solution.labels` into `board.labels` unchanged (the
exact label integers from the solution work fine as a labeling — they just
also happen to be usable directly): `violations` returns `[]`, `solved`
returns `{"solved": true}`.
