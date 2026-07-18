package patches

import "github.com/Jensen95/tui-games/internal/engine"

// Technique name constants used by LogicSolve's deduction ladder. Exact
// naming is an implementation detail; tests only assert non-emptiness when a
// puzzle closes, never a specific technique string.
const (
	TechniqueNone engine.Technique = ""
	// TechniqueSingleton fires when a clue has exactly one remaining
	// candidate rectangle.
	TechniqueSingleton engine.Technique = "clue-singleton"
	// TechniqueCellForced fires when a still-uncovered cell can only be
	// covered by one (clue, candidate) pair across the whole board.
	TechniqueCellForced engine.Technique = "cell-forced"
	// TechniqueContradiction fires when tentatively placing a candidate and
	// propagating the two techniques above to fixpoint proves a
	// contradiction, so that candidate is eliminated as impossible.
	TechniqueContradiction engine.Technique = "contradiction-elimination"
)

func techRank(t engine.Technique) int {
	switch t {
	case TechniqueSingleton:
		return 1
	case TechniqueCellForced:
		return 2
	case TechniqueContradiction:
		return 3
	default:
		return 0
	}
}

// Solver is the Patches complete + logic solver. See
// engine.Solver[*Puzzle, *Solution].
type Solver struct {
	P *Puzzle
}

// NewSolver creates a solver for a puzzle.
func NewSolver(p *Puzzle) *Solver {
	return &Solver{P: p}
}

var _ engine.Solver[*Puzzle, *Solution] = (*Solver)(nil)

// clueInfo pairs a clue with every candidate rectangle that could realize
// it: a w×h rectangle (matching its number and shape) that covers its own
// anchor cell, stays in bounds, and contains no other clue's cell.
type clueInfo struct {
	idx   int
	clue  Clue
	cands []Rect
}

func buildClueInfos(p *Puzzle) []clueInfo {
	keys := sortedClueKeys(p.Clues)
	infos := make([]clueInfo, len(keys))
	for i, k := range keys {
		c := p.Clues[k]
		infos[i] = clueInfo{idx: k, clue: c, cands: candidatesFor(p, k, c)}
	}
	return infos
}

// candidatesFor enumerates every placement of a rectangle that: has area ==
// clue.Number, satisfies clue.Shape, covers cell idx, stays within the grid,
// and contains no cell belonging to a different clue.
func candidatesFor(p *Puzzle, idx int, clue Clue) []Rect {
	if clue.Number < 1 {
		return nil
	}
	r0, c0 := idx/p.C, idx%p.C
	var out []Rect
	for w := 1; w <= clue.Number; w++ {
		if clue.Number%w != 0 {
			continue
		}
		h := clue.Number / w
		if h < 1 || h > p.R || w > p.C {
			continue
		}
		switch clue.Shape {
		case Square:
			if w != h {
				continue
			}
		case Wide:
			if w <= h {
				continue
			}
		case Tall:
			if h <= w {
				continue
			}
		}
		r0min, r0max := max(0, r0-h+1), min(r0, p.R-h)
		c0min, c0max := max(0, c0-w+1), min(c0, p.C-w)
		for R0 := r0min; R0 <= r0max; R0++ {
			for C0 := c0min; C0 <= c0max; C0++ {
				rect := Rect{R0: R0, C0: C0, W: w, H: h}
				if rectContainsOtherClue(p, rect, idx) {
					continue
				}
				out = append(out, rect)
			}
		}
	}
	return out
}

// rectContainsOtherClue reports whether rect covers any clue cell other than ownIdx.
func rectContainsOtherClue(p *Puzzle, rect Rect, ownIdx int) bool {
	for r := rect.R0; r < rect.R0+rect.H; r++ {
		for c := rect.C0; c < rect.C0+rect.W; c++ {
			i := r*p.C + c
			if i == ownIdx {
				continue
			}
			if _, ok := p.Clues[i]; ok {
				return true
			}
		}
	}
	return false
}

// rectContainsCell reports whether rect covers the given row-major cell index.
func rectContainsCell(rect Rect, cellIdx, cols int) bool {
	r, c := cellIdx/cols, cellIdx%cols
	return r >= rect.R0 && r < rect.R0+rect.H && c >= rect.C0 && c < rect.C0+rect.W
}

// rectsOverlap reports whether two rectangles share any cell.
func rectsOverlap(a, b Rect) bool {
	return a.R0 < b.R0+b.H && b.R0 < a.R0+a.H && a.C0 < b.C0+b.W && b.C0 < a.C0+a.W
}

