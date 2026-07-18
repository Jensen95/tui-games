package zip

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sort"

	"github.com/Jensen95/tui-games/internal/engine"
)

// ---------------------------------------------------------------------------
// Small grid helpers (index-based; row-major with C columns).
// ---------------------------------------------------------------------------

// serpentine returns the boustrophedon Hamiltonian path over an R×C grid.
// (Mirrors the test fixture helper; duplicated here because that one lives in
// a _test.go file and is not available to non-test builds.)
func serpentine(rows, cols int) []int {
	path := make([]int, 0, rows*cols)
	for r := 0; r < rows; r++ {
		if r%2 == 0 {
			for c := 0; c < cols; c++ {
				path = append(path, r*cols+c)
			}
		} else {
			for c := cols - 1; c >= 0; c-- {
				path = append(path, r*cols+c)
			}
		}
	}
	return path
}

// adjacentIdx reports whether two cell indices are orthogonally adjacent in a
// grid with C columns. Uses row/col arithmetic so it never wraps around edges.
func adjacentIdx(a, b, C int) bool {
	ra, ca := a/C, a%C
	rb, cb := b/C, b%C
	dr := ra - rb
	if dr < 0 {
		dr = -dr
	}
	dc := ca - cb
	if dc < 0 {
		dc = -dc
	}
	return dr+dc == 1
}

// openNeighbors returns the wall-free orthogonal neighbor indices of idx,
// in ascending index order (so DFS is deterministic).
func openNeighbors(p Puzzle, idx int) []int {
	r, c := idx/p.C, idx%p.C
	out := make([]int, 0, 4)
	if r > 0 {
		n := idx - p.C
		if !p.Walls[WallKey(idx, n)] {
			out = append(out, n)
		}
	}
	if c > 0 {
		n := idx - 1
		if !p.Walls[WallKey(idx, n)] {
			out = append(out, n)
		}
	}
	if c < p.C-1 {
		n := idx + 1
		if !p.Walls[WallKey(idx, n)] {
			out = append(out, n)
		}
	}
	if r < p.R-1 {
		n := idx + p.C
		if !p.Walls[WallKey(idx, n)] {
			out = append(out, n)
		}
	}
	// r-C < c-1 < c+1 < r+C, so the append order above is already ascending.
	return out
}

// gridNeighbors returns all in-bounds orthogonal neighbor indices (ignoring
// walls) — used only by the wall-free Hamiltonian path generator.
func gridNeighbors(idx, R, C int) []int {
	r, c := idx/C, idx%C
	out := make([]int, 0, 4)
	if r > 0 {
		out = append(out, idx-C)
	}
	if c > 0 {
		out = append(out, idx-1)
	}
	if c < C-1 {
		out = append(out, idx+1)
	}
	if r < R-1 {
		out = append(out, idx+C)
	}
	return out
}

// startCell returns the cell numbered 1 (the path start).
func startCell(p Puzzle) (int, bool) {
	for cell, num := range p.Waypoint {
		if num == 1 {
			return cell, true
		}
	}
	return 0, false
}

// endCell returns the cell carrying the maximum waypoint number (the path end).
func endCell(p Puzzle) (int, bool) {
	best, bestNum := 0, -1
	for cell, num := range p.Waypoint {
		if num > bestNum {
			bestNum, best = num, cell
		}
	}
	if bestNum < 0 {
		return 0, false
	}
	return best, true
}

