# Agent Prompt — Cross-Validator / Reviewer

Reusable task template. The orchestrator fills the `{{PLACEHOLDERS}}` and dispatches this
to a coding agent. This agent **independently verifies** another agent's work and, for the
solver pair, **authors the second solver** so the uniqueness check is genuinely independent.
It must be a **different agent** from both the red author and the green implementer.

---

## Fill before dispatch

- `{{MODEL}}` — usually the strongest model for the game's core (opus for zip/queens/DLX/canonicalization)
- `{{GAME}}` — tango | queens | zip | patches | mini-sudoku  (or `engine`)
- `{{SCOPE}}` — what's being validated, e.g. `generator-uniqueness` / `second-solver` / `dedup-canonicalization`
- `{{TARGET_BRANCH}}` — the merged/candidate branch to review
- `{{SPEC}}` — path + section for the invariant being cross-checked
- `{{BRANCH}}` — `{{MODEL}}/test/{{GAME}}-{{SCOPE}}-crosscheck`

---

## Your task

You are the **independent reviewer** for `{{GAME}} / {{SCOPE}}`. You did not write the
generator, the primary solver, or the original tests. Your job is to make "the tests pass"
mean something. Read `{{SPEC}}`, `docs/04-testing-strategy.md` (esp. layer 3
cross-validation and layer 4 dedup), and `docs/05-agent-workflow.md`
(the cross-validation matrix).

Do the applicable items below for this task.

### A. Author the *second* solver (when `{{SCOPE}}` involves uniqueness)

The uniqueness guarantee rests on **two independently written solvers agreeing**. If the
green implementer wrote the complete (exact-cover/DLX or full-backtracking) solver, you
write the **logic/deduction** solver from the spec's deduction ladder — or vice versa. Same
public contract, different internals, no peeking at their implementation beyond the interface.

Then wire the cross-check test:
- For **every** generated puzzle over many seeds: both solvers return the **same** unique
  solution; the complete solver confirms the solution count is exactly 1.
- A generator bug now surfaces as a mismatch or a non-unique flag — a real signal.

### B. Validator ↔ brute force (tiny grids)

Add/confirm a fuzz test asserting the fast validator agrees with an exhaustive brute-force
checker on small boards. See `docs/04-testing-strategy.md` layer 3.

### C. Dedup / canonicalization (when `{{SCOPE}}` is dedup)

Verify the fingerprint is invariant under the game's full symmetry group and distinguishes
genuinely different puzzles:
- Tango — dihedral (D4) + sun/moon swap.
- Queens — dihedral + **color-agnostic** region relabeling.
- Sudoku — dihedral + band/stack permutations + digit relabeling (per `games/mini-sudoku.md`).
- Patches — dihedral; shape types transform correctly under rotation (Wide↔Tall).
Assert: transformed puzzle ⇒ same fingerprint; a hand-picked different puzzle ⇒ different
fingerprint. (See the per-game dedup table in `docs/02-engine-and-generation.md`.)

### D. Review the seam, conventions, and gotchas

- Engine purity: no TUI imports in `engine`/`games`, no unseeded randomness, no I/O; the
  dependency-guard passes.
- The spec's gotchas are actually encoded (Tango diagonal exemption, Queens local
  8-neighbor adjacency, Sudoku 2×3 boxes, Patches strict Wide/Tall).
- Determinism: re-run the generator with a fixed seed twice ⇒ identical output.
- Commit/branch conventions followed (`docs/05-agent-workflow.md`).

## Rules

1. **Independence is the whole point.** Don't copy the implementation you're checking. If
   you must read it to review, don't mirror its structure in your second solver.
2. **Reproduce before you approve.** Run the full property + fuzz suites yourself; paste
   output. If something is flaky, find the seed and report it.
3. **You may add tests; you don't rewrite their impl.** If you find a bug, file it back to
   the orchestrator with a **failing test that reproduces it** (that test becomes the next
   red task for the original author). Don't silently patch their code.

## Definition of done

- Second solver (if applicable) written independently and agreeing with the first on all
  seeds tested; solution count asserted == 1.
- Cross-validation, brute-force, dedup, and determinism checks present and green.
- Seam + conventions + gotchas verified.
- Explicit approve/reject with evidence.

## Commit & branch

- Branch: `{{BRANCH}}`
- Atomic Conventional Commits:
  ```
  test({{GAME}}): independent second solver + cross-check for {{SCOPE}}
  test({{GAME}}): dedup invariance under symmetry group
  ```
- If you found and reproduced a defect, the reproducing test may land here as red, tagged
  for the original author to fix.

## Report back to orchestrator

1. What you verified and how (commands + pasted output).
2. Second-solver result: agree/disagree across N seeds; any counterexample seed.
3. Defects found, each with a reproducing test and a suggested owner.
4. **Approve / Reject** for merge, with the one-line reason.
