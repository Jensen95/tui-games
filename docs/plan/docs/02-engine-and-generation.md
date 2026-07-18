# 02 — Engine, Generation, Uniqueness & Deduplication

This is the shared contract every game implements, plus the cross-cutting strategy for generating puzzles that are **valid, uniquely solvable, and non-duplicate**. Per-game algorithm details live in each `games/*.md`; this doc is the common frame they all fit.

> **Hard constraint — runtime generation is pure Go, no LLM.** Everything here is
> deterministic algorithmic code: solution-first construction, backtracking / exact-cover
> (DLX) / region-growing, seeded `math/rand/v2`. The shipped binary generates puzzles with
> **zero LLM/agent calls and no network dependency** — feed it a seed, get a puzzle. LLM
> agents appear only during *development* (writing and testing this code, per
> `05-agent-workflow.md`); they are never part of the running product. A `Generate` that
> reached for a model would break both determinism (same seed ⇒ same puzzle) and the
> offline-by-design goal, and is out of bounds.

## The interfaces

Keep them minimal. Concrete puzzle/solution types are per-game; the engine passes them as `any`/generics or via small typed wrappers. A generics-based shape (Go supports this cleanly):

```go
// Difficulty is shared across games.
type Difficulty uint8
const (Easy Difficulty = iota; Medium; Hard; Expert)

// A Puzzle is whatever a game needs; it must be serializable + fingerprintable.
type Puzzle interface {
    GameID() GameID
    Difficulty() Difficulty
    Seed() int64
}

// Validator referees a board state (partial or complete).
type Validator[B any] interface {
    // Violations returns all currently-violated rules for board b.
    // For an empty/partial board it must return only already-broken rules.
    Violations(b B) []Violation
    // Solved reports whether b is a complete, valid solution.
    Solved(b B) bool
}

// Solver finds solutions and — crucially — counts them (up to a cap).
type Solver[P Puzzle, S any] interface {
    // Solve returns one solution if any.
    Solve(p P) (S, bool)
    // CountSolutions returns min(#solutions, cap). Uniqueness == (cap>=2 && result==1).
    CountSolutions(p P, cap int) int
    // LogicSolve attempts a no-guess solve; returns the solution, whether it
    // fully closed, and the deepest technique used (for difficulty labeling/hints).
    LogicSolve(p P) (S, bool, Technique)
}

// Generator produces fresh puzzles.
type Generator[P Puzzle, S any] interface {
    // Generate returns a puzzle guaranteed valid + uniquely solvable at ~diff,
    // together with its solution (kept private by callers, used for hints/win-check).
    Generate(diff Difficulty, r *rand.Rand) (P, S, error)
}

// Fingerprinter canonicalizes a puzzle for dedup.
type Fingerprinter[P Puzzle] interface {
    // Canonical returns a stable, symmetry-normalized byte serialization.
    Canonical(p P) []byte
    // Fingerprint is a hash of Canonical (e.g. xxhash/sha256 of it).
    Fingerprint(p P) [32]byte
}

// A Game bundles them for the registry + TUI.
type Game interface {
    ID() GameID
    Name() string
    // The registry stores these as type-erased adapters; each game provides
    // constructors returning its concrete Validator/Solver/Generator/Fingerprinter.
}
```

> Implementation note: Go generics + a small type-erased registry entry per game (holding closures like `Generate(diff) (Puzzle, error)` and `Solved(board) bool`) keeps the TUI generic while each game stays strongly typed internally. Don't over-engineer the abstraction; five games is few enough that a little duplication beats a leaky generic.

## The generation invariant (identical for all five games)

Every `Generate` call must produce a puzzle `P` for which **all** of these hold — and this is enforced *inside* `Generate` before it returns (generate-and-verify loop), and again by property tests:

1. `Validator.Solved(solution) == true` — the recorded solution actually satisfies the rules.
2. `Solver.CountSolutions(P, cap=2) == 1` — **exactly one** solution. This is the non-negotiable "no ambiguous puzzles" guarantee.
3. For the no-guess difficulty tiers: `Solver.LogicSolve(P)` fully closes the board, and its deepest `Technique` matches the requested `Difficulty` band.
4. `Fingerprint(P)` is not already in the caller-supplied "seen" set (dedup).