// sortedWaypointNumbers returns the waypoint numbers, ascending.
func sortedWaypointNumbers(p Puzzle) []int {
	nums := make([]int, 0, len(p.Waypoint))
	for _, n := range p.Waypoint {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	return nums
}

// ---------------------------------------------------------------------------
// Validation.
// ---------------------------------------------------------------------------

// solvedCheck implements the spec's Solved-state definition.
func solvedCheck(p Puzzle, path []int) bool {
	N := p.R * p.C
	if N == 0 || len(path) != N {
		return false
	}
	seen := make([]bool, N)
	for _, c := range path {
		if c < 0 || c >= N || seen[c] {
			return false
		}
		seen[c] = true
	}
	start, ok := startCell(p)
	if !ok || path[0] != start {
		return false
	}
	end, ok := endCell(p)
	if !ok || path[N-1] != end {
		return false
	}
	for i := 0; i+1 < N; i++ {
		a, b := path[i], path[i+1]
		if !adjacentIdx(a, b, p.C) {
			return false
		}
		if p.Walls[WallKey(a, b)] {
			return false
		}
	}
	wpNums := sortedWaypointNumbers(p)
	enc := make([]int, 0, len(wpNums))
	for _, c := range path {
		if n, has := p.Waypoint[c]; has {
			enc = append(enc, n)
		}
	}
	if len(enc) != len(wpNums) {
		return false
	}
	for i := range enc {
		if enc[i] != wpNums[i] {
			return false
		}
	}
	return true
}

// violationsOf computes all currently-broken rules on a (possibly partial)
// board. Only already-broken rules are reported — an unfinished path that has
// not yet done anything illegal yields none.
func violationsOf(b Board) []engine.Violation {
	p := b.Puzzle
	path := b.Path
	C := p.C
	if len(path) == 0 {
		return nil
	}
	var out []engine.Violation

	// Wrong start.
	if start, ok := startCell(p); ok && path[0] != start {
		out = append(out, engine.Violation{
			Rule:    RuleWrongStart,
			Message: "path must start on the cell numbered 1",
			Cells:   []engine.Cell{engine.CellAt(path[0], C)},
		})
	}

	// Revisits.
	seen := make(map[int]bool, len(path))
	var revisit []engine.Cell
	for _, c := range path {
		if seen[c] {
			revisit = append(revisit, engine.CellAt(c, C))
		}
		seen[c] = true
	}
	if len(revisit) > 0 {
		out = append(out, engine.Violation{
			Rule:    RuleRevisit,
			Message: "a cell is visited more than once",
			Cells:   revisit,
		})
	}

	// Steps: non-adjacency and wall crossings.
	var nonAdj, wallX []engine.Cell
	for i := 0; i+1 < len(path); i++ {
		a, bb := path[i], path[i+1]
		if !adjacentIdx(a, bb, C) {
			nonAdj = append(nonAdj, engine.CellAt(a, C), engine.CellAt(bb, C))
			continue
		}
		if p.Walls[WallKey(a, bb)] {
			wallX = append(wallX, engine.CellAt(a, C), engine.CellAt(bb, C))
		}
	}
	if len(nonAdj) > 0 {
		out = append(out, engine.Violation{
			Rule:    RuleNonAdjacentStep,
			Message: "consecutive cells must be orthogonally adjacent (no diagonals or jumps)",
			Cells:   nonAdj,
		})
	}
	if len(wallX) > 0 {
		out = append(out, engine.Violation{
			Rule:    RuleWallCrossing,
			Message: "a step crosses a wall",
			Cells:   wallX,
		})
	}

	// Waypoint order: the numbered cells encountered so far must be exactly the
	// smallest waypoints, in ascending order (a prefix of the sorted numbers).
	wpNums := sortedWaypointNumbers(p)
	var enc []int
	var encCells []engine.Cell
	for _, c := range path {
		if n, has := p.Waypoint[c]; has {
			enc = append(enc, n)
			encCells = append(encCells, engine.CellAt(c, C))
		}
	}
	for i := range enc {
		if i >= len(wpNums) || enc[i] != wpNums[i] {
			out = append(out, engine.Violation{
				Rule:    RuleWaypointOrder,
				Message: "numbered cells must be visited in ascending order",
				Cells:   encCells,
			})
			break
		}
	}

	return out
}

// ---------------------------------------------------------------------------
// Complete solver: enumerate Hamiltonian paths (bounded count).
// ---------------------------------------------------------------------------

// feasibleCompletion is a necessary-condition prune: given the current visited
// set and head cell cur, can the remaining unvisited cells still form a
// Hamiltonian path ending at end? It checks connectivity of the remaining
// cells plus degree lower-bounds (interior vertices need degree >= 2).
func feasibleCompletion(p Puzzle, visited []bool, cur, end, visitedCount int) bool {
	N := len(visited)
	remaining := N - visitedCount
	if remaining == 0 {
		return cur == end
	}
	if visited[end] {
		return false // the end cell must be visited last
	}
	// Degree lower-bounds on remaining vertices.
	for x := 0; x < N; x++ {
		if visited[x] {
			continue
		}
		deg := 0
		for _, nb := range openNeighbors(p, x) {
			if !visited[nb] || nb == cur {
				deg++
			}
		}
		if x == end {
			if deg < 1 {
				return false
			}
		} else if deg < 2 {
			return false
		}
	}
	// The head must have somewhere to go.
	curDeg := 0
	for _, nb := range openNeighbors(p, cur) {
		if !visited[nb] {
			curDeg++
		}
	}
	if curDeg < 1 {
		return false
	}
	// Connectivity: every remaining cell must be reachable from cur through
	// unvisited cells.
	stack := make([]int, 0, remaining)
	local := make([]bool, N)
	reach := 0
	for _, nb := range openNeighbors(p, cur) {
		if !visited[nb] && !local[nb] {
			local[nb] = true
			reach++
			stack = append(stack, nb)
		}
	}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, nb := range openNeighbors(p, x) {
			if !visited[nb] && !local[nb] {
				local[nb] = true
				reach++
				stack = append(stack, nb)
			}
		}
	}
	if reach != remaining || !local[end] {
		return false
	}
	return true
}

