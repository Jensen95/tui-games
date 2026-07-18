package tango

import (
	"sort"

	"github.com/Jensen95/tui-games/internal/engine"
)

// Fingerprinter implements engine.Fingerprinter[Puzzle]. Tango's symmetry
// group has size 16 per docs/plan/games/tango.md "Uniqueness &
// deduplication": the 8 dihedral transforms of the square × the 2 symbol
// swaps (sun<->moon). Canonical picks the lexicographically smallest
// serialization across all 16.
type Fingerprinter struct{}

var _ engine.Fingerprinter[Puzzle] = Fingerprinter{}

// transformGivens maps a givens map through transform t on an n×n grid.
func transformGivens(givens map[int]Symbol, n int, t engine.Transform) map[int]Symbol {
	out := make(map[int]Symbol, len(givens))
	for idx, sym := range givens {
		c := engine.CellAt(idx, n)
		d := t.Apply(c, n, n)
		out[engine.Index(d, n)] = sym
	}
	return out
}

// transformEdgeSet maps one edge map through transform t, routing each
// transformed pair into h or v based on the transformed cells' ACTUAL
// adjacency: a transform can turn a horizontal edge into a vertical one
// (e.g. under a 90° rotation), so the source map (H or V) doesn't determine
// the destination map — the transformed geometry does.
func transformEdgeSet(m map[[2]int]Relation, n int, t engine.Transform, h, v map[[2]int]Relation) {
	for pair, rel := range m {
		c0 := engine.CellAt(pair[0], n)
		c1 := engine.CellAt(pair[1], n)
		d0 := t.Apply(c0, n, n)
		d1 := t.Apply(c1, n, n)
		i0, i1 := engine.Index(d0, n), engine.Index(d1, n)
		lo, hi := i0, i1
		if lo > hi {
			lo, hi = hi, lo
		}
		if d0.Row == d1.Row {
			h[[2]int{lo, hi}] = rel
		} else {
			v[[2]int{lo, hi}] = rel
		}
	}
}

// transformPuzzleGrid applies transform t to a puzzle's givens and edge
// sets, returning the transformed structures. Tango boards are always
// square, so a dimension-swapping transform (Rot90/Rot270/FlipMain/FlipAnti)
// still lands in an n×n grid.
func transformPuzzleGrid(p Puzzle, t engine.Transform) (map[int]Symbol, map[[2]int]Relation, map[[2]int]Relation) {
	givens := transformGivens(p.Givens, p.N, t)
	h := make(map[[2]int]Relation, len(p.HEdges)+len(p.VEdges))
	v := make(map[[2]int]Relation, len(p.HEdges)+len(p.VEdges))
	transformEdgeSet(p.HEdges, p.N, t, h, v)
	transformEdgeSet(p.VEdges, p.N, t, h, v)
	return givens, h, v
}

// serializePuzzle encodes one already-oriented (transformed + optionally
// symbol-swapped) puzzle into a comparison-stable byte string: every
// candidate for one puzzle shares N, so a plain lexicographic byte compare
// orders them correctly. Givens/edges are sorted by index so the encoding
// doesn't depend on map iteration order.
func serializePuzzle(n int, givens map[int]Symbol, h, v map[[2]int]Relation) []byte {
	type givenPair struct {
		idx int
		sym Symbol
	}
	gs := make([]givenPair, 0, len(givens))
	for idx, sym := range givens {
		gs = append(gs, givenPair{idx, sym})
	}
	sort.Slice(gs, func(i, j int) bool { return gs[i].idx < gs[j].idx })

	type edgeTriple struct {
		a, b int
		rel  Relation
	}
	toSorted := func(m map[[2]int]Relation) []edgeTriple {
		out := make([]edgeTriple, 0, len(m))
		for pair, rel := range m {
			out = append(out, edgeTriple{pair[0], pair[1], rel})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].a != out[j].a {
				return out[i].a < out[j].a
			}
			return out[i].b < out[j].b
		})
		return out
	}
	hs := toSorted(h)
	vs := toSorted(v)

	out := make([]byte, 0, 2+3*len(gs)+2+5*(len(hs)+len(vs)))
	out = append(out, byte(n))
	out = append(out, byte(len(gs)))
	for _, g := range gs {
		out = append(out, byte(g.idx>>8), byte(g.idx), byte(g.sym))
	}
	out = append(out, byte(len(hs)))
	for _, e := range hs {
		out = append(out, byte(e.a>>8), byte(e.a), byte(e.b>>8), byte(e.b), byte(e.rel))
	}
	out = append(out, byte(len(vs)))
	for _, e := range vs {
		out = append(out, byte(e.a>>8), byte(e.a), byte(e.b>>8), byte(e.b), byte(e.rel))
	}
	return out
}

// orientPuzzle applies transform t and, if swap is true, the sun<->moon
// symbol swap, then serializes the result.
func orientPuzzle(p Puzzle, t engine.Transform, swap bool) []byte {
	givens, h, v := transformPuzzleGrid(p, t)
	if swap {
		swapped := make(map[int]Symbol, len(givens))
		for idx, sym := range givens {
			swapped[idx] = flip(sym)
		}
		givens = swapped
	}
	return serializePuzzle(p.N, givens, h, v)
}

// Canonical returns the lexicographically smallest symmetry-normalized
// serialization of p across its 16-member transform group (8 dihedral × 2
// symbol swaps).
func (f Fingerprinter) Canonical(p Puzzle) []byte {
	candidates := make([][]byte, 0, 2*len(engine.AllTransforms))
	for _, t := range engine.AllTransforms {
		candidates = append(candidates, orientPuzzle(p, t, false))
		candidates = append(candidates, orientPuzzle(p, t, true))
	}
	return engine.CanonicalMin(candidates)
}

// Fingerprint hashes Canonical(p).
func (f Fingerprinter) Fingerprint(p Puzzle) [32]byte {
	return engine.FingerprintBytes(f.Canonical(p))
}
