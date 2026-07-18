# 06 — Roadmap & Milestones

Phased, dependency-ordered. Each phase lists its tasks (candidate atomic units for agent dispatch) and an exit gate. Phases 1–2 fan out across games/agents; Phases 0 and 3 are mostly serial.

## Phase 0 — Foundations (serial, blocking)

Everything downstream depends on these; freeze the contracts before fan-out.

Tasks:
- Repo skeleton per `01-architecture.md`; `go.mod`, `Makefile` (build/test/lint/fuzz/bench/corpus), CI pipeline.
- **Dependency guard** (lint rule / CI check) forbidding `internal/engine` & `internal/games/*` from importing `internal/tui`, `charm.land/*`, or output packages.
- Engine interfaces (`Validator`, `Solver`, `Generator`, `Fingerprinter`, `Game`, `Difficulty`, `Technique`, `Violation`) — frozen.
- Generic grid helpers: indexing, neighbor iteration, the 8 **dihedral transforms**, first-appearance relabeling utility (shared by canonicalizers).
- Registry + `cmd/lig` skeleton with `--game/--difficulty/--seed` flags and both a TUI entrypoint and a headless `generate`/`verify` entrypoint (stubbed).
- Pin exact Bubble Tea v2 / Lip Gloss v2 / Bubbles v2 versions after confirming current symbols.

**Exit gate:** `go build ./...` green; dependency guard active; interfaces reviewed and frozen; a trivial stub game round-trips through the registry and both entrypoints.

## Phase 1 — Game engines (PARALLEL, one track per game)

Each track follows the TDD loop and produces: types → partial+full validator → complete solver → logic solver (second solver by a different agent) → generator (solution-first + carve/reshape) → fingerprint/canonicalization → property/dedup/determinism tests. Use each `games/*.md` "red tests to write first" as the starting backlog.

**Start Zip first** (highest algorithmic + perf risk); the others can begin in parallel immediately after Phase 0.

Per-game exit gate (identical for all five):
- Validator truth tables pass, including the "should-not-trigger" near-misses.
- Complete solver + logic solver agree; ambiguous fixtures return count == 2.
- Generator holds the **generation invariant** over ≥1,000 seeds (valid, unique, logic-solvable for no-guess tiers, difficulty label correct).
- Canonicalization test passes (all symmetry transforms → one fingerprint); batch fingerprints pairwise distinct.
- Determinism: same seed → identical puzzle.
- `generate --game X` and `verify` work headlessly for that game; generator benchmark within the `02` budget.

Suggested track ownership (see `05-agent-workflow.md`): Zip → Opus; Queens → Opus; Patches → Opus/Sonnet (DLX core Opus, rest Sonnet); Tango → Sonnet; Mini Sudoku → Sonnet; scaffolding/fixtures/simple validators → Haiku as the red side. Cross-validation/reviews by a different model than the implementer.

## Phase 2 — TUI (parallel-ish)

Tasks:
- Shell/state machine (Menu → Generating → Playing → WinSummary), resize handling, help bar, quit — Sonnet.
- Shared theme + colorblind-safe palettes; key map — Sonnet.
- **Mouse hit-testing** helper + its unit tests — Sonnet (Haiku red side).
- Per-game **board adapters** (one per game): render + key/mouse mapping + drag state machines (Zip path-drag, Patches rect-drag, Queens mark-drag). Adapters can start as soon as their game's `Solved()/Violations()` exist, so they overlap Phase 1's tail.
- Generation-in-a-Cmd + spinner.
- Golden view snapshots + `teatest` happy-path solve per game.

**Exit gate:** every game playable end-to-end with keyboard **and** mouse; live validation feedback; hints work; win flow works; golden + teatest suites green.

## Phase 3 — Integration, dedup, tuning, polish (serial)

Tasks:
- Wire the optional **corpus** (persisted fingerprints + saved puzzles); `generate --count N` consults it for cross-run dedup.
- Difficulty tuning pass per game (confirm the bands feel right; adjust carving heuristics).
- Performance/bench pass (watch Zip; choose backbite vs DFS as default based on measured p99).
- Nightly fuzz + corpus-build job in CI.
- README with build/run/keybindings; screenshots/gifs; `--help`.
- Final accessibility pass (min terminal size message, adaptive colors, non-color region cues).
- **Finalize design tokens** against the real TUI: run all three themes (grey/dark/light) in a real terminal at truecolor, 256-, 16-color, and `NO_COLOR`; tune the region palette at real cell size; settle the open accent decision; then freeze `08-theme-style-guide.md` (see its finalization checklist).

**Exit gate = v1 Definition of Done** (from `00-overview.md`): one binary; `go test ./...` green in CI; each game generates fresh, unique, uniquely-solvable puzzles at ≥2 difficulties, playable with keyboard + mouse; headless generate/verify works.

## Phase 4 — Android (future, out of scope for v1)

See `07-android-future.md`. The only Phase-0–3 obligation toward this is keeping the engine pure (enforced by the dependency guard), so Phase 4 is a UI project, not a rewrite.

## Critical path & risks

- **Critical path:** Phase 0 → Zip engine → (Zip adapter) → integration. Zip dominates risk on both correctness and latency.
- **Top risks & mitigations:**
  - *Hamiltonian generation too slow* → prototype backbite generator in Phase 1 day one; keep DFS as reference; bench-gate in CI.
  - *Bubble Tea v2 API drift vs. training data* → confirm symbols against current docs; pin versions; write a tiny spike before the full shell.
  - *Non-unique puzzles slipping through* → two independent solvers + property tests + nightly fuzz corpus; never trust a single solver.
  - *Region colors leaking into logic/dedup (Queens/Patches)* → explicit color-agnostic canonicalization tests.

## Milestone summary

| Milestone | Meaning |
|---|---|
| **M0** | Foundations frozen; stub round-trips. |
| **M1** | All five engines pass their exit gate headlessly (generate + verify, unique, tested). |
| **M2** | All five playable in the TUI with keyboard + mouse; golden/teatest green. |
| **M3** | Integration + dedup corpus + tuning + CI fuzz; **v1 DoD met**. |
| **M4** | (Future) Android UI on the reused engine. |