// walkSolutions invokes visit on every Hamiltonian-path solution of p (start at
// cell 1, end at the max waypoint, waypoints ascending, wall-respecting). It
// stops early when visit returns true. The passed slice is reused — copy it if
// you need to keep it.
func walkSolutions(p Puzzle, visit func(path []int) bool) {
	N := p.R * p.C
	if N == 0 {
		return
	}
	start, ok := startCell(p)
	if !ok {
		return
	}
	end, ok := endCell(p)
	if !ok {
		return
	}
	wpNums := sortedWaypointNumbers(p)
	if n, has := p.Waypoint[start]; !has || len(wpNums) == 0 || n != wpNums[0] {
		return // start must be the cell numbered 1
	}

	visited := make([]bool, N)
	path := make([]int, 0, N)
	visited[start] = true
	path = append(path, start)

	stop := false
	var dfs func(cur, ptr int)
	dfs = func(cur, ptr int) {
		if stop {
			return
		}
		if len(path) == N {
			if cur == end && ptr == len(wpNums) {
				if visit(path) {
					stop = true
				}
			}
			return
		}
		if !feasibleCompletion(p, visited, cur, end, len(path)) {
			return
		}
		for _, nb := range openNeighbors(p, cur) {
			if visited[nb] {
				continue
			}
			if nb == end && len(path)+1 < N {
				continue // end must be last
			}
			np := ptr
			if w, has := p.Waypoint[nb]; has {
				if ptr >= len(wpNums) || w != wpNums[ptr] {
					continue
				}
				np = ptr + 1
			}
			visited[nb] = true
			path = append(path, nb)
			dfs(nb, np)
			path = path[:len(path)-1]
			visited[nb] = false
			if stop {
				return
			}
		}
	}
	dfs(start, 1)
}

// enumerateSolutions counts solutions up to cap. When wantFirst is set it also
// returns the first solution found (in ascending-neighbor DFS order).
func enumerateSolutions(p Puzzle, cap int, wantFirst bool) (int, []int) {
	if cap <= 0 {
		return 0, nil
	}
	count := 0
	var first []int
	walkSolutions(p, func(path []int) bool {
		count++
		if wantFirst && first == nil {
			first = append([]int(nil), path...)
		}
		return count >= cap
	})
	return count, first
}

// firstAltSolution returns the first solution of p whose path differs from
// avoid, if one exists.
func firstAltSolution(p Puzzle, avoid []int) ([]int, bool) {
	var alt []int
	walkSolutions(p, func(path []int) bool {
		if !equalInts(path, avoid) {
			alt = append([]int(nil), path...)
			return true
		}
		return false
	})
	return alt, alt != nil
}

// ---------------------------------------------------------------------------
// Logic (no-guess) solver: forced single-continuation with feasibility filter.
// ---------------------------------------------------------------------------

