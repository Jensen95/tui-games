# Zip — Specification

## Summary

Zip is LinkedIn's path-drawing puzzle: draw **one continuous line that visits every cell exactly once** (a Hamiltonian path), passing through the numbered cells in **ascending order**, moving only orthogonally, and never crossing **walls** (thick borders between some cells). Start on cell `1`, finish on the highest number. Exactly one valid path exists per puzzle. This is the algorithmically hardest of the five to *generate* well, because Hamiltonian-path construction with waypoint + wall constraints is the crux.

## Grid & pieces

- Rectangular grid, commonly **6×6**; support arbitrary `R×C` (LinkedIn-style clones range up to 12×12).
- Some cells carry **waypoint numbers** `1..K` (K ≥ 2). `1` is the start, `K` the end. Waypoints are ordered checkpoints, *not* one-per-cell in general — most cells are unnumbered.
- **Walls**: a wall sits on an *edge between two adjacent cells* and blocks movement across that edge. Walls are optional and increase difficulty.
- The solution is a path (ordered list of cells) that the player draws.

## Rules (precise)

A drawn path is a valid solution iff:

1. **Hamiltonian.** It visits **every** cell exactly once (no cell skipped, none revisited).
2. **Contiguous & orthogonal.** Consecutive cells in the path are edge-adjacent (up/down/left/right); no diagonals; the line never crosses itself.
3. **No wall crossings.** No step crosses a wall edge.
4. **Waypoint order.** The numbered cells are encountered in strictly increasing order: the path reaches `1` first, `2` after `1`, …, `K` last. Equivalently, `1` is an endpoint of the path and `K` is the other endpoint, and each `i` appears before `i+1` along the traversal.

## Solved-state definition (for the validator)

```
Valid(path) :=
  len(path) == R*C
∧ set(path) == all cells                      // every cell once
∧ ∀ i: adjacent(path[i], path[i+1]) ∧ ¬wall_between(path[i], path[i+1])
∧ path[0] == cell(number==1)
∧ the subsequence of numbered cells along path == [1,2,...,K] in order
```

