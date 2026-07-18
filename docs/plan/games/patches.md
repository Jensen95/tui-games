# Patches — Specification

## Summary

Patches is LinkedIn's rectangle-partition puzzle: divide the whole grid into **non-overlapping rectangles that tile it completely**, where each rectangle contains exactly **one numbered clue**, the number equals the rectangle's **area** (cell count), and each clue also specifies a **shape type** (square / wide / tall / free) the rectangle must match. It is the classic **Shikaku (Rectangles)** puzzle with an added shape-type constraint. Colors are purely cosmetic. Every board has a unique solution.

## Grid & pieces

- Grid: LinkedIn ships **5×5**; clones use 4×4–7×7 (some 6×6). Parameterize on `R×C` (default 5×5).
- **Clues**: some cells contain a clue = `(number, shapeType)`. The count of clues equals the number of rectangles in the solution, and the numbers sum to `R*C`.
- **Shape type** per clue:
  - `Square` — the rectangle must be a square (`k×k`, area = k²).
  - `Wide` — width **strictly greater** than height.
  - `Tall` — height **strictly greater** than width.
  - `Free` / `Any` — no shape restriction (the "special" shape that can be square, wide, or tall).
- A player's move = drawing/anchoring a rectangle that covers a clue.

## Rules (precise)

A tiling into rectangles is a valid solution iff:

1. **Exact cover.** Every cell belongs to exactly one rectangle; no overlaps, no gaps.
2. **One clue per rectangle.** Each rectangle contains exactly one clue cell (a rectangle with zero or ≥2 clues is invalid).
3. **Area matches number.** Each rectangle's area (width × height) equals its clue's number.
4. **Shape type matches.** The rectangle's dimensions satisfy the clue's shape type:
   - `Square`: width == height.
   - `Wide`: width > height.
   - `Tall`: height > width.
   - `Free`: any width×height with the right area.

> Note the shape constraint genuinely narrows things: a clue `Wide · 6` could be `6×1`, `3×2` (both width>height) but **not** `1×6` or `2×3`. Working out which valid rectangle of the right area *and* shape fits is the puzzle. Colors carry no meaning.

## Solved-state definition (for the validator)

```
Valid(rects) :=
  every cell covered exactly once by exactly one rect        // exact cover
∧ ∀ rect: exactly one clue inside
∧ ∀ rect: rect.w * rect.h == rect.clue.number
∧ ∀ rect: shapeOK(rect.w, rect.h, rect.clue.shape)
```

Partial validator (live in TUI): each placed rectangle must be axis-aligned, contain exactly one clue, match that clue's area+shape, and not overlap an existing rectangle. The board is won when placed rectangles tile the grid.

## Data model (Go sketch)

```go
type Shape uint8
const (Square Shape = iota; Wide; Tall; Free)

type Clue struct { Number int; Shape Shape }

type Puzzle struct {
    R, C   int
    Clues  map[int]Clue // anchor cell index -> clue
    Seed   int64
    Diff   Difficulty
}

type Rect struct { R0, C0, W, H int } // top-left + dims
type Solution struct { Rects []Rect } // each maps 1:1 to a clue
```

## Generation approach

Partition first, then derive clues — exactly the Shikaku generation pattern.

1. **Randomly partition the grid into rectangles.** Recursive approach: maintain a set of uncovered cells; repeatedly pick an uncovered cell and grow a random axis-aligned rectangle from it that stays within bounds and within uncovered space; commit it; repeat until the grid is tiled. Bias rectangle sizes toward a target distribution (avoid degenerate all-1×1 tilings). Alternative: recursive guillotine splits, then randomly merge, to control the size mix.
2. **Assign each rectangle its clue.** `number = area`. **shapeType** = derive from dimensions, but you get to *choose* how specific to be: for a square rectangle you may label it `Square` (specific) or `Free` (loose); for a non-square you may label `Wide`/`Tall` (specific) or `Free`. More specific labels ⇒ easier. Choose the clue's **anchor cell** = a random cell inside the rectangle (LinkedIn places the number somewhere within the region).
3. **Enforce uniqueness.** Run the complete solver. If >1 tiling satisfies the clues, tighten clues (make a `Free` into its specific `Wide`/`Tall`/`Square`, or nudge a partition boundary) until the solution is unique. Loosening clues raises difficulty but must never break uniqueness.
4. **Difficulty targeting.** Easy: specific shapes, primes/near-edge clues that have only one orientation, compact grid. Hard: more `Free` clues, larger rectangles with several valid orientations, interference between neighboring clues. (Prime-numbered clues like 5 or 7 have only `1×n`/`n×1` options → natural easy anchors; a `9` marked `Free` could be `3×3`, `1×9`, or `9×1` → harder.)