// logicSolvePath greedily extends the path only when exactly one continuation
// keeps the puzzle feasible. It never guesses: if two or more continuations are
// feasible it stops and reports the puzzle as not closed by pure logic.
func logicSolvePath(p Puzzle) ([]int, bool) {
	N := p.R * p.C
	if N == 0 {
		return nil, false
	}
	start, ok := startCell(p)
	if !ok {
		return nil, false
	}
	end, ok := endCell(p)
	if !ok {
		return nil, false
	}
	wpNums := sortedWaypointNumbers(p)
	if n, has := p.Waypoint[start]; !has || len(wpNums) == 0 || n != wpNums[0] {
		return nil, false
	}

	visited := make([]bool, N)
	visited[start] = true
	path := make([]int, 0, N)
	path = append(path, start)
	ptr := 1

	for len(path) < N {
		cur := path[len(path)-1]
		cands := make([]int, 0, 4)
		for _, nb := range openNeighbors(p, cur) {
			if visited[nb] {
				continue
			}
			if nb == end && len(path)+1 < N {
				continue
			}
			if w, has := p.Waypoint[nb]; has {
				if ptr >= len(wpNums) || w != wpNums[ptr] {
					continue
				}
			}
			visited[nb] = true
			feasible := feasibleCompletion(p, visited, nb, end, len(path)+1)
			visited[nb] = false
			if !feasible {
				continue
			}
			cands = append(cands, nb)
		}
		if len(cands) != 1 {
			return path, false // dead end (0) or a branch point (>1): needs guessing
		}
		nb := cands[0]
		visited[nb] = true
		path = append(path, nb)
		if _, has := p.Waypoint[nb]; has {
			ptr++
		}
	}
	if !solvedCheck(p, path) {
		return path, false
	}
	return path, true
}

// ---------------------------------------------------------------------------
// Random Hamiltonian path (backbite perturbation of a serpentine seed).
// ---------------------------------------------------------------------------

func shuffleInts(s []int, r *rand.Rand) {
	for i := len(s) - 1; i > 0; i-- {
		j := r.IntN(i + 1)
		s[i], s[j] = s[j], s[i]
	}
}

// randomHamiltonian returns a uniform-ish random Hamiltonian path over the
// R×C grid via repeated backbite moves on a serpentine seed. Backbite keeps the
// path Hamiltonian at every step, so this is fast and never fails.
func randomHamiltonian(R, C int, r *rand.Rand) []int {
	N := R * C
	path := serpentine(R, C)
	if N <= 2 {
		return path
	}
	pos := make([]int, N)
	for i, c := range path {
		pos[c] = i
	}
	moves := 8 * N
	if moves < 64 {
		moves = 64
	}
	for m := 0; m < moves; m++ {
		if r.IntN(2) == 0 {
			// Backbite on the head.
			head := path[0]
			nbs := gridNeighbors(head, R, C)
			w := nbs[r.IntN(len(nbs))]
			k := pos[w]
			if k <= 1 {
				continue
			}
			reverseSeg(path, 0, k) // reverse indices [0,k)
		} else {
			// Backbite on the tail.
			tail := path[N-1]
			nbs := gridNeighbors(tail, R, C)
			w := nbs[r.IntN(len(nbs))]
			k := pos[w]
			if k >= N-2 {
				continue
			}
			reverseSeg(path, k+1, N) // reverse indices [k+1,N)
		}
		for i, c := range path {
			pos[c] = i
		}
	}
	return path
}

func reverseSeg(s []int, lo, hi int) {
	for lo < hi-1 {
		s[lo], s[hi-1] = s[hi-1], s[lo]
		lo++
		hi--
	}
}

// ---------------------------------------------------------------------------
// Generation (solution-first, densify-to-unique).
// ---------------------------------------------------------------------------

func sizeFor(diff engine.Difficulty) (int, int) {
	switch diff {
	case engine.Easy:
		return 5, 5
	default:
		return 6, 6
	}
}

func targetWaypoints(diff engine.Difficulty, N int) int {
	var k int
	switch diff {
	case engine.Easy:
		k = N * 55 / 100
	case engine.Medium:
		k = N * 40 / 100
	default: // Hard, Expert
		k = N * 30 / 100
	}
	if k < 2 {
		k = 2
	}
	return k
}

func makePuzzleFromChosen(path []int, R, C int, chosen []bool, walls map[[2]int]bool, diff engine.Difficulty) Puzzle {
	wp := make(map[int]int, len(path))
	num := 0
	for pos := 0; pos < len(path); pos++ {
		if chosen[pos] {
			num++
			wp[path[pos]] = num
		}
	}
	w := make(map[[2]int]bool, len(walls))
	for k := range walls {
		w[k] = true
	}
	return Puzzle{R: R, C: C, Waypoint: wp, Walls: w, SeedVal: 0, Diff: diff}
}

// edgeInAltNotPath returns a wall key on an edge the alternative solution uses
// but the intended path does not. Two distinct Hamiltonian solutions over the
// same cells always differ by at least one edge, and any such edge is by
// definition off the intended path — so walling it kills the alternative
// without ever blocking the intended solution.
func edgeInAltNotPath(alt, path []int) ([2]int, bool) {
	pathEdges := make(map[[2]int]bool, len(path))
	for i := 0; i+1 < len(path); i++ {
		pathEdges[WallKey(path[i], path[i+1])] = true
	}
	for i := 0; i+1 < len(alt); i++ {
		k := WallKey(alt[i], alt[i+1])
		if !pathEdges[k] {
			return k, true
		}
	}
	return [2]int{}, false
}

