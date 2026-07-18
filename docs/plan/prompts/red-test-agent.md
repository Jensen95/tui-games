# Agent Prompt — Red-Test Author

Reusable task template. The orchestrator fills the `{{PLACEHOLDERS}}` and dispatches this
to a coding agent. This agent writes **failing tests only** — it does not write
implementation. A *different* agent will make the tests green.

---

## Fill before dispatch

- `{{MODEL}}` — haiku | sonnet | opus (see `docs/05-agent-workflow.md` → model→task mapping)
- `{{GAME}}` — tango | queens | zip | patches | mini-sudoku  (or `engine` for shared code)
- `{{SCOPE}}` — kebab-case unit, e.g. `waypoint-order-validator`
- `{{SPEC}}` — path + section, e.g. `games/zip.md → "TDD test matrix (red tests first)"`
- `{{BEHAVIOR}}` — the one behavior/invariant this task pins (copy the exact line from the spec)
- `{{INTERFACE}}` — the Go signature(s) under test, from `docs/02-engine-and-generation.md`
- `{{BRANCH}}` — `{{MODEL}}/test/{{GAME}}-{{SCOPE}}`

---

## Your task

You are the **red-test author** for `{{GAME}}`. Read `{{SPEC}}` in full, plus
`docs/01-architecture.md` (the pure-engine / thin-TUI seam) and
`docs/04-testing-strategy.md` (the six test layers).

Write **failing** tests that pin exactly this behavior:

> {{BEHAVIOR}}

Against this interface (do not change it — if it looks wrong, stop and report to the
orchestrator):

```go
{{INTERFACE}}
```

## Rules

1. **Tests only.** No implementation, no stubs beyond what the compiler strictly needs.
   If the package doesn't compile because the type is missing, add the *minimal* type
   declaration (empty struct / unimplemented method returning zero + `panic("todo")`)
   so the test compiles and **fails** — nothing more. The green agent owns the real body.
2. **Fail for the right reason.** Each test must fail because the behavior is absent, not
   because of a typo or a panic in setup. Run them and confirm red (assertion failure or
   the `todo` panic), then paste the failing output in your report.
3. **One behavior per test.** Small, named, table-driven where natural. Prefer many tiny
   truth-table cases over one mega-test. Name tests `Test{{Thing}}_{{condition}}`.
4. **Cover the spec's gotchas.** Every item in the game's `Gotchas` section that this task
   touches gets an explicit test (e.g. Tango: no-3-in-a-row is horizontal/vertical **only,
   never diagonal**; Queens: adjacency is **local 8-neighbor, not full chess diagonal**;
   Sudoku boxes are **2×3**; Patches Wide/Tall are **strict** inequalities).
5. **Determinism.** Any test that generates uses a fixed seed (`math/rand/v2`, seeded).
   No wall-clock, no unseeded randomness, no network, no sleeps.
6. **Property tests** (when the task is a generator/solver invariant): assert the invariant
   over many seeds, not one example. State the invariant as a comment. See
   `docs/04-testing-strategy.md` layer 2.
7. **Golden files** go under `testdata/`; generate them in a later green step, not here —
   for now assert against inline expected values so the test is self-contained and red.

## Definition of done

- New `_test.go` file(s) compile and **fail**.
- Failing output captured in your report.
- Every clause of `{{BEHAVIOR}}` and every relevant gotcha has a test.
- No implementation logic added.

## Commit & branch

- Branch: `{{BRANCH}}`
- One atomic commit (or a few, split by sub-behavior):
  ```
  test({{GAME}}): red tests for {{SCOPE}}
  ```
- Conventional Commits, imperative mood. Do not squash unrelated changes in.

## Report back to orchestrator

1. Files added.
2. The exact failing test output (proof of red).
3. Any spec ambiguity you hit and the assumption you made (flag, don't silently decide).
4. Interface mismatches, if any (do **not** "fix" the interface — report it).