// search runs a complete backtracking exact-cover search: assign each clue
// one of its candidate rectangles so that no two overlap. Because every
// candidate is checked to contain exactly its own clue, and the clue numbers
// summing to R*C is verified up front, any overlap-free full assignment
// automatically tiles the grid with no gaps (the union of non-overlapping
// areas summing to R*C must be the whole grid). Uses a most-constrained-first
// (MRV) heuristic for speed; stops once cap solutions are found.
func search(p *Puzzle, infos []clueInfo, cap int) []*Solution {
	if cap <= 0 {
		return nil
	}
	n := p.R * p.C
	sum := 0
	for _, ci := range infos {
		sum += ci.clue.Number
	}
	if sum != n {
		return nil
	}

	owner := make([]int, n)
	for i := range owner {
		owner[i] = -1
	}
	assigned := make([]Rect, len(infos))
	used := make([]bool, len(infos))
	var results []*Solution

	rectFree := func(rect Rect) bool {
		for r := rect.R0; r < rect.R0+rect.H; r++ {
			for c := rect.C0; c < rect.C0+rect.W; c++ {
				if owner[r*p.C+c] != -1 {
					return false
				}
			}
		}
		return true
	}
	markOwner := func(rect Rect, id int) {
		for r := rect.R0; r < rect.R0+rect.H; r++ {
			for c := rect.C0; c < rect.C0+rect.W; c++ {
				owner[r*p.C+c] = id
			}
		}
	}
	clearOwner := func(rect Rect) {
		for r := rect.R0; r < rect.R0+rect.H; r++ {
			for c := rect.C0; c < rect.C0+rect.W; c++ {
				owner[r*p.C+c] = -1
			}
		}
	}

	var rec func() bool
	rec = func() bool {
		best := -1
		var bestCands []Rect
		for i := range infos {
			if used[i] {
				continue
			}
			var valid []Rect
			for _, rct := range infos[i].cands {
				if rectFree(rct) {
					valid = append(valid, rct)
				}
			}
			if len(valid) == 0 {
				return false
			}
			if best == -1 || len(valid) < len(bestCands) {
				best, bestCands = i, valid
			}
		}
		if best == -1 {
			results = append(results, &Solution{Rects: append([]Rect(nil), assigned...)})
			return len(results) >= cap
		}
		for _, rct := range bestCands {
			markOwner(rct, best)
			assigned[best] = rct
			used[best] = true
			stop := rec()
			clearOwner(rct)
			used[best] = false
			if stop {
				return true
			}
		}
		return false
	}
	rec()
	return results
}

// Solve returns one solution if one exists.
func (s *Solver) Solve(p *Puzzle) (*Solution, bool) {
	infos := buildClueInfos(p)
	results := search(p, infos, 1)
	if len(results) == 0 {
		return nil, false
	}
	return results[0], true
}

// CountSolutions returns min(#solutions, cap).
func (s *Solver) CountSolutions(p *Puzzle, cap int) int {
	if cap <= 0 {
		return 0
	}
	infos := buildClueInfos(p)
	return len(search(p, infos, cap))
}

// LogicSolve attempts a no-guess solve via a three-rung deduction ladder
// (clue-singleton, cell-forced, contradiction-elimination — see the
// Technique constants above). Returns the solution, whether it fully closed,
// and the deepest technique required.
func (s *Solver) LogicSolve(p *Puzzle) (*Solution, bool, engine.Technique) {
	st := newLogicState(p)
	if st.run() {
		return st.solution(), true, st.deepest
	}
	return nil, false, st.deepest
}

// logicState holds the candidate rectangles the deduction ladder narrows to
// fixpoint, one committed rectangle per clue once solved.
type logicState struct {
	p        *Puzzle
	infos    []clueInfo
	active   [][]Rect // per clue index (parallel to infos): still-viable candidates
	placed   []Rect
	isPlaced []bool
	owner    []int // per cell: clue index owning it, or -1
	deepest  engine.Technique
	failed   bool
}

func newLogicState(p *Puzzle) *logicState {
	infos := buildClueInfos(p)
	active := make([][]Rect, len(infos))
	for i, ci := range infos {
		active[i] = append([]Rect(nil), ci.cands...)
	}
	owner := make([]int, p.R*p.C)
	for i := range owner {
		owner[i] = -1
	}
	return &logicState{
		p:        p,
		infos:    infos,
		active:   active,
		placed:   make([]Rect, len(infos)),
		isPlaced: make([]bool, len(infos)),
		owner:    owner,
	}
}

func (st *logicState) clone() *logicState {
	cl := &logicState{p: st.p, infos: st.infos, deepest: st.deepest, failed: st.failed}
	cl.active = make([][]Rect, len(st.active))
	for i, a := range st.active {
		cl.active[i] = append([]Rect(nil), a...)
	}
	cl.placed = append([]Rect(nil), st.placed...)
	cl.isPlaced = append([]bool(nil), st.isPlaced...)
	cl.owner = append([]int(nil), st.owner...)
	return cl
}

func (st *logicState) deepen(t engine.Technique) {
	if techRank(t) > techRank(st.deepest) {
		st.deepest = t
	}
}

func (st *logicState) solvedAll() bool {
	for _, d := range st.isPlaced {
		if !d {
			return false
		}
	}
	return true
}