If any check fails, `Generate` retries (new random choices / new seed) up to a bounded number of attempts, then returns an error (which in practice should never fire for correctly-tuned generators at these sizes).

### The universal generation recipe

The same three-step skeleton works for all five (details differ per game):

1. **Solution-first.** Construct a *complete valid solution* directly (fill/partition/path). Never place clues speculatively and hope a solution exists.
2. **Derive & carve clues.** Read the clue candidates off the solution, then **remove** clues while re-checking uniqueness after each removal, targeting the difficulty band. (For Queens/Zip, "carving" is instead "reshape regions / add walls" to force uniqueness — same idea: adjust constraints until exactly one solution remains.)
3. **Verify, fingerprint, dedup.** Run the generation invariant; on pass, record the fingerprint; on dup/fail, retry.

## Uniqueness: why two solvers

Every game ships **two** solvers and they check each other:

- The **complete solver** (backtracking / DLX / SAT-style) is *ground truth* for existence and count. It's what certifies uniqueness.
- The **logic solver** is the *human model*: it proves the puzzle is solvable by a defined ladder of no-guess techniques, labels difficulty, and powers hints.

**Cross-validation invariant (tested):** `LogicSolve` output, when it closes, must equal the complete solver's unique solution. If the complete solver says "unique" but the logic solver stalls, the puzzle needs guessing → it's either rejected or bumped to an "Expert (search)" tier that we don't advertise as no-guess. This mutual check is deliberate: a single solver that's subtly wrong could both generate and "verify" bad puzzles; two independently-authored solvers make that failure mode loud. (In the agent workflow, have *different agents* write the two solvers — see `05-agent-workflow.md`.)

## Solvability tiers

| Tier | Guarantee |
|---|---|
| Easy / Medium / Hard | Fully solvable by the logic ladder (no guessing). Difficulty = deepest technique needed. |
| Expert (optional) | Unique solution guaranteed, but may require limited search/bifurcation. Clearly labeled; off by default. |

## Deduplication

Two levels:

1. **Within a batch / session (in-memory).** The `Generate` caller keeps a `map[[32]byte]struct{}` of fingerprints and rejects collisions. Cheap and always on.
2. **Across runs (optional corpus).** `internal/corpus` persists fingerprints (and optionally the puzzles) to disk (e.g. a simple file or embedded KV). `generate --count N` and CI corpus-building consult it so we can amass a large library of provably-distinct puzzles.

### Canonicalization = the heart of dedup

Two puzzles are "the same" if one is a symmetry transform of the other. Each game defines its symmetry group (see its spec); the fingerprint is the hash of the **lexicographically-smallest serialization across all transforms**. Summary:

| Game | Transform group for canonicalization |
|---|---|
| Tango | 8 dihedral × 2 symbol-swap (sun↔moon) = 16 |
| Queens | 8 dihedral, with region labels normalized by first-appearance (color-agnostic) |
| Zip | 8 dihedral (numbering fixes path direction) |
| Patches | 8 dihedral (colors cosmetic → excluded) |
| Mini Sudoku | 8 dihedral × digit relabel × band/stack permutations (use full or a documented subset) |

**Test the canonicalizer directly:** generate a puzzle, apply each transform in its group, assert every transformed copy produces the *same* fingerprint (see each spec's TDD matrix). This is the property that makes dedup meaningful.

## Difficulty labeling

Difficulty is an **output** of generation, derived from the logic solver's deepest technique, not a guessed input. `Generate(diff)` biases the carving so the deepest-needed technique lands in the requested band, then *confirms* the label with `LogicSolve` before returning. This keeps "Easy" honestly easy.

## Performance budget (per generated puzzle, single core)

Targets to hold via benchmarks in CI:

| Game | Target p99 | Risk |
|---|---|---|
| Mini Sudoku | < 5 ms | none |
| Tango | < 20 ms | low |
| Patches | < 30 ms | low–med (exact cover) |
| Queens | < 50 ms | med (region reshape loop) |
| Zip | < 150 ms (≤8×8) | **high — Hamiltonian search**; have the backbite generator as the fast path |

If a target regresses, the headless `generate` benchmark in CI catches it. Zip is the one to watch; prototype its generator first (see roadmap).
