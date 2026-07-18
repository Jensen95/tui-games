package zip

// crosscheck.go: a second, independently-authored complete solver used only
// to cross-validate the primary solver in impl.go (walkSolutions /
// feasibleCompletion). Built directly from docs/plan/games/zip.md's rules
// and Solved-state definition, not from impl.go's structure:
//
//   - Neighbor order is reversed (down, right, up, left instead of the
//     primary's up, left, right, down), so the two DFS's explore the search
//     tree in a different order and would surface a first-solution bug
//     under TestSolver_Solve_GoldenPuzzle-style checks even if by luck one
//     had a directional blind spot.
//   - The only pruning is a breadth-first reachability count from the
//     current head over the unvisited cells (does every remaining cell,
//     including the end cell, sit in one connected unvisited component
//     reachable from the head?). It deliberately omits the primary's
//     degree-lower-bound heuristic (interior cells need >=2 open unvisited
//     neighbors) — that's a legitimate speed optimization but not a
//     correctness requirement, and skipping it means this solver's
//     correctness rests on different logic.
//   - Completed candidates are independently re-verified against the raw
//     spec definition (crossIsValidComplete) rather than trusted from the
//     search bookkeeping, and rather than delegating to impl.go's
//     solvedCheck.
//
// If the primary solver and this one ever disagree on a solution count or
// the unique solution's path, that is a genuine, actionable bug signal.

// crossWaypointOrder returns the puzzle's waypoint numbers in ascending
// order. Note the numbers present need not be contiguous (a hand-built
// fixture may label only the two endpoints, e.g. 1 and 6, leaving the
// interior unlabeled) — only their relative order matters for path
// validation, so this collects whatever numbers actually appear rather than
// assuming a dense 1..K range.
func crossWaypointOrder(p Puzzle) []int {
	nums := make([]int, 0, len(p.Waypoint))
	for _, n := range p.Waypoint {
		nums = append(nums, n)
	}
	for i := 1; i < len(nums); i++ {
		for j := i; j > 0 && nums[j-1] > nums[j]; j-- {
			nums[j-1], nums[j] = nums[j], nums[j-1]
		}
	}
	return nums
}

// crossEndpoints returns the cell numbered 1 and the cell numbered K (the
// maximum waypoint number), independently of impl.go's startCell/endCell.
func crossEndpoints(p Puzzle) (start, end int, ok bool) {
	startFound, endFound := false, false
	maxNum := 0
	for cell, num := range p.Waypoint {
		if num == 1 {
			start = cell
			startFound = true
		}
		if num > maxNum {
			maxNum = num
			end = cell
			endFound = true
		}
	}
	return start, end, startFound && endFound
}

