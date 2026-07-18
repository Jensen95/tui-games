package tango

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
	Sym int `json:"sym"`
}

// wireEdge is the on-disk shape of one edge constraint.
type wireEdge struct {
	A   int `json:"a"`
	B   int `json:"b"`
	Rel int `json:"rel"`
}

// wire is the stable on-disk encoding of a Tango puzzle. It carries clues
// only — givens, edge constraints, and metadata — never the solution, which
// must never leak into an encoded puzzle.
type wire struct {
	Game       string      `json:"game"`
	N          int         `json:"n"`
	Givens     []wireGiven `json:"givens"`
	HEdges     []wireEdge  `json:"hedges"`
	VEdges     []wireEdge  `json:"vedges"`
	Seed       int64       `json:"seed"`
	Difficulty string      `json:"difficulty"`
}

func edgesToWire(m map[[2]int]Relation) []wireEdge {
	out := make([]wireEdge, 0, len(m))
	for pair, rel := range m {
		out = append(out, wireEdge{A: pair[0], B: pair[1], Rel: int(rel)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].A != out[j].A {
			return out[i].A < out[j].A
		}
		return out[i].B < out[j].B
	})
	return out
}

func wireToEdges(list []wireEdge) map[[2]int]Relation {
	out := make(map[[2]int]Relation, len(list))
	for _, e := range list {
		out[[2]int{e.A, e.B}] = Relation(e.Rel)
	}
	return out
}

// Encode serializes p to its stable JSON form (clues only, no solution).
func Encode(p Puzzle) ([]byte, error) {
	givens := make([]wireGiven, 0, len(p.Givens))
	for idx, sym := range p.Givens {
		givens = append(givens, wireGiven{Idx: idx, Sym: int(sym)})
	}
	sort.Slice(givens, func(i, j int) bool { return givens[i].Idx < givens[j].Idx })

	w := wire{
		Game:       string(GameID),
		N:          p.N,
		Givens:     givens,
		HEdges:     edgesToWire(p.HEdges),
		VEdges:     edgesToWire(p.VEdges),
		Seed:       p.SeedVal,
		Difficulty: p.Diff.String(),
	}
	return json.Marshal(w)
}

// Decode parses a puzzle previously produced by Encode.
func Decode(data []byte) (Puzzle, error) {
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return Puzzle{}, fmt.Errorf("tango: decode: %w", err)
	}
	if w.Game != string(GameID) {
		return Puzzle{}, fmt.Errorf("tango: decode: game %q, want %q", w.Game, GameID)
	}
	diff, err := engine.ParseDifficulty(w.Difficulty)
	if err != nil {
		return Puzzle{}, fmt.Errorf("tango: decode: %w", err)
	}
	givens := make(map[int]Symbol, len(w.Givens))
	for _, g := range w.Givens {
		givens[g.Idx] = Symbol(g.Sym)
	}
	return Puzzle{
		N:       w.N,
		Givens:  givens,
		HEdges:  wireToEdges(w.HEdges),
		VEdges:  wireToEdges(w.VEdges),
		SeedVal: w.Seed,
		Diff:    diff,
	}, nil
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// structurallyValid checks a decoded puzzle's shape independent of solving:
// N is a positive even number, givens/edges are in bounds and hold
// Sun/Moon/Equal/Cross values, and every H/V edge actually connects
// orthogonally adjacent cells in the claimed direction.
func structurallyValid(p Puzzle) error {
	if p.N <= 0 || p.N%2 != 0 {
		return fmt.Errorf("tango: N=%d must be a positive even number", p.N)
	}
	total := p.N * p.N
	for idx, sym := range p.Givens {
		if idx < 0 || idx >= total {
			return fmt.Errorf("tango: given index %d out of range 0..%d", idx, total-1)
		}
		if sym != Sun && sym != Moon {
			return fmt.Errorf("tango: given at %d has invalid symbol %d", idx, sym)
		}
	}
	checkEdges := func(m map[[2]int]Relation, wantHorizontal bool) error {
		for pair, rel := range m {
			a, b := pair[0], pair[1]
			if a < 0 || a >= total || b < 0 || b >= total {
				return fmt.Errorf("tango: edge (%d,%d) out of range", a, b)
			}
			if rel != Equal && rel != Cross {
				return fmt.Errorf("tango: edge (%d,%d) has invalid relation %d", a, b, rel)
			}
			ca, cb := engine.CellAt(a, p.N), engine.CellAt(b, p.N)
			horizontal := ca.Row == cb.Row && absInt(ca.Col-cb.Col) == 1
			vertical := ca.Col == cb.Col && absInt(ca.Row-cb.Row) == 1
			if wantHorizontal && !horizontal {
				return fmt.Errorf("tango: hedge (%d,%d) is not horizontally adjacent", a, b)
			}
			if !wantHorizontal && !vertical {
				return fmt.Errorf("tango: vedge (%d,%d) is not vertically adjacent", a, b)
			}
		}
		return nil
	}
	if err := checkEdges(p.HEdges, true); err != nil {
		return err
	}
	if err := checkEdges(p.VEdges, false); err != nil {
		return err
	}
	return nil
}

// Entry returns the engine registry entry for tango. It is not registered
// here — internal/games/all wires every game into the registry in one
// place.
func Entry() engine.Entry {
	gen := Generator{}
	solver := Solver{}
	fp := Fingerprinter{}

	return engine.Entry{
		ID:   GameID,
		Name: "Tango",
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
			if n := solver.CountSolutions(p, 2); n != 1 {
				return fmt.Errorf("tango: puzzle has %d solutions, want exactly 1", n)
			}
			return nil
		},
	}
}