## Solver approach (uniqueness + solvability)

- **Complete solver:** model as exact cover — each clue can be realized by an enumerable set of candidate rectangles (all placements of the right area+shape that cover the clue and stay in bounds); choose one candidate per clue so they tile the grid with no overlap. Solve via **Algorithm X / DLX (dancing links)** or constraint-propagation DFS. Count solutions **up to 2**; uniqueness == 1. Very fast at these sizes.
- **Logic solver (optional):** propagate forced placements — a clue with a single feasible candidate rectangle is forced; a cell coverable by only one candidate forces it — to certify no-guessing and label difficulty and drive hints.
- **Cross-validation invariant:** complete-solver count == 1; logic solver (if used) agrees.

## Uniqueness & deduplication

- **Symmetry group:** 8 dihedral transforms of the grid (rotations/reflections). Colors are cosmetic so they never enter the fingerprint. Clue *anchor position within a rectangle* is cosmetic-ish — decide whether two puzzles that differ only in where the number sits inside identical rectangles are "the same"; recommended: **treat anchor position as significant** (it changes the visual puzzle) but normalize under the 8 transforms.
- Fingerprint = hash of canonical `(clue positions, numbers, shapes)`.

## TUI interaction

- **Draw rectangles by mouse (primary):** click a clue cell and **drag** to the opposite corner to define the rectangle, matching LinkedIn's "click a numbered cell and drag" affordance. Release commits it; click a placed rectangle to remove it.
- **Keyboard drawing:** move cursor to a clue, press `Enter` to start a rectangle anchored there, use arrows to extend width/height, `Enter` to commit, `Esc` to cancel. `x`/`Delete` on a covered cell removes its rectangle.
- **Rendering:** each committed rectangle filled with a distinct (cosmetic) color + border; clue badge shows number + a small shape icon (□ square, ▭ wide, ▯ tall, ◇ free). Invalid attempts flash and are rejected.
- Helpers: `u` undo, `Ctrl+r` reset, `?` help, `H` hint (reveal one forced rectangle).

## TDD test matrix (red tests first)

**Validator**
- A correct hand-built tiling → valid.
- Two rectangles overlapping → exact-cover violation.
- A gap (uncovered cell) → invalid.
- A rectangle containing two clues / zero clues → one-clue violation.
- Rectangle whose area ≠ clue number → area violation.
- `Wide` clue realized as a tall rectangle → shape violation; `Square` realized as `2×3` → shape violation.
- Prime clue (e.g. 5) realized as `1×5` → valid; as `2×3`-ish impossible (guards area+shape interplay).

**Solvers**
- Golden puzzle (unique tiling) → complete solver returns it; count == 1.
- Hand-built puzzle with two tilings → count == 2.
- Logic solver closes all shipped examples (if implemented).

**Generator (property-based over many seeds & sizes)**
- Every generated puzzle: clue numbers sum to `R*C`; recorded solution is `Valid`; complete-solver count == 1.
- Each clue's labeled shape is actually satisfied by its solution rectangle.
- Fingerprints pairwise distinct across a batch; dihedral transforms share a fingerprint.
- Generation p99 latency under target.

**Determinism:** same seed → identical puzzle.

## Gotchas

- `Wide`/`Tall` are **strict** inequalities (a square is neither wide nor tall).
- `Free` clues are the main source of multiple solutions — lean on the complete solver when using them.
- Exact-cover bookkeeping (no overlap **and** no gap) — test both failure directions separately.
- Colors must never influence solving, fingerprinting, or tests.

## References

- Official gameplay (fill grid with shapes, numbered cells = cell count, drag to draw, square/tall/wide + one special shape): https://www.coolmathgames.com/0-patches-by-linkedin
- Shikaku/Rectangles lineage; number = area; one number per shape; prime/border strategy: https://patchesanswer.com/insights/how-to-play-linkedin-patches
- 5×5 grid, colors are aesthetic only, constrained vs. free shapes: https://www.askdavetaylor.com/how-to-play-the-patches-game-on-linkedin/
- Shape types constrain which rectangles are valid (Square·4 = 2×2; Wide·6 = 6×1/3×2 only): https://www.patchesgame.com/how-to-play.html