// crossNeighbors returns idx's wall-free orthogonal neighbors in a
// deliberately different order than impl.go's openNeighbors: down, right,
// up, left (the primary emits up, left, right, down).
func crossNeighbors(p Puzzle, idx int) []int {
	r, c := idx/p.C, idx%p.C
	out := make([]int, 0, 4)
	if r < p.R-1 {
		n := idx + p.C
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
	return out
}

// crossReachesAllRemaining runs a breadth-first flood fill from head over
// unvisited cells and reports whether every unvisited cell (including end)
// is reachable from head without crossing walls or entering visited cells.
// This is the sole pruning rule this solver uses: no degree heuristics.
func crossReachesAllRemaining(p Puzzle, visited []bool, head, end, remaining int) bool {
	if remaining == 0 {
		return head == end
	}
	if visited[end] {
		return false
	}
	N := len(visited)
	seen := make([]bool, N)
	queue := make([]int, 0, remaining)
	for _, nb := range crossNeighbors(p, head) {
		if !visited[nb] && !seen[nb] {
			seen[nb] = true
			queue = append(queue, nb)
		}
	}
	count := 0
	for qi := 0; qi < len(queue); qi++ {
		cur := queue[qi]
		count++
		for _, nb := range crossNeighbors(p, cur) {
			if !visited[nb] && !seen[nb] {
				seen[nb] = true
				queue = append(queue, nb)
			}
		}
	}
	return count == remaining && seen[end]
}

// crossIsValidComplete independently re-checks a full-length path against
// the spec's Solved-state definition (Hamiltonian, orthogonal, wall-free,
// starts at 1, waypoints strictly ascending in path order), without calling
// impl.go's solvedCheck.
func crossIsValidComplete(p Puzzle, path []int) bool {
	N := p.R * p.C
	if len(path) != N {
		return false
	}
	seenCell := make([]bool, N)
	for _, c := range path {
		if c < 0 || c >= N || seenCell[c] {
			return false
		}
		seenCell[c] = true
	}
	start, end, ok := crossEndpoints(p)
	if !ok || path[0] != start || path[N-1] != end {
		return false
	}
	for i := 0; i+1 < N; i++ {
		a, b := path[i], path[i+1]
		ra, ca := a/p.C, a%p.C
		rb, cb := b/p.C, b%p.C
		dr, dc := ra-rb, ca-cb
		if dr < 0 {
			dr = -dr
		}
		if dc < 0 {
			dc = -dc
		}
		if dr+dc != 1 {
			return false
		}
		if p.Walls[WallKey(a, b)] {
			return false
		}
	}
	wantOrder := crossWaypointOrder(p)
	var gotOrder []int
	for _, c := range path {
		if n, has := p.Waypoint[c]; has {
			gotOrder = append(gotOrder, n)
		}
	}
	if len(gotOrder) != len(wantOrder) {
		return false
	}
	for i := range gotOrder {
		if gotOrder[i] != wantOrder[i] {
			return false
		}
	}
	return true
}

// crossCountSolutions enumerates Hamiltonian-path solutions of p up to cap
// (min(#solutions, cap)) via an independently-structured DFS, returning the
// count and — when at least one exists — a copy of the first solution path
// found in this solver's own (reversed) exploration order.
func crossCountSolutions(p Puzzle, cap int) (int, []int) {
	if cap <= 0 {
		return 0, nil
	}
	N := p.R * p.C
	if N == 0 {
		return 0, nil
	}
	start, end, ok := crossEndpoints(p)
	if !ok {
		return 0, nil
	}
	wantOrder := crossWaypointOrder(p)
	if len(wantOrder) == 0 {
		return 0, nil
	}
	if n, has := p.Waypoint[start]; !has || n != wantOrder[0] {
		return 0, nil
	}

	visited := make([]bool, N)
	path := make([]int, 0, N)
	visited[start] = true
	path = append(path, start)

	count := 0
	var first []int

	var search func(head, ptr int) bool // returns true => caller should stop (cap reached)
	search = func(head, ptr int) bool {
		if len(path) == N {
			if head != end || ptr != len(wantOrder) {
				return false
			}
			if !crossIsValidComplete(p, path) {
				return false
			}
			count++
			if first == nil {
				first = append([]int(nil), path...)
			}
			return count >= cap
		}
		if !crossReachesAllRemaining(p, visited, head, end, N-len(path)) {
			return false
		}
		for _, nb := range crossNeighbors(p, head) {
			if visited[nb] {
				continue
			}
			if nb == end && len(path)+1 < N {
				continue
			}
			np := ptr
			if w, has := p.Waypoint[nb]; has {
				if ptr >= len(wantOrder) || w != wantOrder[ptr] {
					continue
				}
				np = ptr + 1
			}
			visited[nb] = true
			path = append(path, nb)
			stop := search(nb, np)
			path = path[:len(path)-1]
			visited[nb] = false
			if stop {
				return true
			}
		}
		return false
	}
	search(start, 1)
	return count, first
}

// crossSolve returns one solution to p (if any) found by this independent
// solver.
func crossSolve(p Puzzle) ([]int, bool) {
	count, first := crossCountSolutions(p, 1)
	if count == 0 {
		return nil, false
	}
	return first, true
}
