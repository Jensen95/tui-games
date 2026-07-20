package queens

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"

	"github.com/Jensen95/tui-games/internal/engine"
)

// wire is the stable on-disk encoding of a Queens puzzle. It carries clues
// only — the region coloring, optional givens, and metadata — never the
// solution, which must never leak into an encoded puzzle.
type wire struct {
	Game       string `json:"game"`
	N          int    `json:"n"`
	Region     []int  `json:"region"`
	Givens     []int  `json:"givens,omitempty"`
	Seed       int64  `json:"seed"`
	Difficulty string `json:"difficulty"`
}

// Encode serializes p to its stable JSON form (clues only, no solution).
func Encode(p Puzzle) ([]byte, error) {
	w := wire{
		Game:       string(GameID),
		N:          p.N,
		Region:     p.Region,
		Givens:     p.Givens,
		Seed:       p.SeedV,
		Difficulty: p.DiffV.String(),
	}
	return json.Marshal(w)
}

// Decode parses a puzzle previously produced by Encode.
func Decode(data []byte) (Puzzle, error) {
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return Puzzle{}, fmt.Errorf("queens: decode: %w", err)
	}
	if w.Game != string(GameID) {
		return Puzzle{}, fmt.Errorf("queens: decode: game %q, want %q", w.Game, GameID)
	}
	diff, err := engine.ParseDifficulty(w.Difficulty)
	if err != nil {
		return Puzzle{}, fmt.Errorf("queens: decode: %w", err)
	}
	return Puzzle{
		N:      w.N,
		Region: w.Region,
		Givens: w.Givens,
		SeedV:  w.Seed,
		DiffV:  diff,
	}, nil
}

// structurallyValid checks a decoded puzzle's shape independent of solving:
// N in the supported range (minN..maxN, i.e. 5..11 — the union of the
// per-difficulty bands in generator.go), a region id per cell, exactly N
// connected regions labeled 0..N-1, and any givens in bounds.
func structurallyValid(p Puzzle) error {
	if p.N < minN || p.N > maxN {
		return fmt.Errorf("queens: N=%d out of range %d..%d", p.N, minN, maxN)
	}
	if len(p.Region) != p.N*p.N {
		return fmt.Errorf("queens: region length %d, want %d", len(p.Region), p.N*p.N)
	}
	seen := make([]bool, p.N)
	for _, id := range p.Region {
		if id < 0 || id >= p.N {
			return fmt.Errorf("queens: region id %d out of range 0..%d", id, p.N-1)
		}
		seen[id] = true
	}
	for id, ok := range seen {
		if !ok {
			return fmt.Errorf("queens: region id %d unused (need exactly N regions)", id)
		}
	}
	if !allRegionsConnected(p.N, p.Region) {
		return fmt.Errorf("queens: a region is not 4-connected")
	}
	for _, g := range p.Givens {
		if g < 0 || g >= p.N*p.N {
			return fmt.Errorf("queens: given index %d out of range", g)
		}
	}
	return nil
}

// allRegionsConnected reports whether every region in the n×n grid is a single
// 4-connected component.
func allRegionsConnected(n int, region []int) bool {
	counts := make([]int, n)
	for _, id := range region {
		counts[id]++
	}
	// BFS each region from its first cell; component size must equal its count.
	first := make([]int, n)
	for i := range first {
		first[i] = -1
	}
	for i, id := range region {
		if first[id] == -1 {
			first[id] = i
		}
	}
	for id := 0; id < n; id++ {
		if counts[id] == 0 {
			return false
		}
		seen := map[int]bool{first[id]: true}
		queue := []int{first[id]}
		size := 0
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			size++
			for _, nb := range engine.Neighbors4(engine.CellAt(cur, n), n, n) {
				ni := engine.Index(nb, n)
				if region[ni] == id && !seen[ni] {
					seen[ni] = true
					queue = append(queue, ni)
				}
			}
		}
		if size != counts[id] {
			return false
		}
	}
	return true
}

// Entry returns this game's registry hook. Wiring code (internal/games/all)
// registers it; this package never registers itself.
func Entry() engine.Entry {
	gen := NewGenerator()
	solver := NewSolver()
	val := NewValidator()
	fp := NewFingerprinter()

	return engine.Entry{
		ID:   GameID,
		Name: "Queens",
		Generate: func(diff engine.Difficulty, r *rand.Rand) (engine.Generated, error) {
			p, sol, err := gen.Generate(diff, r)
			if err != nil {
				return engine.Generated{}, err
			}
			encoded, err := Encode(p)
			if err != nil {
				return engine.Generated{}, err
			}
			return engine.Generated{
				Puzzle:      p,
				Solution:    sol,
				Encoded:     encoded,
				Fingerprint: fp.Fingerprint(p),
			}, nil
		},
		Verify: func(encoded []byte) error {
			p, err := Decode(encoded)
			if err != nil {
				return err
			}
			if err := structurallyValid(p); err != nil {
				return err
			}
			// Givens, if any, must be consistent with a real solution.
			sol, ok := solver.Solve(p)
			if !ok {
				return fmt.Errorf("queens: puzzle has no solution")
			}
			if !val.Solved(boardOf(p, sol)) {
				return fmt.Errorf("queens: solver result is not a valid solution")
			}
			if n := solver.CountSolutions(p, 2); n != 1 {
				return fmt.Errorf("queens: puzzle has %d solutions, want exactly 1", n)
			}
			return nil
		},
	}
}

// boardOf builds a complete Board from a puzzle's region grid and a solution.
func boardOf(p Puzzle, sol Solution) Board {
	cells := make([]Cell, p.N*p.N)
	for row, col := range sol.QueenAt {
		cells[row*p.N+col] = Queen
	}
	return Board{N: p.N, Region: p.Region, Cells: cells}
}