Partial validator (live in TUI): the in-progress path must be a simple orthogonal wall-respecting chain starting at `1`, and must not violate waypoint order *so far* (e.g. can't hit `3` before `2`). It should also flag **dead-end trapping** heuristically as a hint, but trapping isn't a rule violation per se — it just makes completion impossible.

## Data model (Go sketch)

```go
type Puzzle struct {
    R, C     int
    Waypoint map[int]int // cell index -> number (1..K)
    // Walls stored as a set of blocked edges between adjacent cells.
    Walls    map[[2]int]bool // key: ordered adjacent pair (min,max) of cell indices
    Seed     int64
    Diff     Difficulty
}

type Solution struct {
    Path []int // ordered cell indices, len R*C
}
```

## Generation approach (the hard part — do this carefully)

Generate the *path first*, then derive constraints — never try to place numbers/walls and hope a path exists.

1. **Generate a random Hamiltonian path** over the R×C grid:
   - Simplest robust method: **backtracking DFS** with randomized neighbor order and a connectivity/pruning check (reject branches that strand unreachable cells; a fast "can all remaining cells still be reached?" flood-fill prune keeps it tractable on ≤ 8×8).
   - Alternative for speed at larger sizes: start from a trivial serpentine (boustrophedon) Hamiltonian path and apply many random **"backbite" moves** (a well-known Hamiltonian-path perturbation that repeatedly reconnects an endpoint) to shuffle it into a random path. This is fast and always stays Hamiltonian.
2. **Place waypoints along the path.** Put `1` at `path[0]`, `K` at `path[last]`, and choose K−2 interior indices (spread out) to receive `2..K−1` in path order. Fewer waypoints ⇒ harder.
3. **Optionally add walls** — but only on edges the solution path does **not** use, so the intended path stays legal. Walls constrain alternatives and add difficulty.
4. **Enforce uniqueness.** Run the complete solver (below): if more than one Hamiltonian path satisfies the waypoints+walls, either add a wall (on a non-solution edge) that kills an alternative, or add another waypoint, then re-test. Iterate until unique.
5. **Difficulty targeting.** Easy: more waypoints, few/no walls, path with obvious forced corners. Hard: minimal waypoints (sometimes just `1` and `K`), more walls, longer forced detours. Corners and degree-2 cells create forced moves — bias toward configurations rich in forced deductions if you want a "no-guess" experience (see below).

## Solver approach (uniqueness + solvability)

- **Complete solver:** DFS enumerating Hamiltonian paths consistent with fixed endpoints/waypoints and walls, with strong pruning (connectivity check; degree checks — a cell with only one open neighbor forces that edge; endpoints have degree 1). Count solutions **up to 2**; uniqueness == 1. Feasible for the target sizes.
- **Logic/forced-move solver (optional but recommended):** repeatedly apply *forced edges* — endpoints and any cell whose open (non-wall, unvisited) neighbors number exactly what's forced — to see whether the path can be built with no branching. If yes, the puzzle is "logic-solvable"; use its depth as difficulty. If the board can only be finished by search, label it harder or reject for the "no-guess" tier.
- **Cross-validation invariant:** complete-solver count == 1, and (for no-guess tier) the forced-move solver completes the path.

## Uniqueness & deduplication

- **Symmetry group:** 8 dihedral transforms. Additionally, a path and its **reversal** are the same *shape* but numbering makes them distinct puzzles (reversing swaps `1`↔`K`); treat numbered puzzles as directed, so reversal is a *different* puzzle unless you also renumber. For fingerprinting the *puzzle* (numbers+walls), canonicalize over the 8 dihedral transforms only (numbering fixes direction).
- Fingerprint = hash of canonical `(waypoints, walls)` layout. Reject corpus collisions.

## TUI interaction

- **Draw by keyboard:** move the path head with arrows / `hjkl`; each move extends the path into the target cell if legal (adjacent, unvisited, no wall). Moving *back* onto the previous cell erases the last segment (LinkedIn's backtrack behavior).
- **Draw by mouse (primary affordance):** **click-and-drag** from cell `1` through cells to lay the path; dragging back retraces/erases. This is the natural Zip interaction and the reason robust mouse support matters most here.
- **Erase:** click/drag back, or `u` undo one segment, `Ctrl+r` reset.
- **Rendering:** draw the path as a thick line/▓ through cell centers with rounded corners at turns; render walls as bold gutter segments; numbered cells show their number as a badge. Show remaining-cell count.
- Helpers: `?` help; `H` hint (reveal the next forced segment). Warn (non-blocking) when the current partial path has stranded an unreachable cell.

## TDD test matrix (red tests first)

**Validator**
- Full serpentine path on an open grid with `1` at start / `K` at end → valid.
- Path missing one cell → invalid (not Hamiltonian).
- Path revisiting a cell → invalid.
- Diagonal step → invalid.
- Step across a wall → invalid.
- Path that reaches `3` before `2` → waypoint-order violation.
- Path where `1` is not the first cell → invalid.

**Solvers**
- Golden puzzle (known unique path) → complete solver returns it; count == 1.
- Hand-built puzzle with two Hamiltonian paths satisfying constraints → count == 2.
- Forced-move solver completes all "no-guess" example puzzles.

**Generator (property-based over many seeds & sizes)**
- Every generated puzzle: the recorded solution is `Valid`; complete-solver count == 1.
- All walls lie on non-solution edges (invariant that keeps the intended path legal).
- Waypoint numbers form a contiguous `1..K` and appear in path order.
- No-guess tier: forced-move solver completes it.
- Fingerprints pairwise distinct across a batch; dihedral transforms of one puzzle share a fingerprint.
- Generation p99 latency under target (watch this — Hamiltonian search is the risk; budget a fallback to the backbite method if DFS latency regresses at larger sizes).

**Determinism:** same seed → identical puzzle.

## Gotchas

- **Generation performance is the project's main algorithmic risk.** Prototype the backbite-based generator early; keep the DFS generator as a correctness reference. Assign the strongest agent here.
- Walls are on **edges**, not cells — the data model and hit-testing must reflect that.
- Reversal/direction subtlety in dedup.
- Live "trapping" detection is a UX nicety, not a rule; keep it out of the hard validator.

## References

- LinkedIn help (draw single path filling every cell, walls, sequence): https://www.linkedin.com/help/linkedin/answer/a7445030
- Rules (visit every cell once, orthogonal, walls block, ascending order): https://www.thewordfinder.com/linkedin-games-hub/zip-hints/
- Waypoints/dead-ends/choke-points strategy; sizes 6×6–12×12: https://www.zipgameunlimited.com/how-to-play
- Walls as thick lines, corner-cell planning: https://www.zipgame.me/unlimited
