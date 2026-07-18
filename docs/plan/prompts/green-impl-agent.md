# Agent Prompt — Green Implementer

Reusable task template. The orchestrator fills the `{{PLACEHOLDERS}}` and dispatches this
to a coding agent. This agent makes the **existing red tests pass** with the minimum
implementation. It must be a **different agent** from the one that wrote the tests.

---

## Fill before dispatch

- `{{MODEL}}` — haiku | sonnet | opus (see `docs/05-agent-workflow.md` → model→task mapping)
- `{{GAME}}` — tango | queens | zip | patches | mini-sudoku  (or `engine`)
- `{{SCOPE}}` — kebab-case unit, matching the red task's scope
- `{{SPEC}}` — path + section, e.g. `games/zip.md → "Generation approach"` / `"Solver approach"`
- `{{RED_BRANCH}}` — the branch that holds the failing tests
- `{{INTERFACE}}` — the Go signature(s) to implement (unchanged from the red task)
- `{{BRANCH}}` — `{{MODEL}}/feat/{{GAME}}-{{SCOPE}}`  (use `fix` instead of `feat` for bug tasks)

---

## Your task

You are the **green implementer** for `{{GAME}}`. Start from `{{RED_BRANCH}}`, which
contains failing tests you did **not** write. Read those tests first — they are the
specification of what "done" means. Then read `{{SPEC}}`, plus
`docs/01-architecture.md` and `docs/02-engine-and-generation.md`.

Implement this interface so the red tests turn green:

```go
{{INTERFACE}}
```

## Rules

1. **Minimum to green.** Write the simplest correct implementation that passes the tests.
   Do not add features, flags, or abstractions the tests don't demand. YAGNI.
2. **Do not edit the tests to make them pass.** If a test looks wrong, stop and report to
   the orchestrator; do not weaken or delete it. (Fixing an obvious compile typo in a test
   is fine; changing an assertion is not.)
3. **Respect the seam.** Engine code stays pure: deterministic, no I/O, no Bubble Tea
   imports, no globals. Randomness only via an injected/seeded `math/rand/v2` source. The
   dependency-guard (see `docs/01-architecture.md`) forbids `engine`/`games` importing the
   TUI — do not break it.
4. **Honor the spec's algorithm notes**, especially:
   - **Zip** — generator is the schedule risk: implement backtracking Hamiltonian search
     with the backbite fallback per `games/zip.md`; keep the perf budget in
     `docs/02-engine-and-generation.md` in mind. If you can't hit budget, report it early.
   - **Queens / Patches / Sudoku** — follow the stated solver strategy (logic solver +
     exact-cover/DLX complete solver). Don't invent a different contract.
   - Encode the gotchas correctly (Tango diagonal exemption, Queens local adjacency,
     Sudoku 2×3 boxes, Patches strict Wide/Tall).
5. **Determinism.** Same seed ⇒ same puzzle, every run, every platform. No maps-in-iteration
   ordering leaks into output; sort where order matters.
6. **Refactor step.** Once green, clean up (names, dead code, small extractions) with tests
   still green. Keep it in separate commits from the feature commit.

## Definition of done

- All red tests from `{{RED_BRANCH}}` pass.
- `go test ./...`, `go vet ./...`, and lint are clean for the touched package.
- Coverage for the package meets the target in `docs/04-testing-strategy.md`.
- No new dependencies without orchestrator sign-off; if added, they're pinned and justified.
- Seam intact (dependency-guard passes).

## Commit & branch

- Branch: `{{BRANCH}}` (based on `{{RED_BRANCH}}` or rebased on it — coordinate with orchestrator).
- Atomic Conventional Commits, imperative mood:
  ```
  feat({{GAME}}): implement {{SCOPE}}
  refactor({{GAME}}): extract helpers for {{SCOPE}}
  ```
- Red (tests) and green (impl) may live in separate commits, but the branch as a whole
  must be green before you hand it back.

## Report back to orchestrator

1. Files changed.
2. Test run showing all previously-red tests now pass (paste output).
3. Coverage number for the package.
4. Any perf/budget concern (Zip especially), any spec ambiguity + assumption made, any
   dependency you felt you needed and why.