// commit places rect for clue i (a no-op success if i is already placed with
// this exact rect), marks its cells owned, and prunes it out of every other
// unplaced clue's active candidates. Returns false on contradiction: an
// occupied cell, or another clue left with zero candidates.
func (st *logicState) commit(i int, rect Rect) bool {
	if st.isPlaced[i] {
		return st.placed[i] == rect
	}
	c := st.p.C
	for r := rect.R0; r < rect.R0+rect.H; r++ {
		for cc := rect.C0; cc < rect.C0+rect.W; cc++ {
			if st.owner[r*c+cc] != -1 {
				return false
			}
		}
	}
	for r := rect.R0; r < rect.R0+rect.H; r++ {
		for cc := rect.C0; cc < rect.C0+rect.W; cc++ {
			st.owner[r*c+cc] = i
		}
	}
	st.placed[i] = rect
	st.isPlaced[i] = true
	st.active[i] = nil

	for j := range st.active {
		if j == i || st.isPlaced[j] {
			continue
		}
		kept := st.active[j][:0]
		for _, r := range st.active[j] {
			if !rectsOverlap(r, rect) {
				kept = append(kept, r)
			}
		}
		st.active[j] = kept
		if len(kept) == 0 {
			return false
		}
	}
	return true
}

// hasContradiction reports whether some unplaced clue has no active
// candidate left, or some uncovered cell can no longer be covered by any
// remaining candidate.
func (st *logicState) hasContradiction() bool {
	if st.failed {
		return true
	}
	for i := range st.infos {
		if !st.isPlaced[i] && len(st.active[i]) == 0 {
			return true
		}
	}
	n := st.p.R * st.p.C
	for cell := 0; cell < n; cell++ {
		if st.owner[cell] != -1 {
			continue
		}
		if st.cellCoverCount(cell) == 0 {
			return true
		}
	}
	return false
}

func (st *logicState) cellCoverCount(cell int) int {
	cnt := 0
	for i := range st.infos {
		if st.isPlaced[i] {
			continue
		}
		for _, r := range st.active[i] {
			if rectContainsCell(r, cell, st.p.C) {
				cnt++
			}
		}
	}
	return cnt
}

// progressSingleton commits any unplaced clue with exactly one remaining candidate.
func (st *logicState) progressSingleton() bool {
	for i := range st.infos {
		if st.isPlaced[i] {
			continue
		}
		if len(st.active[i]) == 1 {
			st.deepen(TechniqueSingleton)
			if !st.commit(i, st.active[i][0]) {
				st.failed = true
			}
			return true
		}
	}
	return false
}

// progressCellForced commits the sole (clue, rect) pair able to cover an
// uncovered cell, when only one such pair remains across the whole board.
func (st *logicState) progressCellForced() bool {
	n := st.p.R * st.p.C
	for cell := 0; cell < n; cell++ {
		if st.owner[cell] != -1 {
			continue
		}
		foundClue, foundRect, count := -1, Rect{}, 0
		for i := range st.infos {
			if st.isPlaced[i] {
				continue
			}
			for _, r := range st.active[i] {
				if rectContainsCell(r, cell, st.p.C) {
					count++
					foundClue, foundRect = i, r
					if count > 1 {
						break
					}
				}
			}
			if count > 1 {
				break
			}
		}
		if count == 1 {
			st.deepen(TechniqueCellForced)
			if !st.commit(foundClue, foundRect) {
				st.failed = true
			}
			return true
		}
	}
	return false
}

// propagateCheap runs only the non-lookahead techniques to fixpoint. Used
// inside progressContradiction's trials; returns false on contradiction.
func (st *logicState) propagateCheap() bool {
	for {
		if st.solvedAll() {
			return true
		}
		if st.hasContradiction() {
			return false
		}
		if st.progressSingleton() {
			continue
		}
		if st.progressCellForced() {
			continue
		}
		return true // stuck but not contradictory
	}
}

// progressContradiction is a depth-1 proof step: if tentatively committing a
// candidate and propagating to fixpoint (on a clone) proves a contradiction,
// that candidate is eliminated from the real state as impossible. Sound —
// only removes provably-impossible candidates — so it never turns a
// no-guess solve into a guess.
func (st *logicState) progressContradiction() bool {
	for i := range st.infos {
		if st.isPlaced[i] {
			continue
		}
		for ci, rect := range st.active[i] {
			trial := st.clone()
			ok := trial.commit(i, rect)
			if ok {
				ok = trial.propagateCheap()
			}
			if !ok {
				st.active[i] = append(append([]Rect{}, st.active[i][:ci]...), st.active[i][ci+1:]...)
				st.deepen(TechniqueContradiction)
				return true
			}
		}
	}
	return false
}

func (st *logicState) run() bool {
	for {
		if st.solvedAll() {
			return true
		}
		if st.hasContradiction() {
			return false
		}
		if st.progressSingleton() {
			continue
		}
		if st.progressCellForced() {
			continue
		}
		if st.progressContradiction() {
			continue
		}
		return false // stuck: would need to guess
	}
}

func (st *logicState) solution() *Solution {
	return &Solution{Rects: append([]Rect(nil), st.placed...)}
}
