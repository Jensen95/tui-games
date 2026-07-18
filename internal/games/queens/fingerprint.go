package queens

import (
	"sort"

	"github.com/Jensen95/tui-games/internal/engine"
)

// Fingerprinter canonicalizes a Queens puzzle for dedup. Canonicalization
// must be color-agnostic: region ids are normalized by first-appearance
// order (engine.RelabelFirstAppearance) under every transform in the 8-way
// dihedral group before picking the lexicographic minimum
// (engine.CanonicalMin). See docs/plan/games/queens.md "Uniqueness &
// deduplication".
type Fingerprinter struct{}

// NewFingerprinter returns a Queens fingerprinter.
func NewFingerprinter() *Fingerprinter { return &Fingerprinter{} }

var _ engine.Fingerprinter[Puzzle] = (*Fingerprinter)(nil)

// serialize encodes one oriented (already relabeled) region grid plus its
// sorted given indices into a comparison-stable byte string. Every candidate
// for one puzzle shares N, region length and givens count, so a plain
// lexicographic byte compare orders them correctly.
func serialize(n int, region []int, givens []int) []byte {
	out := make([]byte, 0, 2+len(region)+1+len(givens))
	out = append(out, byte(n))
	for _, id := range region {
		out = append(out, byte(id))
	}
	out = append(out, byte(len(givens)))
	for _, g := range givens {
		out = append(out, byte(g))
	}
	return out
}

// orient applies transform t to p's region grid and givens, then normalizes
// region labels by first appearance so the encoding is color-agnostic.
func orient(p Puzzle, t engine.Transform) []byte {
	n := p.N
	region := make([]int, n*n)
	for r := 0; r < n; r++ {
		for c := 0; c < n; c++ {
			src := engine.Cell{Row: r, Col: c}
			dst := t.Apply(src, n, n)
			region[engine.Index(dst, n)] = p.Region[engine.Index(src, n)]
		}
	}
	region = engine.RelabelFirstAppearance(region)

	var givens []int
	if len(p.Givens) > 0 {
		givens = make([]int, len(p.Givens))
		for i, g := range p.Givens {
			src := engine.CellAt(g, n)
			dst := t.Apply(src, n, n)
			givens[i] = engine.Index(dst, n)
		}
		sort.Ints(givens)
	}
	return serialize(n, region, givens)
}

// Canonical returns the lexicographically smallest symmetry-normalized
// serialization of p across its 8-way dihedral transform group.
func (f *Fingerprinter) Canonical(p Puzzle) []byte {
	candidates := make([][]byte, 0, len(engine.AllTransforms))
	for _, t := range engine.AllTransforms {
		candidates = append(candidates, orient(p, t))
	}
	return engine.CanonicalMin(candidates)
}

// Fingerprint hashes Canonical(p).
func (f *Fingerprinter) Fingerprint(p Puzzle) [32]byte {
	return engine.FingerprintBytes(f.Canonical(p))
}