// buildUnique carves a puzzle out of a fixed solution path. It seeds a subset
// of waypoints, then forces uniqueness by repeatedly finding an alternative
// solution and walling an off-solution edge it relies on (the spec's approach).
// If the puzzle is unique but the logic solver can't yet close it, it adds a
// waypoint. Full numbering is always unique + logic-closable, so this
// terminates.
func buildUnique(path []int, R, C int, diff engine.Difficulty, r *rand.Rand) (Puzzle, bool) {
	N := R * C

	interior := make([]int, 0, N-2)
	for i := 1; i < N-1; i++ {
		interior = append(interior, i)
	}
	shuffleInts(interior, r)

	chosen := make([]bool, N)
	chosen[0] = true
	chosen[N-1] = true
	startK := targetWaypoints(diff, N) - 2
	if startK < 0 {
		startK = 0
	}
	next := 0
	for ; next < startK && next < len(interior); next++ {
		chosen[interior[next]] = true
	}

	walls := make(map[[2]int]bool)

	const maxIter = 4000
	for iter := 0; iter < maxIter; iter++ {
		p := makePuzzleFromChosen(path, R, C, chosen, walls, diff)
		if !solvedCheck(p, path) {
			return Puzzle{}, false // should never happen
		}
		if alt, has := firstAltSolution(p, path); has {
			// Kill this alternative with a targeted off-solution wall.
			if edge, ok := edgeInAltNotPath(alt, path); ok && !walls[edge] {
				walls[edge] = true
				continue
			}
			// Fallback: constrain further with an extra waypoint.
			if next < len(interior) {
				chosen[interior[next]] = true
				next++
				continue
			}
			return Puzzle{}, false
		}
		// Unique. Confirm the logic solver closes it.
		if lp, closed := logicSolvePath(p); closed && equalInts(lp, path) {
			return p, true
		}
		if next < len(interior) {
			chosen[interior[next]] = true
			next++
			continue
		}
		return Puzzle{}, false // unreachable: full numbering is unique + closable
	}
	return Puzzle{}, false
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func generateZip(diff engine.Difficulty, r *rand.Rand) (Puzzle, Solution, error) {
	R, C := sizeFor(diff)
	const maxAttempts = 200
	for attempt := 0; attempt < maxAttempts; attempt++ {
		path := randomHamiltonian(R, C, r)
		if p, ok := buildUnique(path, R, C, diff, r); ok {
			return p, Solution{Path: append([]int(nil), path...)}, nil
		}
	}
	return Puzzle{}, Solution{}, fmt.Errorf("zip: generation failed after %d attempts", maxAttempts)
}

// ---------------------------------------------------------------------------
// Fingerprint / canonicalization.
// ---------------------------------------------------------------------------

// transformForCanon geometrically maps p under transform tr (waypoint numbers
// preserved; numbering fixes path direction so no relabeling is applied).
func transformForCanon(p Puzzle, tr engine.Transform) Puzzle {
	newR, newC := tr.Dims(p.R, p.C)
	remap := func(idx int) int {
		c := engine.CellAt(idx, p.C)
		nc := tr.Apply(c, p.R, p.C)
		return engine.Index(nc, newC)
	}
	wp := make(map[int]int, len(p.Waypoint))
	for idx, num := range p.Waypoint {
		wp[remap(idx)] = num
	}
	walls := make(map[[2]int]bool, len(p.Walls))
	for edge := range p.Walls {
		walls[WallKey(remap(edge[0]), remap(edge[1]))] = true
	}
	return Puzzle{R: newR, C: newC, Waypoint: wp, Walls: walls}
}

// serializePuzzle produces a fixed-length, order-independent byte encoding of a
// puzzle's geometry (dims, per-cell waypoint numbers, per-cell right/down wall
// bits). Distinct non-symmetric puzzles serialize differently.
func serializePuzzle(p Puzzle) []byte {
	n := p.R * p.C
	buf := make([]byte, 0, 2+n+2*n)
	buf = append(buf, byte(p.R), byte(p.C))
	for i := 0; i < n; i++ {
		buf = append(buf, byte(p.Waypoint[i]))
	}
	for i := 0; i < n; i++ {
		row, col := i/p.C, i%p.C
		var rb, db byte
		if col+1 < p.C && p.Walls[WallKey(i, i+1)] {
			rb = 1
		}
		if row+1 < p.R && p.Walls[WallKey(i, i+p.C)] {
			db = 1
		}
		buf = append(buf, rb, db)
	}
	return buf
}

func canonicalBytes(p Puzzle) []byte {
	cands := make([][]byte, 0, len(engine.AllTransforms))
	for _, tr := range engine.AllTransforms {
		cands = append(cands, serializePuzzle(transformForCanon(p, tr)))
	}
	return engine.CanonicalMin(cands)
}

// ---------------------------------------------------------------------------
// Encode / Decode (clues only — the solution never leaks into the encoding).
// ---------------------------------------------------------------------------

type wirePuzzle struct {
	R         int      `json:"r"`
	C         int      `json:"c"`
	Waypoints [][2]int `json:"waypoints"` // {cell, number}, sorted by cell
	Walls     [][2]int `json:"walls"`     // {a, b} with a<b, sorted
	Diff      int      `json:"diff"`
}

// Encode serializes a puzzle's clues to stable JSON. It never includes the
// solution.
func Encode(p Puzzle) []byte {
	w := wirePuzzle{R: p.R, C: p.C, Diff: int(p.Diff)}

	cells := make([]int, 0, len(p.Waypoint))
	for cell := range p.Waypoint {
		cells = append(cells, cell)
	}
	sort.Ints(cells)
	for _, cell := range cells {
		w.Waypoints = append(w.Waypoints, [2]int{cell, p.Waypoint[cell]})
	}

	walls := make([][2]int, 0, len(p.Walls))
	for k := range p.Walls {
		walls = append(walls, k)
	}
	sort.Slice(walls, func(i, j int) bool {
		if walls[i][0] != walls[j][0] {
			return walls[i][0] < walls[j][0]
		}
		return walls[i][1] < walls[j][1]
	})
	w.Walls = walls

	b, _ := json.Marshal(w)
	return b
}

// Decode reverses Encode.
func Decode(data []byte) (Puzzle, error) {
	var w wirePuzzle
	if err := json.Unmarshal(data, &w); err != nil {
		return Puzzle{}, err
	}
	if w.R <= 0 || w.C <= 0 {
		return Puzzle{}, fmt.Errorf("zip: bad dimensions %dx%d", w.R, w.C)
	}
	p := Puzzle{
		R:        w.R,
		C:        w.C,
		Waypoint: make(map[int]int, len(w.Waypoints)),
		Walls:    make(map[[2]int]bool, len(w.Walls)),
		Diff:     engine.Difficulty(w.Diff),
	}
	n := w.R * w.C
	for _, wp := range w.Waypoints {
		if wp[0] < 0 || wp[0] >= n {
			return Puzzle{}, fmt.Errorf("zip: waypoint cell %d out of range", wp[0])
		}
		p.Waypoint[wp[0]] = wp[1]
	}
	for _, e := range w.Walls {
		p.Walls[WallKey(e[0], e[1])] = true
	}
	return p, nil
}

// verifyEncoded decodes and re-checks the generation invariant independently.
func verifyEncoded(encoded []byte) error {
	p, err := Decode(encoded)
	if err != nil {
		return err
	}
	n := p.R * p.C
	if n < 2 {
		return fmt.Errorf("zip: grid too small (%d cells)", n)
	}
	if _, ok := startCell(p); !ok {
		return fmt.Errorf("zip: no start (cell numbered 1)")
	}
	if _, ok := endCell(p); !ok {
		return fmt.Errorf("zip: no end waypoint")
	}
	// Waypoint numbers must be contiguous 1..K.
	k := len(p.Waypoint)
	if k < 2 {
		return fmt.Errorf("zip: need at least 2 waypoints, have %d", k)
	}
	seen := make([]bool, k+1)
	for _, num := range p.Waypoint {
		if num < 1 || num > k {
			return fmt.Errorf("zip: waypoint number %d outside 1..%d", num, k)
		}
		if seen[num] {
			return fmt.Errorf("zip: duplicate waypoint number %d", num)
		}
		seen[num] = true
	}
	if cnt, _ := enumerateSolutions(p, 2, false); cnt != 1 {
		return fmt.Errorf("zip: puzzle is not uniquely solvable (found %d solutions)", cnt)
	}
	return nil
}
