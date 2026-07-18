# 00 — Overview

## What we're building

A single Go binary that plays five LinkedIn-style logic puzzles in the terminal — **Tango, Queens, Zip, Patches, Mini Sudoku** — with full keyboard **and** mouse navigation, built on the Charm **Bubble Tea v2** stack. Every game can **generate fresh puzzles on demand**, and every generated puzzle is **guaranteed valid, uniquely solvable, and de-duplicated** against previously seen puzzles.

## Goals

1. **Playable TUI** for all five games with a shared shell (menu → pick game → play → win), keyboard + mouse.
2. **On-the-fly generation** — a new puzzle in well under a second per game, at selectable difficulty.
3. **Correctness by construction** — every generated puzzle is machine-verified: valid, exactly one solution, logic-solvable (for the no-guess tiers), and not a duplicate.
4. **Solid, layered tests** for each game — the generator/solver/validator triangle is cross-checked so a bug in one component can't silently hide behind a matching bug in another.
5. **Portability** — the puzzle *engine* is pure Go with zero TUI/IO dependencies, so it can be reused later behind an Android UI (see `07-android-future.md`).

## Non-goals (for now)

- The three LinkedIn **word** games (Pinpoint, Crossclimb, Wend) — different domain (dictionaries/NLP), deliberately out of scope.
- Networked daily puzzles / streaks / accounts. This is a local, offline, unlimited generator.
- **LLM/agent-driven puzzle generation at runtime.** Agents *build* the code; they are not in the running product. All generation is deterministic pure-Go algorithms (see `02-engine-and-generation.md`), so the binary runs fully offline with no model calls and same-seed reproducibility.
- The Android app itself — only the architectural seam that makes it cheap later.
- Pixel-perfect visual cloning of LinkedIn. We follow the *rules and interactions* faithfully; visual style is our own.

## Success criteria (Definition of Done for v1)

- `go build ./...` produces one binary; `go test ./...` is green in CI.
- Each game: playable end-to-end with keyboard and mouse; generates a fresh, unique, uniquely-solvable puzzle at ≥2 difficulty levels.
- Engine test suites include validator unit tests, solver uniqueness tests, and property-based generator invariants over many seeds; TUI has golden + `teatest` interaction tests.
- A `generate --count N --game X` CLI path can emit N distinct puzzles and a `verify` path re-checks any puzzle file (useful for building a corpus and for CI fuzzing).

## Tech stack (verified current as of July 2026)

| Concern | Choice | Notes |
|---|---|---|
| Language | **Go** (latest stable) | Strong for CLI/TUI, fast solvers, easy cross-compile. |
| TUI framework | **Bubble Tea v2** | `charm.land/bubbletea/v2`. v2 shipped 2026-02-23 with breaking changes (see below). |
| Styling/layout | **Lip Gloss v2** | v2 released same day; declarative styles, adaptive colors. |
| Components | **Bubbles v2** | help, key bindings, viewport, spinner, etc. |
| TUI testing | **teatest** (`github.com/charmbracelet/x/exp/teatest`) + golden files | Plus direct `Model.Update` unit tests. |
| Mouse hit-testing | **Manual cell math** (primary); bubblezone optional | For grids, compute cell from mouse X/Y relative to grid origin — robust and dependency-free. Verify bubblezone v2 compat before adopting. |
| Property testing | Go's `testing` + a property/quickcheck helper | e.g. `testing/quick` or `pgregory.net/rapid`. |
| Randomness | `math/rand/v2`, explicitly seeded | Determinism: seed in → puzzle out. |

### Bubble Tea v2 — what changed (agents must not code against v1 from memory)

- **Import path** is now the vanity domain: `charm.land/bubbletea/v2` (was `github.com/charmbracelet/bubbletea`). Lip Gloss and Bubbles moved to `charm.land/...v2` likewise.
- **`View()` returns a `tea.View` struct**, not a `string`.
- **Keyboard API restructured:** `tea.KeyMsg` → `tea.KeyPressMsg` (and there is now `tea.KeyReleaseMsg` — useful for game input); field names changed; the space key is the named key `"space"`, not `" "`.
- **Mouse API restructured** along the same lines (message-per-action). Enable mouse reporting via program options and switch on the mouse message types.
- New **Cursed Renderer** (ncurses-based) — big perf win, no code changes needed.
- Native **clipboard (OSC52)** and **progressive keyboard enhancements** available.
- Migration is described as *mechanical, not conceptual* — the Model/Update/View architecture is unchanged.

> **Instruction to implementers:** the exact v2 symbol names/signatures evolve across v2.x patch releases. Treat the bullets above as the shape; confirm precise signatures against the current `charm.land/bubbletea/v2` docs and the v2 release notes before writing code, and pin exact versions in `go.mod`. Do **not** rely on pre-v2 tutorials.

## How to read this plan

- `games/*.md` — the five game specs (rules, data model, generation, solver, TDD matrix). The heart of the work.
- `01-architecture.md` — repo/module layout and the core/TUI seam.
- `02-engine-and-generation.md` — the shared engine interfaces every game implements, and the generation/uniqueness/dedup strategy in one place.
- `03-tui-design.md` — Bubble Tea v2 app design, navigation, components, theming.
- `04-testing-strategy.md` — the TDD approach and the test layers.
- `05-agent-workflow.md` — how to execute this with a fan-out of Claude agents under TDD, with commit/branch conventions.
- `06-roadmap.md` — phased milestones, dependency order, and task breakdown.
- `07-android-future.md` — the portability plan.
- `prompts/*.md` — reusable agent task templates (red-test author, green implementer, cross-validator).
