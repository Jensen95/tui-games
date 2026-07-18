# 01 — Architecture

## Guiding principle: a pure engine with a thin TUI on top

The single most important architectural decision is a hard seam between:

- **`internal/engine` (and per-game packages)** — pure Go. No Bubble Tea, no `os`, no I/O, no terminal, no color. Just types, rules, generators, solvers. This is what gets reused on Android later, what property tests hammer, and what different agents can build in parallel without colliding.
- **`internal/tui`** — Bubble Tea v2 models/views. Depends on the engine; the engine never depends on it.

Everything else follows from keeping that seam clean.

## Repository layout

A single Go module (monorepo-style within one repo). Suggested tree:

```
linkedin-games-tui/
├── go.mod                      # module github.com/<org>/linkedin-games-tui
├── go.sum
├── Makefile                    # build, test, lint, fuzz, generate-corpus targets
├── README.md
├── cmd/
│   └── lig/                    # the binary (name it what you like)
│       └── main.go             # wires flags -> either TUI or headless generate/verify
├── internal/
│   ├── engine/                 # shared, game-agnostic contracts + helpers
│   │   ├── engine.go           # Game, Generator, Solver, Validator interfaces
│   │   ├── grid.go             # generic grid helpers (indexing, neighbors, dihedral transforms)
│   │   ├── difficulty.go       # Difficulty enum + labeling helpers
│   │   ├── registry.go         # registry mapping GameID -> Game implementation
│   │   └── fingerprint.go      # canonicalization + hashing scaffolding
│   ├── games/
│   │   ├── tango/              # types.go, rules.go, generate.go, solve.go, fingerprint.go, *_test.go
│   │   ├── queens/
│   │   ├── zip/
│   │   ├── patches/
│   │   └── minisudoku/
│   ├── tui/
│   │   ├── app.go              # root model: screen state machine
│   │   ├── menu.go             # main menu / game picker
│   │   ├── gameview.go         # generic game screen hosting a per-game "board renderer + input mapper"
│   │   ├── theme.go            # Lip Gloss v2 styles, palettes (colorblind-safe)
│   │   ├── keymap.go           # shared key bindings (bubbles/key)
│   │   ├── mouse.go            # grid hit-testing helpers
│   │   └── boards/             # per-game view/input adapters (thin: translate engine <-> screen)
│   │       ├── tango.go ...
│   └── corpus/                 # optional: persisted seen-fingerprints + saved puzzles
│       └── store.go
├── testdata/                   # golden files, example puzzles per game
└── docs/                       # this plan (and living design docs)
```

### Why one module, not many

Simpler dependency management and CI for a small team; the `internal/engine` ↔ `internal/tui` package boundary already enforces the important seam. If Android later needs the engine as its own module, promote `internal/engine` + `internal/games` to a separate published module (`.../engine`) — the import seam makes that a mechanical lift.

## The core abstraction: a `Game`

Each game is a value implementing a small interface (full signatures in `02-engine-and-generation.md`). The TUI and the headless CLI both talk only to this interface plus a per-game *view adapter*. Adding a sixth game = implement the interface + a view adapter + register it. Nothing else changes.

```
        ┌────────────────────────────────────────────┐
        │                 cmd/lig                      │
        │   parse flags → TUI mode | headless mode     │
        └───────────────┬───────────────┬──────────────┘
                        │               │
                 internal/tui     headless generate/verify
                        │               │
                        ▼               ▼
              ┌───────────────────────────────────┐
              │        internal/engine            │
              │  Game / Generator / Solver /      │
              │  Validator / Fingerprint          │
              └───────────────┬───────────────────┘
                              │ implemented by
       ┌──────────┬───────────┼───────────┬───────────┐
       ▼          ▼           ▼           ▼           ▼
     tango      queens       zip       patches     minisudoku   (pure Go)
```

## Data flow at runtime (TUI)

1. Menu model lets the user pick a game + difficulty.
2. TUI calls `engine.Registry[gameID].Generate(difficulty, seed)` → a `Puzzle` (+ its known solution, kept private for hints/win-check).
3. The per-game **view adapter** renders the puzzle via Lip Gloss and maps key/mouse messages to engine *moves* (place symbol, draw rect, extend path…).
4. After each move the adapter calls the engine **partial validator** for live feedback and checks the win condition.
5. On win, show stats; offer "new puzzle" (regenerate) or back to menu.

Generation can run in a Bubble Tea `Cmd` (off the update loop) with a spinner so the UI never blocks — relevant mainly for Zip at larger sizes.

## Headless mode (same engine, no TUI)

`cmd/lig generate --game zip --difficulty hard --count 100 --out corpus/zip/` and `cmd/lig verify <file>` exercise the engine with **zero** TUI dependencies. This path is gold for CI (fuzz generation nightly), for building a de-duplicated corpus, and for reproducing bugs from a seed. It also proves the engine is genuinely TUI-independent (if it weren't, this wouldn't compile).

## Dependency & determinism rules (enforced in review/CI)

- `internal/engine` and `internal/games/*` must not import `internal/tui`, `charm.land/*`, `os`, or `fmt`-for-output. Add a CI check (e.g. a small `go list` dependency assertion or an `depguard` lint rule).
- All randomness flows through an injected, explicitly-seeded `*rand.Rand` (`math/rand/v2`). No global rand in the engine. Same seed ⇒ identical puzzle, always.
- Engine functions are pure where possible: no time, no environment reads.

## Concurrency

- The engine is used single-threaded per call but is safe to call from many goroutines if each call gets its own `*rand.Rand` (no shared mutable state). This lets `generate --count N` fan out across CPU cores, and lets the TUI generate in a background `Cmd`.
