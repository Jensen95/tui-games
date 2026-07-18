# 05 — Agent Workflow (Fan-out, TDD Split, Cross-Validation)

This project is built by a **fan-out of coding agents** under TDD, coordinated by a lead/orchestrator that dispatches tasks rather than editing code itself. The five games are independent modules behind a shared engine contract, which makes them ideal for parallel agents that are then cross-validated.

> **Build-time only.** Agents are how the code gets *written and tested*. They are **not**
> part of the running product: the shipped binary generates puzzles with pure-Go algorithms,
> no LLM calls, fully offline (see `02-engine-and-generation.md`). Nothing in this workflow
> introduces a runtime model dependency — the agent fan-out ends when the code ships.

## Roles

| Role | Responsibility |
|---|---|
| **Orchestrator (lead)** | Owns the plan and the merge queue. Decomposes work into atomic tasks, dispatches them to agents, enforces the TDD loop and conventions, integrates branches, resolves conflicts. Does **not** write implementation code directly — it instructs agents. |
| **Red-test author** | From a `games/*.md` "red tests to write first" list (or a doc section), writes *failing* tests that pin the behavior. Commits `test: …`. |
| **Green implementer** | Makes the red tests pass with the minimum implementation. A **different agent** from the red author. Commits `feat:`/`fix: …`. |
| **Cross-validator / reviewer** | Independently reviews and, for the solver pair, *authors the second solver* so the two-solver uniqueness check is genuinely independent. Runs the property/fuzz suites, checks the dependency seam, approves the merge. |

Keeping red-author, green-implementer, and cross-validator as distinct agents is the point: it turns "the tests pass" into meaningful evidence instead of one mind grading its own homework.

## Model → task mapping (fan-out by capability)

Assign by difficulty so effort matches the problem:

- **Opus** — the hard algorithmic cores where correctness/perf is subtle:
  - Zip's Hamiltonian-path generator (backtracking + backbite) and its complete + forced-move solvers.
  - Queens' region-growing generator + uniqueness reshaping and its complete + logic solvers.
  - The exact-cover/DLX complete solvers (Patches, Sudoku) if a shared DLX is built.
  - Canonicalization/fingerprinting where the symmetry group is non-trivial (Sudoku band/stack; Queens color-agnostic).
- **Sonnet** — standard games and the TUI:
  - Tango and Mini Sudoku generators/solvers (well-trodden).
  - The Bubble Tea v2 shell, board adapters, mouse hit-testing, theming.
  - Property-test harnesses.
- **Haiku** — scaffolding and breadth:
  - Repo skeleton, `go.mod`, Makefile, CI config, lint/dependency-guard setup.
  - Boilerplate types, simple partial validators, golden-file fixtures, example puzzles, docs/READMEs.
  - Straightforward unit tests (validator truth tables) authored as the "red" side.

Right-size, don't over-assign: a simple validator doesn't need Opus; the Zip generator shouldn't go to Haiku. Cross-validation lets a cheaper model's output be verified by a stronger one where it matters.

## Parallelization plan (respect the dependency order)

Some work is **blocking** and must land before fan-out; the rest is embarrassingly parallel.

```
Phase 0 (serial, blocking):  engine interfaces + generic grid/dihedral helpers + registry
                              + repo skeleton + CI + dependency-guard
        │
        ▼
Phase 1 (PARALLEL, one track per game):   ┌─ tango ─┐
   each track runs its own TDD loop:      ├─ queens ┤
   red tests → green impl → 2nd solver    ├─ zip ───┤   ← start Zip FIRST (highest risk)
   → property/dedup tests → cross-review   ├─ patches┤
                                          └─ sudoku ┘
        │  (games depend only on the engine contract, not on each other)
        ▼
Phase 2 (parallel-ish):  TUI shell + per-game board adapters (adapters can start
                         once a game's engine + Solved()/Violations() exist)
        │
        ▼
Phase 3 (serial): integration, corpus/dedup wiring, difficulty tuning, perf/bench, polish
```

Rules that make parallelism safe:
- **Contracts first.** Nobody starts a game track until the engine interfaces are frozen (Phase 0). Interface changes after that go through the orchestrator.
- **One package per agent-track.** Each game owns `internal/games/<name>/`; no cross-writes. The TUI shell and each adapter are separate files/owners.
- **Zip goes first / gets the most attention** because its generator is the schedule risk (Hamiltonian search + perf). De-risk it early so a fallback (backbite) can be chosen before the deadline.

## The TDD loop, per atomic task

1. **Plan** — orchestrator hands the agent the exact spec section + interface + the invariant to add. One task = one small, reviewable unit.
2. **Red** — red-author agent writes failing test(s); commit on a branch.
3. **Green** — a *different* agent implements to green; commit.
4. **Refactor** — clean up, tests still green; commit.
5. **Cross-validate** — reviewer agent runs property/fuzz suites, checks the seam and conventions, and (for solvers) confirms the independent second solver agrees. Approve → merge.

## Cross-validation matrix (who checks whom)

- **Generator ↔ complete solver:** every generated puzzle is verified unique by the complete solver (written/owned by a different agent than the generator). Bug in generator → solver flags a non-unique or invalid puzzle.
- **Complete solver ↔ logic solver:** must agree on the unique solution (different authors). Bug in one → mismatch fails a test.
- **Validator ↔ brute force (tiny grids):** fuzz asserts the fast validator matches an exhaustive checker on small boards.
- **Adapter ↔ engine validator:** the TUI never re-implements rules; it calls the engine validator, so UI feedback and tests share one referee.

## Branch & commit conventions

**Branches:** `{llm}/{type}/{scope-kebab}` — the conventional-commit *type* comes right after the model (llm) specifier.

Examples:
```
opus/feat/zip-hamiltonian-generator
opus/test/queens-uniqueness-solver
sonnet/feat/tui-board-adapter-tango
sonnet/refactor/mouse-hit-testing
haiku/chore/repo-skeleton-ci
haiku/docs/games-readme
```

**Commits:** Conventional Commits, **atomic** (one logical change each), imperative mood, scoped by package/game:
```
feat(zip): add backbite-based Hamiltonian path generator
test(zip): red tests for waypoint-order validator
fix(queens): treat corner-adjacency as touching
refactor(engine): extract dihedral transform helpers
test(sudoku): box-only violation truth table
chore(ci): add depguard rule forbidding engine->tui imports
docs(patches): note strict wide/tall inequality
```

Guidelines: keep each commit independently reviewable and revertable; tests and the code they cover may land in separate commits (red vs green) but the branch as a whole must be green before merge; reference the task/issue id in the body where one exists.

## Integration protocol

- Small, frequent PRs per atomic task; the orchestrator maintains merge order = the dependency order above (engine → games → TUI → integration).
- CI must be green (tests, vet, lint, dependency-guard, benchmarks within budget) before merge.
- After each merge, re-run the full property + fuzz suites; a nightly corpus build is the backstop that catches rare generator defects with a reproducible seed.
