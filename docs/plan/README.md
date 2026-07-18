# LinkedIn Games TUI — Implementation Plan

A complete, build-ready **plan** (not the implementation) for a single Go binary that plays
five LinkedIn-style logic puzzles in the terminal — **Tango, Queens, Zip, Patches, and
Mini Sudoku** — on the Charm **Bubble Tea v2** stack, with keyboard **and** mouse, on-the-fly
puzzle generation, and correctness guaranteed by construction.

This repository is documentation only. It is meant to be handed to a fan-out of coding
agents (or engineers) who turn it into code under TDD.

---

## TL;DR

- **Five games**, one binary, terminal UI. Word games (Pinpoint/Crossclimb/Wend) are out of scope.
- **Generate fresh puzzles on demand** — every generated puzzle is machine-verified **valid,
  uniquely solvable, and de-duplicated** against ones seen before.
- **Generation is pure Go, offline, deterministic** — seeded algorithms, **no LLM/agent calls
  at runtime**. Agents are used only to *build* the code; they are never part of the running product.
- **Pure engine / thin TUI** seam: the puzzle logic is pure Go with zero TUI or I/O
  dependencies, so it can be reused behind an **Android** UI later. TUI only for now.
- **Correctness by cross-validation:** for each game a generator, a complete solver, and an
  independent second (logic) solver are written by *different* agents, so a bug in one can't
  hide behind a matching bug in another.
- **Built by TDD fan-out:** plan → red → green → refactor, with red author, green
  implementer, and cross-validator kept as separate agents.

## Tech (verified current as of July 2026)

Go (latest stable) · **Bubble Tea v2** (`charm.land/bubbletea/v2`) · Lip Gloss v2 · Bubbles v2 ·
`teatest` + golden files for the TUI · manual cell math for mouse hit-testing (bubblezone
optional) · `math/rand/v2`, explicitly seeded, for deterministic generation.

> **Note on Bubble Tea v2:** v2 shipped 2026-02-23 and has breaking changes vs v1 — new
> import path, `View()` returns a `tea.View` struct, `KeyMsg` → `KeyPressMsg`
> (+`KeyReleaseMsg`), a restructured mouse API, and more. See `docs/00-overview.md` and
> `docs/03-tui-design.md`. **Agents must confirm exact v2 symbols against the current Charm
> docs and pin versions** before writing shell code — some of this post-dates common
> training data.

## Name resolution (please confirm)

The request listed the games as *Tango, Zip, Patces, Weeks, Mini sudoku*. Two look like
typos and were resolved by research to the closest real LinkedIn games:

- **"Patces" → Patches** (a Shikaku/rectangle-partition + shape-type puzzle).
- **"Weeks" → Queens** (the one-queen-per-row/col/region puzzle).

The whole set was then chosen as a coherent family of **grid logic** games. If either guess
is wrong (e.g. you meant a different game), say so and the relevant `games/*.md` spec gets
swapped — the architecture doesn't change.

---

## How to use this plan

1. **Read `docs/00-overview.md`** for the vision, goals, non-goals, and Definition of Done.
2. **Read `docs/01-architecture.md`** for the repo layout and the pure-engine/thin-TUI seam.
   Freeze the engine interfaces (`docs/02-engine-and-generation.md`) — this is Phase 0 and
   blocks everything else.
3. **Pick a game**, open its `games/<name>.md`, and work its **"TDD test matrix (red tests
   first)"** section top-down.
4. **Run the TDD loop** using the templates in `prompts/`: dispatch a red-test author, then a
   *different* green implementer, then a cross-validator (who also writes the independent
   second solver). See `docs/05-agent-workflow.md`.
5. **Follow the roadmap** in `docs/06-roadmap.md` — Phase 0 serial, then games in parallel
   (start **Zip first**; its generator is the schedule risk), then TUI, then integration.

## Repository map

```
README.md                         ← you are here
docs/
  00-overview.md                  vision, goals, non-goals, DoD, tech stack, BubbleTea v2 changes
  01-architecture.md              repo tree, pure-engine/thin-TUI seam, Game abstraction, headless mode
  02-engine-and-generation.md     engine interfaces, generation recipe, two-solver uniqueness, dedup, perf budget
  03-tui-design.md                BubbleTea v2 app, board adapters, keyboard+mouse nav, theming, accessibility
  04-testing-strategy.md          six test layers, coverage gates, fuzzing/nightly, the TDD loop
  05-agent-workflow.md            roles, model→task mapping, parallelization, cross-validation matrix, conventions
  06-roadmap.md                   phases, exit gates, critical path, milestones M0–M4
  07-android-future.md            how the pure engine ports to Android later (gomobile/Compose)
  08-theme-style-guide.md         grey/dark/light TUI themes — token tables + region palette (provisional starting point)
games/
  README.md                       games index + characteristics-at-a-glance table + shared vocabulary
  tango.md                        6×6 sun/moon (Takuzu/Binairo)
  queens.md                       N×N one-queen-per-row/col/region, local adjacency
  zip.md                          Hamiltonian path through numbered waypoints  ← highest generation risk
  patches.md                      Shikaku rectangles + shape types (Square/Wide/Tall/Free)
  mini-sudoku.md                  6×6 Sudoku, 2×3 boxes
prompts/
  red-test-agent.md               reusable template: write failing tests only
  green-impl-agent.md             reusable template: make the red tests pass, minimally
  cross-validation-agent.md       reusable template: independent review + second solver
```

## Conventions (enforced throughout)

- **Branches:** `{llm}/{type}/{scope}` — conventional-commit *type* comes right after the
  model specifier, e.g. `opus/feat/zip-hamiltonian-generator`, `haiku/chore/repo-skeleton-ci`.
- **Commits:** Conventional Commits, **atomic**, imperative mood, scoped by package/game,
  e.g. `feat(zip): add backbite-based Hamiltonian path generator`.
- **Agents don't grade their own homework:** red author ≠ green implementer ≠ cross-validator.

## The five games at a glance

| Game | Grid | Core idea | Gotcha to not get wrong |
|---|---|---|---|
| **Tango** | 6×6 | Balance suns/moons; no three in a row | "Three in a row" is horizontal/vertical **only — never diagonal** |
| **Queens** | N×N | One queen per row, column, and color region | Adjacency is **local 8-neighbor**, not full chess diagonal |
| **Zip** | grid | One continuous path visiting every cell, through numbers in order | It's a **Hamiltonian path** — hardest to generate; de-risk first |
| **Patches** | grid | Partition into rectangles, one per numbered clue (area + shape) | Wide/Tall are **strict** inequalities; Square is w==h |
| **Mini Sudoku** | 6×6 | Standard Sudoku with digits 1–6 | Boxes are **2×3** (2 rows × 3 cols), not 3×2 or 2×2 |

See `games/README.md` for the fuller comparison and shared engine vocabulary.
