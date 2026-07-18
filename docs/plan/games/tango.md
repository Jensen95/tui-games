# Tango — Specification

## Summary

Tango is LinkedIn's binary-logic puzzle: a **6×6** grid where every cell holds one of two symbols (a **sun ☀** or a **moon ☾**). It is a modern skin over the classic **Takuzu / Binairo** (a.k.a. *0h h1*) puzzle, with an added layer of **equality / inequality edge constraints** between adjacent cells. Every well-formed puzzle has exactly one solution reachable by pure deduction — no guessing.

## Grid & pieces

- Fixed grid: **6×6** (36 cells). Keep the size a constant but parameterize the engine on an even `N` so a future 8×8 variant is trivial.
- Two symbols only. Model as a tri-state per cell: `Empty`, `Sun`, `Moon`.
- **Givens**: a subset of cells are pre-filled and locked.
- **Edge constraints**: between some orthogonally-adjacent cell pairs there is a marker:
  - `=` (equals): the two cells must hold the **same** symbol.
  - `×` (cross): the two cells must hold **different** symbols.

## Rules (precise)

A completed board is valid iff **all** of the following hold:

1. **Balance.** Each row contains exactly `N/2` suns and `N/2` moons (3 and 3 on a 6×6). Each column likewise.
2. **No three in a row.** No three identical symbols are consecutive **horizontally or vertically**. (This is *not* a diagonal rule — diagonals are unconstrained. Several secondary sources get this wrong; the canonical Takuzu rule and LinkedIn's own behavior are horizontal/vertical only.)
3. **Edge constraints.** Every `=` pair matches; every `×` pair differs.
4. *(Design property, optional to enforce)* **Uniqueness of lines.** In a well-designed board no two rows are identical and no two columns are identical. Treat this as a *solving aid / generation quality check*, not a hard rule the player is told about. Some Takuzu variants enforce it; LinkedIn boards are built to respect it but it is derivable from the above on a 6×6 in practice.

## Solved-state definition (for the validator)

```
Valid(board) :=
  ∀ row: count(Sun)==N/2 ∧ count(Moon)==N/2
∧ ∀ col: count(Sun)==N/2 ∧ count(Moon)==N/2
∧ ∀ line (row or col), ∀ i: ¬(cell[i]==cell[i+1]==cell[i+2] ∧ all filled)
∧ ∀ edge '=' (a,b): a==b
∧ ∀ edge '×' (a,b): a≠b
```

The **partial** validator (used live in the TUI and by tests) reports the subset of the above that is *already violated* by filled cells, and never flags an unfilled cell.

## Deduction ladder (drives both the "no-guess" guarantee and the hint system)

Order techniques from cheapest to most expensive; a puzzle is "solvable" if this ladder closes the board:

1. **Edge propagation.** A known cell + `=`/`×` edge forces its neighbor.
2. **Pair / doublet rule.** Two identical adjacent symbols force the opposite symbol on both outer flanks (else 3-in-a-row).
3. **Gap / sandwich rule.** Pattern `A _ A` forces the middle to `¬A`.
4. **Line-count rule.** A line already holding `N/2` of one symbol forces the rest to the other symbol.
5. **Uniqueness / line-difference** (advanced, optional): if only one completion keeps all rows/cols distinct, it is forced.

The generator's target difficulty = the deepest technique required.

## Data model (Go sketch)

```go
type Symbol uint8
const (Empty Symbol = iota; Sun; Moon)

type Relation uint8
const (None Relation = iota; Equal; Cross)

type Board struct {
    N     int
    Cells []Symbol // len N*N, row-major
}

type Puzzle struct {
    N      int
    Givens map[int]Symbol // cell index -> locked symbol
    // Edge constraints keyed by an ordered adjacent pair (min,max) of indices.
    HEdges map[[2]int]Relation // horizontal-neighbor relations
    VEdges map[[2]int]Relation // vertical-neighbor relations
    Seed   int64
    Diff   Difficulty
}
```

## Generation approach

1. **Build a full valid solution.** Backtracking fill (row by row) that respects balance + no-3-in-a-row, with random symbol order per cell. Reject partial rows/cols that already violate balance capacity or create a triplet. This is fast on 6×6.
2. **Derive candidate edges.** For every adjacent pair in the solution, note whether they're equal or different; these are the *possible* `=`/`×` clues.
3. **Carve to a minimal, uniquely-solvable clue set.** Start from the full solution and a rich set of edges + givens; **remove** givens/edges one at a time (in random order); after each removal re-run the deduction-ladder solver and keep the removal only if the puzzle is still *uniquely* solvable at or below the target difficulty. Stop when no further removal preserves uniqueness. This yields a lean puzzle.
4. **Difficulty targeting.** Bias which clues survive by the deepest technique needed. Easy = many givens, shallow ladder; hard = few givens, deeper ladder + more edge-driven chains.

## Solver approach (for uniqueness + solvability)

- Implement a **logic solver** (applies the deduction ladder to fixpoint) — used to classify difficulty and power hints.
- Implement a **complete solver** (DFS/backtracking or a small SAT/exact-cover encoding) that **counts solutions up to 2** (stop at 2). Uniqueness = exactly 1. The complete solver is the ground truth; the logic solver is the difficulty/hint oracle.
- **Cross-validation invariant:** for every generated puzzle, `logic-solver solution == complete-solver solution` and complete-solver count == 1. If the logic solver stalls but the complete solver finds a unique answer, the puzzle requires guessing → reject or raise difficulty label.

## Uniqueness & deduplication

- **Canonical form** for fingerprinting: Tango has a natural symmetry group of size up to 32 — the 8 dihedral transforms of the square (rotations/reflections) × the 2 symbol swaps (sun↔moon) × (optionally) row/col block permutations if you decide those preserve identity. Recommend: **8 dihedral × 2 symbol-swap = 16** transforms. Canonicalize by taking the lexicographically-smallest serialized `(givens+edges)` across all 16.
- Fingerprint = hash of the canonical serialization. Keep a set of seen fingerprints; reject collisions.

## TUI interaction

- **Cursor**: arrow keys / `hjkl` move a highlighted cell.
- **Cycle symbol**: `Space` or `Enter` cycles Empty → Sun → Moon → Empty on the focused cell (givens are immutable and visually distinct).
- **Direct set**: `s` sets sun, `m` sets moon, `x`/`Delete` clears.
- **Mouse**: left-click a cell cycles it; right-click clears. (LinkedIn's own convention is click=sun, click-again=moon; mirror that.)
- **Feedback**: live partial-validator underlines/reddens the specific violated line or edge; a win banner on completion.
- **Helpers**: `u` undo, `Ctrl+r` reset, `?` help, `H` hint (reveals one forced cell + which technique fired).
- Render `=`/`×` markers *between* cells (in the gutter), suns/moons as glyphs with per-symbol color from the theme.

## TDD test matrix (red tests to write first)

**Engine — validator**
- Empty board → no violations.
- A hand-built valid solution → `Valid == true`.
- Row with 4 suns → balance violation on that row only.
- `A A A` horizontal and vertical → triplet violation; `A A A` diagonal → **no** violation (guards the common bug).
- `=` pair with differing symbols → edge violation; `×` pair with equal symbols → edge violation.

**Engine — solvers**
- Golden puzzle with known unique solution → complete solver returns it; count == 1.
- Puzzle with two solutions (hand-crafted) → count == 2 (guards the "stop at 2 / uniqueness" logic).
- Logic solver solves all shipped example puzzles to completion.

**Engine — generator (property-based, run over many seeds)**
- Every generated puzzle: `Valid(solution)`, complete-solver count == 1, logic solver closes it, difficulty label matches deepest technique.
- Removing any single remaining given/edge breaks uniqueness (minimality), *if* you promise minimality.
- Fingerprints of a large batch are pairwise distinct.
- Generation p99 latency under target (e.g. < 50 ms on 6×6).

**Determinism**
- Same seed → identical puzzle (reproducibility for corpus + bug repro).

## Gotchas

- The diagonal misconception (rule 2) — assert against it explicitly.
- Edge constraints are between **orthogonal** neighbors only.
- When carving clues, always re-check uniqueness with the *complete* solver, not just the logic solver, or you can ship guess-required boards.

## References

- LinkedIn help & how-to: https://www.linkedin.com/games (Tango: "Harmonize the grid")
- Rules (row/col balance, no-3-in-a-row H/V, = / ×): https://www.thewordfinder.com/linkedin-games-hub/tango-hints/
- Rules + Takuzu/Binairo lineage, uniqueness aid: https://flipultimate.com/blog/how-to-play-tango-puzzle
- Complete rule set with worked edge examples: https://www.tango-unlimited.com/rules
- Strategy (gap/sandwich, rescan, line uniqueness): https://connectsafely.ai/articles/linkedin-tango
