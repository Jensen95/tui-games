package patches

// This file holds an INDEPENDENT second solver used only to cross-validate the
// primary solver (solver.go) and the generator. It is deliberately written
// from the spec (docs/plan/games/patches.md "Rules (precise)" and "Solver
// approach"), NOT by mirroring solver.go, so that a bug shared between the
// primary solver and the generator surfaces here as a disagreement rather than
// being silently reproduced.
//
// Algorithm — cell-first exhaustive exact cover:
//
//	Scan cells in row-major order for the first still-uncovered cell. In any
//	valid tiling that cell is the TOP-LEFT corner of the rectangle covering it
//	(every earlier cell is already covered, so the covering rectangle cannot
//	extend up or left). Enumerate every rectangle anchored at that corner that
//	fits in free space and contains exactly one clue whose number equals the
//	rectangle's area and whose shape matches the rectangle; place it and
//	recurse. When no cell is uncovered the board is fully tiled and — because
//	every placed rectangle carried exactly one clue and the rectangles are
//	disjoint and gap-free — it is a complete valid solution.
//
// This is structurally different from solver.go, which precomputes a candidate
// set per clue and assigns clues via MRV backtracking with an owner grid.
// Here the recursion is driven by empty cells and rectangles are enumerated on
// the fly from a forced corner, so gaps are impossible by construction and the
// two solvers share no candidate-generation code.

// crossShapeOK is an independent restatement of the spec's shape rule (spec
// Rules #4). Kept separate from validator.go's shapeOK so a bug there cannot
// hide here.
func crossShapeOK(s Shape, w, h int) bool {
	switch s {
	case Square:
		return w == h
	case Wide:
		return w > h
	case Tall:
		return h > w
	case Free:
		return true
	default:
		return false
	}
}

// crossSolver is the exhaustive cross-check solver over one puzzle.
type crossSolver struct {
	p       *Puzzle
	owner   []int // per cell: placement id owning it, or -1
	rects   []Rect
	results [][]Rect // completed tilings (each a copy of rects), capped by the caller
	cap     int
}

// newCrossSolver builds a solver bound to p with the given solution cap.
func newCrossSolver(p *Puzzle, cap int) *crossSolver {
	owner := make([]int, p.R*p.C)
	for i := range owner {
		owner[i] = -1
	}
	return &crossSolver{p: p, owner: owner, cap: cap}
}

// clueAt reports the clue at cell index i, if any.
func (cs *crossSolver) clueAt(i int) (Clue, bool) {
	c, ok := cs.p.Clues[i]
	return c, ok
}

// firstFree returns the row-major index of the first uncovered cell, or -1 if
// the whole grid is covered.
func (cs *crossSolver) firstFree() int {
	for i, o := range cs.owner {
		if o < 0 {
			return i
		}
	}
	return -1
}

// place / unplace mark a rectangle's cells owned by id (or free again).
func (cs *crossSolver) place(rect Rect, id int) {
	for r := rect.R0; r < rect.R0+rect.H; r++ {
		for c := rect.C0; c < rect.C0+rect.W; c++ {
			cs.owner[r*cs.p.C+c] = id
		}
	}
}

func (cs *crossSolver) unplace(rect Rect) {
	for r := rect.R0; r < rect.R0+rect.H; r++ {
		for c := rect.C0; c < rect.C0+rect.W; c++ {
			cs.owner[r*cs.p.C+c] = -1
		}
	}
}

// blockFree reports whether every cell of the W×H block anchored at (r0,c0) is
// in bounds and currently uncovered.
func (cs *crossSolver) blockFree(r0, c0, w, h int) bool {
	if r0+h > cs.p.R || c0+w > cs.p.C {
		return false
	}
	for r := r0; r < r0+h; r++ {
		for c := c0; c < c0+w; c++ {
			if cs.owner[r*cs.p.C+c] >= 0 {
				return false
			}
		}
	}
	return true
}

// singleClueInBlock returns the sole clue inside the W×H block anchored at
// (r0,c0) and true, or false if the block holds zero or more than one clue.
func (cs *crossSolver) singleClueInBlock(r0, c0, w, h int) (Clue, bool) {
	var found Clue
	count := 0
	for r := r0; r < r0+h; r++ {
		for c := c0; c < c0+w; c++ {
			if cl, ok := cs.clueAt(r*cs.p.C + c); ok {
				count++
				if count > 1 {
					return Clue{}, false
				}
				found = cl
			}
		}
	}
	return found, count == 1
}

// recurse fills the first free cell in every legal way, collecting completed
// tilings up to cap.
func (cs *crossSolver) recurse() {
	if len(cs.results) >= cs.cap {
		return
	}
	free := cs.firstFree()
	if free < 0 {
		// Fully covered: every placed rect had exactly one clue and the rects
		// are disjoint and gap-free, so this is a valid complete solution.
		cs.results = append(cs.results, append([]Rect(nil), cs.rects...))
		return
	}
	r0, c0 := free/cs.p.C, free%cs.p.C
	maxH := cs.p.R - r0
	maxW := cs.p.C - c0
	for h := 1; h <= maxH; h++ {
		// If the leftmost column of this height is blocked below, taller
		// rectangles are impossible too — but keep it simple and just test
		// each (w,h) via blockFree, which stays correct at these grid sizes.
		for w := 1; w <= maxW; w++ {
			if !cs.blockFree(r0, c0, w, h) {
				// Growing wider keeps this row blocked; stop widening.
				break
			}
			clue, ok := cs.singleClueInBlock(r0, c0, w, h)
			if !ok {
				continue
			}
			if w*h != clue.Number {
				continue
			}
			if !crossShapeOK(clue.Shape, w, h) {
				continue
			}
			id := len(cs.rects)
			rect := Rect{R0: r0, C0: c0, W: w, H: h}
			cs.rects = append(cs.rects, rect)
			cs.place(rect, id)
			cs.recurse()
			cs.unplace(rect)
			cs.rects = cs.rects[:len(cs.rects)-1]
			if len(cs.results) >= cs.cap {
				return
			}
		}
	}
}

// bruteSolutions returns up to cap complete tilings of p found by the
// independent cell-first search.
func bruteSolutions(p *Puzzle, cap int) [][]Rect {
	if cap <= 0 {
		return nil
	}
	cs := newCrossSolver(p, cap)
	cs.recurse()
	return cs.results
}

// bruteCount returns min(#solutions, cap) via the independent solver.
func bruteCount(p *Puzzle, cap int) int {
	return len(bruteSolutions(p, cap))
}

// coverLabels turns a set of rectangles into a per-cell coverage signature:
// for each cell, the bounding box (R0,C0,W,H) of the rectangle covering it, or
// a zero Rect if uncovered. Two tilings are the SAME partition iff their
// signatures are equal, independent of the order the rectangles were listed in
// (so a solution from the primary solver and one from the cross solver compare
// directly).
func coverLabels(p *Puzzle, rects []Rect) []Rect {
	sig := make([]Rect, p.R*p.C)
	for _, rect := range rects {
		for r := rect.R0; r < rect.R0+rect.H; r++ {
			for c := rect.C0; c < rect.C0+rect.W; c++ {
				if r >= 0 && r < p.R && c >= 0 && c < p.C {
					sig[r*p.C+c] = rect
				}
			}
		}
	}
	return sig
}

// sameTiling reports whether two rectangle sets partition the grid identically.
func sameTiling(p *Puzzle, a, b []Rect) bool {
	sa, sb := coverLabels(p, a), coverLabels(p, b)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}
