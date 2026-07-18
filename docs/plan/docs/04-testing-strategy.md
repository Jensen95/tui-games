# 04 — Testing Strategy

## Philosophy

Tests are not an afterthought here — they're how we *know* generated puzzles are correct, since no human eyeballs each one. The generator/solver/validator triangle is cross-checked so a bug can't hide. We work **TDD** (plan → red → green → refactor), and — per the team workflow — the agent that writes a failing test is generally **not** the agent that makes it pass (see `05-agent-workflow.md`).

## The test layers

### 1. Engine unit tests (per game, pure, fast)

- **Validator truth tables.** Hand-built boards exercising each rule and — importantly — each *near-miss* that should **not** trigger (Tango diagonal triple = OK; Queens same-diagonal-but-far = OK; Sudoku box-only violation). These guard the classic bugs called out in each spec's "Gotchas."
- **Solver correctness.** Golden puzzles with known unique solutions; hand-built ambiguous puzzles that must return count == 2; the logic solver must close every shipped example.
- These run in milliseconds and form the bulk of the red tests.

### 2. Property-based generator invariants (per game, over many seeds)

For N random seeds (say 1,000 in CI, more in nightly fuzz), assert for **every** generated puzzle:

- `Validator.Solved(solution)` is true.
- `Solver.CountSolutions(p, 2) == 1` (unique).
- No-guess tiers: `LogicSolve` closes it; deepest technique ∈ the requested difficulty band.
- (If minimality is promised) removing any one remaining clue breaks uniqueness.
- The recorded solution obeys every game-specific structural invariant (e.g. Zip: all walls on non-solution edges; Patches: clue numbers sum to grid area; Queens: N connected regions).

Property tests are the safety net that makes "generate on the fly" trustworthy. Use `testing/quick` or `pgregory.net/rapid`; feed the engine explicit seeds so any failure is reproducible from the printed seed.

### 3. Cross-validation tests (the two-solver check)

- `LogicSolve(p)` result == `Solve(p)` (complete solver) whenever LogicSolve closes.
- Independently: generate with game A's generator, verify with a **separately-authored** validator/solver. In the agent workflow these are written by different agents, so agreement is meaningful evidence rather than a tautology.

### 4. Canonicalization / dedup tests (per game)

- Generate a puzzle, apply **every** transform in its symmetry group (see `02-engine-and-generation.md`), assert all transformed copies share one fingerprint.
- Generate a large batch, assert fingerprints are pairwise distinct.
- Assert a deliberately transformed duplicate is *rejected* by the dedup set.

### 5. Determinism tests

- `Generate(diff, rand.New(seed))` twice with the same seed → byte-identical puzzle. This underpins reproducibility, corpus builds, and bug repro.

### 6. TUI tests

- **Mouse hit-testing (pure unit):** given a `GridGeometry`, assert coordinate→`CellRef` mapping for cell centers, borders, gutters, and out-of-bounds. No terminal needed. This is where most mouse bugs are cheaply caught.
- **Adapter input (unit):** feed `HandleKey`/`HandleMouse` sequences to a board adapter, assert board state changes (place/cycle/undo/reset/draw-rect/extend-path) and that `Violations()`/`Solved()` reflect them.
- **Golden view snapshots:** render a fixed puzzle at a fixed size, compare to a golden file (`charmbracelet/x/exp/golden` or teatest golden). Regenerate with an `-update` flag on intentional visual changes.
- **End-to-end with `teatest`:** start the program, script a full solve (keys/mouse messages) for a *fixed seed* puzzle, and assert the win banner appears; script an invalid move and assert the error styling text appears. Cover at least a happy-path solve per game.

## Coverage targets & gates

- Engine packages (`internal/engine`, `internal/games/*`): high line + branch coverage (aim 90%+); every rule and every "should-not-trigger" near-miss has a test.
- TUI: cover hit-testing and each adapter's input mapping thoroughly; golden/teatest for the happy paths.
- CI is red on: any test failure, `go vet`, lint (incl. the dependency-guard that forbids the engine importing the TUI), and generator benchmark regressions beyond the `02` budget.

## Fuzzing / nightly

- Go native fuzzing (`go test -fuzz`) on validators (random boards must never panic; validator agrees with a brute-force checker on tiny grids) and on the headless `generate`→`verify` round-trip.
- Nightly job: generate a large corpus per game, assert all unique + uniquely-solvable, and archive it. Any failure prints the offending seed.

## TDD loop in practice (per unit of work)

1. **Plan** — read the relevant `games/*.md` section; agree the interface + the invariant being added.
2. **Red** — write the failing test(s) that pin the behavior (validator rule, solver count, generator invariant, mouse mapping…). Commit `test: …` on a branch.
3. **Green** — implement the minimum to pass. Commit `feat:`/`fix: …`.
4. **Refactor** — clean up with tests green. Commit `refactor: …`.

Every game's spec already lists its "red tests to write first" — those are the starting backlog for step 2.
