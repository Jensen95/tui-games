package patches

import "github.com/Jensen95/tui-games/internal/engine"

// Fingerprinter canonicalizes a Patches puzzle for dedup. Colors are purely
// cosmetic (this game's data model doesn't even carry them), so the only
// symmetry group is the 8-way dihedral transform group over (clue anchor
// position, number, shape) triples — see docs/plan/games/patches.md
// "Uniqueness & deduplication".
type Fingerprinter struct{}

// NewFingerprinter creates a fingerprinter.
func NewFingerprinter() *Fingerprinter {
	return &Fingerprinter{}
}

var _ engine.Fingerprinter[*Puzzle] = (*Fingerprinter)(nil)

// appendUint32 appends v as 4 big-endian bytes, so a plain lexicographic
// byte comparison agrees with numeric comparison for the small non-negative
// values (grid dims, cell indices, clue numbers) this package ever encodes.
func appendUint32(b []byte, v int) []byte {
	u := uint32(v)
	return append(b, byte(u>>24), byte(u>>16), byte(u>>8), byte(u))
}

// serializePuzzle encodes one oriented grid's dimensions plus its sorted
// (anchor index, number, shape) clue triples into a comparison-stable byte
// string. Every candidate for one puzzle shares the same clue count, so this
// orders them correctly.
func serializePuzzle(rows, cols int, clues map[int]Clue) []byte {
	keys := sortedClueKeys(clues)
	out := make([]byte, 0, 12+len(keys)*9)
	out = appendUint32(out, rows)
	out = appendUint32(out, cols)
	out = appendUint32(out, len(keys))
	for _, k := range keys {
		c := clues[k]
		out = appendUint32(out, k)
		out = appendUint32(out, c.Number)
		out = append(out, byte(c.Shape))
	}
	return out
}

// transformShape maps a clue's shape through transform t. The four
// dimension-swapping transforms (Rot90, Rot270, transpose, anti-transpose)
// exchange a rectangle's width and height, so a Wide rectangle becomes Tall
// and vice versa; Square and Free are fixed under every transform. Getting
// this right is what makes a puzzle and its rotation share a fingerprint —
// see docs/plan/games/patches.md "Uniqueness & deduplication" and the
// cross-validation dedup table (Patches: Wide↔Tall under rotation).
func transformShape(s Shape, t engine.Transform) Shape {
	if !t.SwapsDims() {
		return s
	}
	switch s {
	case Wide:
		return Tall
	case Tall:
		return Wide
	default:
		return s
	}
}

// orient applies transform t to p's grid and clues, returning the
// transformed dimensions and clue map. Clue anchor cells move under t and
// each clue's shape is remapped by transformShape so the result describes the
// genuinely rotated/reflected puzzle, not merely a repositioned one.
func orient(p *Puzzle, t engine.Transform) (int, int, map[int]Clue) {
	newR, newC := t.Dims(p.R, p.C)
	newClues := make(map[int]Clue, len(p.Clues))
	for idx, clue := range p.Clues {
		src := engine.CellAt(idx, p.C)
		dst := t.Apply(src, p.R, p.C)
		clue.Shape = transformShape(clue.Shape, t)
		newClues[engine.Index(dst, newC)] = clue
	}
	return newR, newC, newClues
}

// Canonical returns the lexicographically smallest symmetry-normalized
// serialization of p across its 8-way dihedral transform group.
func (f *Fingerprinter) Canonical(p *Puzzle) []byte {
	candidates := make([][]byte, 0, len(engine.AllTransforms))
	for _, t := range engine.AllTransforms {
		rows, cols, clues := orient(p, t)
		candidates = append(candidates, serializePuzzle(rows, cols, clues))
	}
	return engine.CanonicalMin(candidates)
}

// Fingerprint hashes Canonical(p).
func (f *Fingerprinter) Fingerprint(p *Puzzle) [32]byte {
	return engine.FingerprintBytes(f.Canonical(p))
}
