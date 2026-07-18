# Game Specifications

This directory contains one detailed spec per game. Each spec is written to be **handed directly to an implementation agent** and is the single source of truth for that game's rules, data model, generation/solver approach, and TDD test matrix.

The five games were chosen from LinkedIn's daily puzzle lineup (which as of mid-2026 also includes the word games Pinpoint, Crossclimb, and Wend — deliberately excluded here). All five are **grid-based constraint/logic puzzles**, which is what makes them a clean, coherent set: they share a grid substrate, they all reduce to constraint satisfaction, and they can all be *generated*, *machine-solved*, *checked for a unique solution*, and *deduplicated* by the same engine contracts.

> ⚠️ **Name resolution.** The original request listed "Patces" and "Weeks". These were read as **Patches** and **Queens** respectively — the two grid-logic games in the LinkedIn lineup whose names those most plausibly garble, and the choice that keeps all five in the same (grid-logic) family. If either was meant to be a different game (e.g. Wend, Pinpoint), swap the corresponding spec file — the engine contracts in `docs/02-engine-and-generation.md` are game-agnostic.

## Characteristics at a glance

| Game | Puzzle family | Default grid | Cell content | Core constraint | Solve type | Generation difficulty |
|---|---|---|---|---|---|---|
| **Tango** | Takuzu / Binairo (+ = / × edges) | 6×6 | Sun / Moon (binary) | Balanced rows+cols, no 3-in-a-row, edge relations | Deductive, unique | Low–Med |
| **Queens** | N-Queens + colored regions | 5×5 … 11×11 (varies) | Empty / Queen | 1 queen per row, col, region; no touching (incl. diagonal-adjacent) | Deductive, unique | Med |
| **Zip** | Hamiltonian path + ordered waypoints | 6×6 (up to larger) | Path segments | Single path visiting every cell once, waypoints in order, walls block | Path/backtracking, unique | **High** |
| **Patches** | Shikaku / Rectangles (+ shape type) | 5×5 (LinkedIn); 4×4–7×7 clones | Rectangle regions | Tile grid with rectangles, one clue each, area = number, shape type matches | Deductive, unique | Med |
| **Mini Sudoku** | 6×6 Sudoku | 6×6, 2×3 boxes | Digit 1–6 | Latin square + box constraint | Deductive, unique | Low–Med |

## What "the engine" must do for every game

Every game implements the same four capabilities (full interface definitions in `docs/02-engine-and-generation.md`):

1. **Validate** — given a (partial or full) board, report rule violations. This is the referee used by the TUI *and* by tests.
2. **Solve** — find a solution (and, critically, determine whether it is **unique**). Uniqueness is the linchpin of "no ambiguous puzzles."
3. **Generate** — produce a fresh puzzle at a requested difficulty, guaranteed valid + uniquely solvable, ideally in well under a second.
4. **Fingerprint** — produce a canonical, symmetry-normalized hash so a generated puzzle can be checked against a corpus for duplicates.

## Shared vocabulary

- **Given / clue** — a pre-filled cell or annotation the player cannot change.
- **Candidate** — a value that could legally go in a cell given current constraints.
- **Forced move** — a cell with exactly one candidate. A puzzle is "logic-solvable without guessing" if it can be solved by repeatedly applying forced moves and a defined ladder of deduction techniques.
- **Unique solution** — exactly one full board satisfies all constraints. This is a hard requirement for every generated puzzle.
- **Symmetry group** — the set of transforms (rotations, reflections, and game-specific relabelings) under which two puzzles are considered "the same" for deduplication.

See each game file for the specifics.
