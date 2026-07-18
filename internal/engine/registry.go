package engine

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"sync"
)

// Generated is the type-erased result of one Generate call, consumed by the
// headless CLI and the TUI. Puzzle and Solution hold the game's concrete types;
// only code that knows the game (its board adapter) downcasts them.
type Generated struct {
	// Puzzle is the concrete per-game puzzle value.
	Puzzle any
	// Solution is the puzzle's unique solution. Callers keep it private —
	// it powers hints and the win check, and must never leak into Encoded.
	Solution any
	// Encoded is the stable serialization of the puzzle (no solution inside).
	// It is what gets written by `lig generate` and accepted by Entry.Verify.
	Encoded []byte
	// Fingerprint is the symmetry-normalized hash used for dedup.
	Fingerprint [32]byte
}

// Entry is a game's type-erased hook into the registry. Each game package
// exposes `func Entry() engine.Entry`; wiring code registers all games in one
// place (internal/games/all).
type Entry struct {
	ID   GameID
	Name string // human-readable, e.g. "Mini Sudoku"

	// Generate produces a fresh puzzle at diff using r for all randomness.
	// It must uphold the generation invariant (valid, exactly one solution,
	// logic-solvable for the no-guess tiers) before returning.
	Generate func(diff Difficulty, r *rand.Rand) (Generated, error)

	// Verify decodes an Encoded puzzle and independently re-checks the
	// generation invariant (valid + exactly one solution). It returns nil for
	// a good puzzle and a descriptive error otherwise.
	Verify func(encoded []byte) error
}

var (
	regMu    sync.RWMutex
	registry = map[GameID]Entry{}
)

// Register adds a game to the registry. It panics on duplicate IDs or
// incomplete entries — both are wiring bugs, not runtime conditions.
func Register(e Entry) {
	if e.ID == "" || e.Name == "" || e.Generate == nil || e.Verify == nil {
		panic(fmt.Sprintf("engine: incomplete registry entry %+v", e))
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := registry[e.ID]; dup {
		panic(fmt.Sprintf("engine: duplicate game id %q", e.ID))
	}
	registry[e.ID] = e
}

// Lookup returns the entry for a game id.
func Lookup(id GameID) (Entry, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	e, ok := registry[id]
	return e, ok
}

// All returns every registered game, sorted by ID for stable output.
func All() []Entry {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Entry, 0, len(registry))
	for _, e := range registry {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
