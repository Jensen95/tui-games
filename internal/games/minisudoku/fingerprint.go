package minisudoku

import (
	"github.com/Jensen95/tui-games/internal/engine"
)

// Fingerprinter implements engine.Fingerprinter[Puzzle].
//
// Mini Sudoku's symmetry group (docs/plan/games/mini-sudoku.md "Uniqueness &
// deduplication", docs/plan/docs/02-engine-and-generation.md's table) is: 8
// dihedral × digit relabel × band/stack permutations. Two refinements vs. a
// literal reading of "8 dihedral":
//
//   - Box-preserving subset. A 2×3 box is NOT square, so rotating the grid
//     90°/270° (or transposing) turns each box into a 3×2 region — no
//     longer a valid Mini Sudoku box. Only the four dihedral transforms that
//     preserve box orientation (Identity, Rot180, FlipH, FlipV) map a valid
//     puzzle to another valid puzzle of the SAME game; the other four are
//     excluded here (if BoxH==BoxW, e.g. a future square-box variant, boxes
//     stay valid under every transform, so all 8 are used).
//   - Band/stack permutations: bands are the N/BoxH groups of BoxH rows
//     (here: 3 bands of 2 rows), stacks are the N/BoxW groups of BoxW
//     columns (here: 2 stacks of 3 columns). Bands may be reordered among
//     themselves, as may the rows within a band; likewise stacks and the
//     columns within a stack — all without changing which digits satisfy
//     the row/col/box constraints.
//   - Digit relabeling is handled by construction: canonicalDigits
//     renumbers by first-appearance order, which is invariant under ANY
//     bijective relabeling of the original grid (the same positions become
//     "first", "second", etc. no matter which symbols were used), so no
//     separate enumeration over digit permutations is needed.
//
// This is a large-but-documented subset of the full group (per the spec's
// "use full or a documented subset" allowance) chosen to stay well within
// the performance budget: a few thousand candidate serializations per
// fingerprint, each O(N²).
type Fingerprinter struct{}

var _ engine.Fingerprinter[Puzzle] = Fingerprinter{}

// permutations returns every permutation of 0..n-1.
func permutations(n int) [][]int {
	elems := make([]int, n)
	for i := range elems {
		elems[i] = i
	}
	var rec func([]int) [][]int
	rec = func(xs []int) [][]int {
		if len(xs) <= 1 {
			return [][]int{append([]int(nil), xs...)}
		}
		var out [][]int
		for i, e := range xs {
			rest := make([]int, 0, len(xs)-1)
			rest = append(rest, xs[:i]...)
			rest = append(rest, xs[i+1:]...)
			for _, p := range rec(rest) {
				out = append(out, append([]int{e}, p...))
			}
		}
		return out
	}
	return rec(elems)
}

// bandStackRowPerms returns every row permutation (a length-N slice mapping
// output row -> source row) obtainable by reordering the N/boxH bands
// (groups of boxH rows) and, independently within each band, reordering its
// boxH rows.
func bandStackRowPerms(n, boxH int) [][]int {
	numBands := n / boxH
	bandOrders := permutations(numBands)
	subOrders := permutations(boxH)

	var perms [][]int
	var build func(bandOrder []int, chosen [][]int, bandIdx int)
	build = func(bandOrder []int, chosen [][]int, bandIdx int) {
		if bandIdx == numBands {
			perm := make([]int, 0, n)
			for outPos, origBand := range bandOrder {
				for _, sub := range chosen[outPos] {
					perm = append(perm, origBand*boxH+sub)
				}
			}
			perms = append(perms, perm)
			return
		}
		for _, sub := range subOrders {
			build(bandOrder, append(chosen, sub), bandIdx+1)
		}
	}
	for _, bandOrder := range bandOrders {
		build(bandOrder, nil, 0)
	}
	return perms
}

// canonicalDigits renumbers cells (0 = empty, else a digit) by
// first-appearance order scanning row-major, e.g. [0,5,5,2] -> [0,1,1,2].
// This collapses every bijective digit relabeling of the same grid to one
// canonical form.
func canonicalDigits(cells []int) []byte {
	next := 1
	seen := make(map[int]int, len(cells))
	out := make([]byte, len(cells))
	for i, v := range cells {
		if v == 0 {
			continue
		}
		m, ok := seen[v]
		if !ok {
			m = next
			seen[v] = m
			next++
		}
		out[i] = byte(m)
	}
	return out
}

// boxPreservingDihedral returns the dihedral transforms that keep a
// boxH×boxW box a valid boxH×boxW box: all 8 if the box is square, else just
// the 4 that never swap grid dimensions.
func boxPreservingDihedral(boxH, boxW int) []engine.Transform {
	if boxH == boxW {
		out := make([]engine.Transform, len(engine.AllTransforms))
		copy(out, engine.AllTransforms[:])
		return out
	}
	return []engine.Transform{engine.Identity, engine.Rot180, engine.FlipH, engine.FlipV}
}

// Canonical returns the lexicographically smallest symmetry-normalized
// serialization of p's givens across the transform group described on
// Fingerprinter.
func (f Fingerprinter) Canonical(p Puzzle) []byte {
	n, boxH, boxW := p.N, p.BoxH, p.BoxW
	if n == 0 {
		n, boxH, boxW = N, BoxH, BoxW
	}

	grid := make([]int, n*n)
	for idx, v := range p.Givens {
		grid[idx] = v
	}

	rowPerms := bandStackRowPerms(n, boxH)
	colPerms := bandStackRowPerms(n, boxW)
	dihedrals := boxPreservingDihedral(boxH, boxW)

	candidates := make([][]byte, 0, len(rowPerms)*len(colPerms)*len(dihedrals))
	out := make([]int, n*n)
	for _, rp := range rowPerms {
		for _, cp := range colPerms {
			for _, t := range dihedrals {
				for i := 0; i < n; i++ {
					for j := 0; j < n; j++ {
						dst := t.Apply(engine.Cell{Row: i, Col: j}, n, n)
						out[dst.Row*n+dst.Col] = grid[rp[i]*n+cp[j]]
					}
				}
				candidates = append(candidates, canonicalDigits(out))
			}
		}
	}
	return engine.CanonicalMin(candidates)
}

// Fingerprint hashes Canonical(p).
func (f Fingerprinter) Fingerprint(p Puzzle) [32]byte {
	return engine.FingerprintBytes(f.Canonical(p))
}
