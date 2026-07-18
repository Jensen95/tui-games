# Mini Sudoku — Specification

## Summary

Mini Sudoku is LinkedIn's compact Sudoku: a **6×6** grid filled with digits **1–6** so that every row, every column, and every **2×3 box** contains each digit exactly once. It's a standard 6×6 Sudoku (the smallest "interesting" rectangular-box Sudoku). This is the most well-trodden of the five — reuse decades of established Sudoku technique.

## Grid & pieces

- Grid: **6×6** (36 cells), digits `1..6`.
- **Boxes:** the grid is divided into six **2-row × 3-column** boxes (2 boxes tall × 3 boxes wide → 6 boxes). Keep the box shape configurable (`boxH×boxW`, default `2×3`) so a future 4×4 (`2×2`) or 8×8 (`2×4`) is trivial. For 6×6 the canonical box is 2×3.
- **Givens:** a subset of cells pre-filled and locked.
- Cell state: `0` (empty) or `1..6`.

## Rules (precise)

A completed grid is valid iff:

1. **Row constraint.** Each row contains `1..6` exactly once.
2. **Column constraint.** Each column contains `1..6` exactly once.
3. **Box constraint.** Each 2×3 box contains `1..6` exactly once.

A well-formed puzzle has a **unique** solution reachable by logic.

## Solved-state definition (for the validator)

```
Valid(grid) :=
  ∀ row: {grid[row][*]} == {1..6}
∧ ∀ col: {grid[*][col]} == {1..6}
∧ ∀ box: {cells of box}   == {1..6}
```

Partial validator: flags a duplicate digit within any row/col/box among filled cells; never flags empties.

## Deduction ladder (no-guess guarantee + hints)

Standard Sudoku technique ladder, scaled to 6×6:

1. **Naked single.** A cell with one remaining candidate.
2. **Hidden single.** A digit with only one possible cell in a row/col/box.
3. **Naked/hidden pairs** and **pointing pairs / box-line reduction.**
4. (Rarely needed at 6×6) higher techniques.

Difficulty = deepest technique required. Most 6×6 boards resolve with singles + pairs.

## Data model (Go sketch)

```go
type Puzzle struct {
    N       int          // 6
    BoxH, BoxW int       // 2, 3
    Givens  map[int]int  // cell index -> digit 1..6
    Seed    int64
    Diff    Difficulty
}

type Solution struct { Cells []int } // len N*N, digits 1..6, row-major
```

## Generation approach (textbook, low-risk)

1. **Build a full valid solution.** Backtracking fill of the 6×6 with digit constraints, randomized digit order → a complete Latin-square-with-boxes grid. (Optionally seed diversity via random band/stack/digit permutations of a base solution.)
2. **Carve givens.** Remove digits one at a time in random order; after each removal, confirm the puzzle still has a **unique** solution (complete solver, count up to 2). Keep the removal only if uniqueness holds. Stop when no more can be removed (minimal) or at the given-count that matches the target difficulty.
3. **Difficulty targeting.** Easy: more givens, solvable by naked/hidden singles only. Hard: fewer givens, requires pairs/pointing. Use the logic solver's deepest-technique to label.
4. **Symmetry (cosmetic option).** LinkedIn/Sudoku often use symmetric given patterns; optionally enforce 180° rotational symmetry of givens for aesthetics — not required.

## Solver approach (uniqueness + solvability)

- **Complete solver:** constraint-propagation DFS or exact-cover/DLX; count solutions **up to 2**; uniqueness == 1. Trivial performance at 6×6.
- **Logic solver:** applies the technique ladder to fixpoint; certifies no-guessing, labels difficulty, drives hints.
- **Cross-validation invariant:** logic-solver solution == complete-solver solution; complete-solver count == 1.

## Uniqueness & deduplication

- **Symmetry group (for "same puzzle"):** Sudoku's symmetry group is large. A practical, sufficient set for dedup: 8 dihedral transforms **×** digit relabeling (normalize by first-appearance order) **×** the box-preserving band/stack permutations (swap the two row-bands; permute the three column-stacks; permute rows within a band; permute columns within a stack). Canonicalize by minimizing the serialized givens over this group (or a documented subset if full canonicalization is too costly — a smaller group still catches the vast majority of dupes).
- Fingerprint = hash of canonical givens. Reject corpus collisions.

## TUI interaction

- **Cursor:** arrows / `hjkl`.
- **Enter digit:** press `1`–`6` to set the focused cell; `0`/`Delete`/`Backspace` clears. Givens are immutable and styled distinctly.
- **Pencil marks (candidates):** a toggle mode (`p`) where `1`–`6` add/remove small candidate marks in the cell corner — standard Sudoku QoL.
- **Mouse:** left-click focuses a cell; a clickable on-screen number pad (or click-then-type) sets the value; right-click clears. Optionally support click-drag on the number pad for pencil marks.
- **Feedback:** duplicate digits in a row/col/box highlight in red (live partial validator); win banner on completion. Optional "highlight all cells with digit X" when a filled cell is focused.
- Helpers: `u` undo, `Ctrl+r` reset, `?` help, `H` hint (reveal one logically-forced cell + the technique).

## TDD test matrix (red tests first)

**Validator**
- A correct completed grid → valid.
- Duplicate in a row → row violation (only that row).
- Duplicate in a column → column violation.
- Duplicate within a 2×3 box (but not sharing a row/col — e.g. cells (0,0) and (1,2)) → **box** violation (guards box-geometry correctness).
- Empty cells present → partial validator reports no violation.

**Solvers**
- Golden puzzle (known unique solution) → complete solver returns it; count == 1.
- Hand-built ambiguous puzzle (too few givens) → count == 2.
- Logic solver closes all shipped examples with no guessing.

**Generator (property-based over many seeds)**
- Every generated puzzle: `Valid(solution)`; complete-solver count == 1; logic solver closes it; difficulty label correct.
- Minimality (if promised): removing any remaining given breaks uniqueness.
- Fingerprints pairwise distinct across a batch; digit-relabeled / rotated copy of a puzzle shares a fingerprint (canonicalization test).
- Generation p99 latency under target (will be tiny).

**Determinism:** same seed → identical puzzle.

## Gotchas

- **Box geometry is 2×3, not 3×2 and not 2×2** for a 6×6 — the classic off-by-shape bug. Test with a box-only violation that isn't also a row/col violation.
- Keep `BoxH/BoxW` parameterized so tests can assert the mapping cell→box.
- Digit labels are symbols; the dedup canonicalizer must relabel them.

## References

- Rules (fill 6×6 so each row/column/box has 1–6): https://gamedaily.com/guides/todays-linkedin-games-answers-2-17-26
- Mini Sudoku = compact 6×6 Sudoku, newest LinkedIn addition: https://www.thewordfinder.com/linkedin-games-hub/
- Lineup + "fill a grid with numbers one through six": https://www.yahoo.com/lifestyle/articles/today-linkedin-games-answers-february-212054957.html
