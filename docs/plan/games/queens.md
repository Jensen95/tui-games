# Queens — Specification

## Summary

Queens is LinkedIn's spatial-logic puzzle: an **N×N** grid divided into **N colored regions**, where you place exactly one queen per row, per column, and per region, such that **no two queens touch — not even diagonally**. It's a regional-constraint variant of the classic N-Queens problem with a Star Battle flavor. Every board has exactly one solution reachable by pure deduction.

## Grid & pieces

- Square grid, size **N×N**. LinkedIn ships varying sizes day to day; commonly 7–11. Support at least **5×5 … 11×11** (parameterize on N).
- **Number of regions == N.** Each region is a *connected* set of cells sharing one color. Every cell belongs to exactly one region.
- Cell state: `Empty`, `Queen`, or (UI-only) `Marked` (a player's "X" note — not part of the solution model).
- Puzzles usually open with a queen or two pre-placed as givens (optional).

## Rules (precise)

A placement of exactly N queens is valid iff:

1. **One per row.** Every row contains exactly one queen.
2. **One per column.** Every column contains exactly one queen.
3. **One per region.** Every colored region contains exactly one queen.
4. **No touching (local).** No two queens occupy edge- **or** corner-adjacent cells. A queen forbids its ≤8 neighbors.

> **Critical subtlety vs. classic N-Queens:** the diagonal rule is **local only**. Two queens far apart on the same diagonal are perfectly legal. Only *adjacency* (the 8 surrounding cells) is forbidden. Encoding the full-diagonal chess rule is the single most common implementation error — assert against it.

## Solved-state definition (for the validator)

```
Valid(placement) :=
  |queens| == N
∧ ∀ row:    exactly one queen
∧ ∀ col:    exactly one queen
∧ ∀ region: exactly one queen
∧ ∀ pair(q1,q2): chebyshev_distance(q1,q2) > 1   // no 8-neighbor adjacency
```

Partial validator: flags any *already* violated constraint (two queens sharing a row/col/region, or adjacent) without requiring N placements.

## Deduction ladder (no-guess guarantee + hints)

1. **Singleton region/row/col.** A region (or row/col) with only one legal cell → forced queen.
2. **Elimination cascade.** Placing a queen removes its row, column, and 8 neighbors from the candidate pool; re-scan for new singletons.
3. **Region-line lock.** If all remaining legal cells of a region lie in one row (or column), that row/col is "reserved" for the region → eliminate other regions' candidates there.
4. **Set locking (k regions in k lines).** k regions confined to k rows/cols reserve those lines (generalized line lock).
5. **Adjacency exclusion.** Two small adjacent regions often mutually eliminate via the no-touch rule.

Well-designed Queens boards are solvable by rules 1–4 with no guessing.

## Data model (Go sketch)

```go
type Cell uint8
const (Empty Cell = iota; Queen)

type Puzzle struct {
    N       int
    Region  []int  // len N*N, region id (0..N-1) per cell, row-major
    Givens  map[int]bool // cell indices pre-placed with a queen (optional)
    Seed    int64
    Diff    Difficulty
}

type Solution struct {
    N     int
    QueenAt []int // len N; QueenAt[row] = column of that row's queen
}
```

Representing a solution as one column per row bakes in rule 1 for free and shrinks the search space.

## Generation approach

Two coupled problems: (a) a valid queen placement, (b) a region coloring that makes that placement the **unique** solution.

1. **Generate a placement.** Randomly place N non-touching queens, one per row and column (permutation of columns, rejected if any two are adjacent). Fast for small N; retry on failure.
2. **Grow regions around the placement.** Seed each region at one queen's cell, then flood-grow regions by randomly assigning unclaimed cells to an adjacent region until every cell is colored and every region stays connected. This *guarantees* one queen per region (each region is seeded by exactly one queen).
3. **Enforce uniqueness by reshaping.** Run the complete solver on the colored board. If it finds >1 solution, adjust region boundaries (move contested border cells between regions) and re-test, or restart. Bias reshaping to tighten regions around alternative placements until only the intended one survives.
4. **Difficulty targeting.** Easy: compact regions, more forced singletons early, optional givens. Hard: elongated/interlocking regions, deeper set-locking required, no givens.

## Solver approach (uniqueness + solvability)

- **Complete solver:** DFS over rows (choose a legal column for each row honoring col/region/adjacency), counting solutions **up to 2**. Uniqueness == count 1. This is ground truth and is very fast for N ≤ 12.
- **Logic solver:** applies the deduction ladder to fixpoint; used to (a) certify no-guessing, (b) label difficulty, (c) drive hints.
- **Cross-validation invariant:** logic-solver solution must equal complete-solver solution, and complete-solver count must be 1.

## Uniqueness & deduplication

- **Symmetry group:** 8 dihedral transforms of the square. Region *colors* are just labels, so canonicalization must be **color-agnostic** — normalize region ids by first-appearance order after each transform. Canonical form = lexicographically smallest `(region-map, givens)` over the 8 transforms with normalized region labels.
- Fingerprint = hash of canonical form; reject collisions against the corpus.

## TUI interaction

- **Cursor:** arrows / `hjkl`.
- **Place / cycle:** LinkedIn's convention is single-tap = mark "X", double-tap = queen; mirror it. Keyboard: `Space` cycles Empty → Marked → Queen → Empty; `Enter` places a queen directly; `x` toggles a mark.
- **Mouse:** left-click cycles (Empty→Mark→Queen), matching LinkedIn; **click-drag** paints marks across multiple cells (their "drag to mark" affordance). Right-click clears.
- **Region rendering:** each region a distinct theme color (background tint); pick an N-color palette that stays distinguishable and colorblind-friendly (avoid red/green-only separation; consider patterns/borders as a secondary channel).
- **Feedback:** conflicting queens flash; a subtle count per region ("region still needs a queen") as an optional assist.
- Helpers: `u` undo, `Ctrl+r` reset, `?` help, `H` hint (reveal one forced queen + the rule that forces it).

## TDD test matrix (red tests first)

**Validator**
- Known valid solution → true.
- Two queens same row / same col / same region → the specific violation.
- Two queens edge-adjacent → adjacency violation.
- Two queens **corner-adjacent** → adjacency violation (guards the 8-neighbor rule).
- Two queens on the same long diagonal but *not* adjacent → **no** violation (guards against the classic-chess bug).

**Solvers**
- Golden board (known unique solution) → complete solver returns it; count == 1.
- Hand-built ambiguous board → count == 2.
- Logic solver closes all shipped examples with no guessing.

**Generator (property-based over many seeds & sizes 5..11)**
- Every generated board: exactly N regions, all connected, every cell colored.
- Complete-solver count == 1; logic solver closes it; difficulty label correct.
- Fingerprints pairwise distinct across a large batch.
- Region relabeling / rotation of a puzzle yields the **same** fingerprint (canonicalization test).
- Generation p99 latency under target across sizes.

**Determinism:** same seed → identical board.

## Gotchas

- The local-vs-global diagonal rule (see above).
- Region connectivity must be preserved during any boundary reshaping.
- Color labels are non-semantic — never let them leak into fingerprints or solver logic.
- Corner-adjacency counts as touching.

## References

- Rules (one per row/col/region, no touch incl. diagonal): https://www.coolmathgames.com/0-queens-by-linkedin
- Local (not full-diagonal) touch rule, explained precisely: https://playqueensgame.org/blog/queens-game-rules-explained/
- N == regions, unique solution, sizes vary: https://dailyqueensgame.com/how-to-play
- Grid sizes vary by day; N-Queens lineage: https://www.thewordfinder.com/linkedin-games-hub/queens-hints/
