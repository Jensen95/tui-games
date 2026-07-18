package minisudoku

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sort"

	"github.com/Jensen95/tui-games/internal/engine"
)

// wireGiven is the on-disk shape of one given cell.
type wireGiven struct {
	Idx int `json:"idx"`
	Val int `json:"val"`
}

// wire is the stable on-disk encoding of a Mini Sudoku puzzle. It carries
// clues only — givens and metadata — never the solution, which must never
// leak into an encoded puzzle.
type wire struct {
	Game       string      `json:"game"`
	N          int         `json:"n"`
	BoxH       int         `json:"boxH"`
	BoxW       int         `json:"boxW"`
	Givens     []wireGiven `json:"givens"`
	Seed       int64       `json:"seed"`
	Difficulty string      `json:"difficulty"`
}

// Encode serializes p to its stable JSON form (clues only, no solution).
func Encode(p Puzzle) ([]byte, error) {
	givens := make([]wireGiven, 0, len(p.Givens))
	for idx, val := range p.Givens {
		givens = append(givens, wireGiven{Idx: idx, Val: val})
	}
	sort.Slice(givens, func(i, j int) bool { return givens[i].Idx < givens[j].Idx })

	w := wire{
		Game:       string(GameID),
		N:          p.N,
		BoxH:       p.BoxH,
		BoxW:       p.BoxW,
		Givens:     givens,
		Seed:       p.SeedVal,
		Difficulty: p.Diff.String(),
	}
	return json.Marshal(w)
}

// Decode parses a puzzle previously produced by Encode.
func Decode(data []byte) (Puzzle, error) {
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return Puzzle{}, fmt.Errorf("minisudoku: decode: %w", err)
	}
	if w.Game != string(GameID) {
		return Puzzle{}, fmt.Errorf("minisudoku: decode: game %q, want %q", w.Game, GameID)
	}
	diff, err := engine.ParseDifficulty(w.Difficulty)
	if err != nil {
		return Puzzle{}, fmt.Errorf("minisudoku: decode: %w", err)
	}
	givens := make(map[int]int, len(w.Givens))
	for _, g := range w.Givens {
		givens[g.Idx] = g.Val
	}
	return Puzzle{
		N:       w.N,
		BoxH:    w.BoxH,
		BoxW:    w.BoxW,
		Givens:  givens,
		SeedVal: w.Seed,
		Diff:    diff,
	}, nil
}

// structurallyValid checks a decoded puzzle's shape independent of solving:
// N/BoxH/BoxW match this game's fixed geometry, and every given is in
// bounds with a digit in 1..N.
func structurallyValid(p Puzzle) error {
	if p.N != N || p.BoxH != BoxH || p.BoxW != BoxW {
		return fmt.Errorf("minisudoku: geometry %dx%d box %dx%d, want %dx%d box %dx%d", p.N, p.N, p.BoxH, p.BoxW, N, N, BoxH, BoxW)
	}
	for idx, val := range p.Givens {
		if idx < 0 || idx >= p.N*p.N {
			return fmt.Errorf("minisudoku: given index %d out of range 0..%d", idx, p.N*p.N-1)
		}
		if val < 1 || val > p.N {
			return fmt.Errorf("minisudoku: given at %d has invalid digit %d", idx, val)
		}
	}
	return nil
}

// Entry returns the engine registry entry for minisudoku. It is not
// registered here — internal/games/all wires every game into the registry
// in one place.
func Entry() engine.Entry {
	gen := Generator{}
	solver := Solver{}
	val := Validator{}
	fp := Fingerprinter{}

	return engine.Entry{
		ID:   GameID,
		Name: "Mini Sudoku",
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
			sol, ok := solver.Solve(p)
			if !ok {
				return fmt.Errorf("minisudoku: puzzle has no solution")
			}
			if !val.Solved(Board{Cells: sol.Cells}) {
				return fmt.Errorf("minisudoku: solver result is not a valid solution")
			}
			if n := solver.CountSolutions(p, 2); n != 1 {
				return fmt.Errorf("minisudoku: puzzle has %d solutions, want exactly 1", n)
			}
			return nil
		},
	}
}
