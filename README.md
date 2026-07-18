# tui-games — LinkedIn-style logic puzzles in your terminal

One Go binary, five grid-logic puzzles — **Tango, Queens, Zip, Patches, Mini
Sudoku** — playable with keyboard and mouse on the Charm [Bubble Tea
v2](https://charm.land) stack. Every puzzle is generated on the fly, offline
and deterministically (seed in → puzzle out), and is machine-verified to be
valid, **uniquely solvable**, and de-duplicated before you ever see it.

The full build-ready plan lives in [`docs/plan/`](docs/plan/README.md) —
architecture, engine contracts, per-game specs, testing strategy, and the
agent workflow used to build this repo.

## Status

- [x] **Phase 0 — Foundations**: engine contracts frozen, registry, grid +
      dihedral helpers, headless CLI, CI/depguard/release pipelines.
- [x] **Phase 1 — Game engines** (parallel, TDD): validator → solvers ×2 →
      generator → fingerprint, per game. All five engines generate, verify,
      and dedup headlessly (M1).
- [x] **Phase 2 — TUI**: Bubble Tea v2 shell + per-game board adapters,
      keyboard + mouse. Two-handed scheme: `wasd` moves, `Space` primary,
      `Shift+Space`/`Shift+digit` secondary (Kitty-protocol terminals),
      with plain-key fallbacks everywhere (M2).
- [ ] **Phase 3 — Integration**: corpus dedup, difficulty tuning, perf, polish.
- [ ] **Phase 4 — Android** (future): reuse the pure engine behind a Compose UI.

## Build & run

Dev tasks use [Task](https://taskfile.dev)
(`go install github.com/go-task/task/v3/cmd/task@latest`).

```sh
task build        # → ./lig
task run          # build + launch the TUI
./lig             # interactive TUI
./lig games       # list game engines
./lig generate --game zip --difficulty hard --count 5 --seed 42 --out puzzles/
./lig verify puzzles/*.json
```

## Development

```sh
task lint         # gofmt + go vet + depguard (engine must not import TUI/os)
task test         # unit + property tests (LIG_SEEDS=250 default)
task race         # with the race detector
task test:ci      # exactly what the CI test job runs
task nightly      # heavy seeds + fuzzing (what the nightly CI job runs)
task bench        # generator perf budgets (see docs/plan/docs/02-*.md)
task --list       # everything else
```

### Architecture in one paragraph

`internal/engine` holds the frozen game-agnostic contracts (Validator, Solver,
Generator, Fingerprinter, registry) and is **pure Go** — no TUI, no I/O, no
`os`, enforced by `scripts/depguard.sh` in CI. Each game lives in
`internal/games/<name>` and implements those contracts; `internal/games/all`
registers them. `internal/tui` (Bubble Tea v2) and the headless
`lig generate`/`lig verify` CLI are two thin consumers of the same engine —
which is also the seam that later makes an Android port a UI project instead of
a rewrite. Details: [`docs/plan/docs/01-architecture.md`](docs/plan/docs/01-architecture.md).

### Testing philosophy

Nobody eyeballs generated puzzles, so tests are the referee: validator truth
tables (including near-misses that must **not** trigger), two independently
authored solvers that must agree, property tests over many seeds asserting the
generation invariant (valid + exactly one solution + logic-solvable +
deduplicated), canonicalization tests across each game's full symmetry group,
and golden/`teatest` coverage for the TUI. Property-test seed count is tunable
via `LIG_SEEDS` (CI: 250, nightly: 5000).

## CI & releases

- **CI** (`.github/workflows/ci.yml`): gofmt, vet, depguard, `go test -race`
  with coverage, build + headless generate/verify smoke test. Runs on pushes
  to `master` and all PRs.
- **Nightly** (`nightly.yml`): heavy-seed property tests, native fuzzing,
  corpus build.
- **Release** (`release.yml`): pushing a `v*` tag builds
  linux/darwin/windows (amd64+arm64) binaries and publishes a GitHub release
  with checksums.
