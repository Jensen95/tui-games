package queens

import (
	"errors"
	"math/rand/v2"

	"github.com/Jensen95/tui-games/internal/engine"
)

// Generator produces fresh Queens puzzles. See engine.Generator[Puzzle, Solution].
type Generator struct{}

// NewGenerator returns a Queens puzzle generator.
func NewGenerator() *Generator { return &Generator{} }

var _ engine.Generator[Puzzle, Solution] = (*Generator)(nil)

const (
	minN = 5
	maxN = 11

	genAttempts   = 400 // outer solution+region attempts
	reshapeRounds = 800 // uniqueness-forcing moves per attempt
	attemptsPerN  = 6   // failed attempts before stepping N down (worst-case cap)
)

// Generate returns a puzzle guaranteed valid and uniquely solvable at ~diff,
// together with its solution. All randomness (including the grid size N, which
// the engine.Generator interface leaves to the implementation — see
// docs/plan/games/queens.md for the supported 5..11 range) comes from r.
//
// Strategy is solution-first per docs 02: build a full valid placement, grow
// N connected regions each seeded by exactly one queen (which guarantees
// one-queen-per-region), then reshape region borders to force uniqueness,
// re-checking the complete solver after each move. The generation invariant
// (valid, exactly one solution, logic-solvable) is enforced before returning.
func (g *Generator) Generate(diff engine.Difficulty, r *rand.Rand) (Puzzle, Solution, error) {
	n := minN + r.IntN(maxN-minN+1)
	solver := NewSolver()

	fails := 0
	for attempt := 0; attempt < genAttempts; attempt++ {
		// Safety valve: a few sizes have rare seeds whose reshape keeps hitting
		// local minima. After enough failures, step N down — still fully
		// deterministic (driven by r) — so worst-case latency stays bounded.
		if fails >= attemptsPerN && n > minN {
			n--
			fails = 0
		}
		sol, ok := randomPlacement(n, r)
		if !ok {
			fails++
			continue
		}
		region := growRegions(n, sol, r)

		p := Puzzle{N: n, Region: region, SeedV: seedFromRand(r), DiffV: diff}

		if !makeUnique(&p, sol, solver, r) {
			fails++
			continue
		}
		// makeUnique guarantees CountSolutions(p,2)==1 with sol the survivor.
		if _, closed, _ := solver.LogicSolve(p); !closed {
			fails++
			continue
		}
		return p, sol, nil
	}
	return Puzzle{}, Solution{}, errors.New("queens: exhausted generation attempts")
}

// seedFromRand pulls a stable per-puzzle seed value off r so the recorded
// Puzzle.Seed reflects this generation. It consumes one draw; kept small so
// determinism (same source state ⇒ same value) holds.
func seedFromRand(r *rand.Rand) int64 {
	return int64(r.Uint64() >> 1)
}

// randomPlacement builds a random permutation placement (one queen per row and
// column) with no two queens 8-neighbor adjacent. Returns ok=false if the
// randomized attempts don't find one (caller retries).
func randomPlacement(n int, r *rand.Rand) (Solution, bool) {
	for try := 0; try < 200; try++ {
		cols := r.Perm(n)
		good := true
		for row := 1; row < n && good; row++ {
			if absInt(cols[row]-cols[row-1]) <= 1 {
				good = false
			}
		}
		if good {
			return Solution{N: n, QueenAt: cols}, true
		}
	}
	return Solution{}, false
}

// growRegions seeds each region at its queen cell (region id == the queen's
// row) and flood-grows the remaining cells by repeatedly attaching an
// uncolored cell to a randomly chosen colored neighbor. Every region stays
// 4-connected and contains exactly one queen.
func growRegions(n int, sol Solution, r *rand.Rand) []int {
	region := make([]int, n*n)
	for i := range region {
		region[i] = -1
	}
	for row := 0; row < n; row++ {
		region[row*n+sol.QueenAt[row]] = row
	}
	remaining := n*n - n
	for remaining > 0 {
		// Collect uncolored cells adjacent to a colored cell.
		type opt struct {
			idx int
			reg int
		}
		var opts []opt
		for i := 0; i < n*n; i++ {
			if region[i] != -1 {
				continue
			}
			cell := engine.CellAt(i, n)
			for _, nb := range engine.Neighbors4(cell, n, n) {
				rg := region[engine.Index(nb, n)]
				if rg != -1 {
					opts = append(opts, opt{idx: i, reg: rg})
				}
			}
		}
		if len(opts) == 0 {
			break // shouldn't happen on a connected grid
		}
		pick := opts[r.IntN(len(opts))]
		region[pick.idx] = pick.reg
		remaining--
	}
	return region
}

// makeUnique reshapes p.Region until the complete solver reports exactly one
// solution — guaranteed to be sol, since every move preserves sol and only
// removes competing placements (moving an alternate's queen cell into another
// region puts two alternate-queens in that region, invalidating the alternate,
// while never adding a queen to any sol-region). Returns false if it can't
// converge (caller regrows).
//
// Two phases: while the (capped) solution count is large, greedily apply the
// kill move that removes the most sampled alternates (fast bulk reduction);
// once the count is small, target the single remaining alternate and remove it
// with a connectivity-repairing kill that always exists, so the loop converges
// to a unique board even when the ordinary boundary move is blocked.
func makeUnique(p *Puzzle, sol Solution, solver *Solver, r *rand.Rand) bool {
	bestBulk := 1 << 30 // best (lowest) count seen while in the bulk phase
	stagnant := 0       // consecutive bulk rounds with no new best
	for round := 0; round < reshapeRounds; round++ {
		cur := solver.CountSolutions(*p, bulkThreshold+1)
		if cur == 1 {
			return true
		}
		alts := findAlternates(*p, sol, altSampleSize)
		if len(alts) == 0 {
			return false // count>1 with no alternate is impossible; guard anyway
		}

		if cur > bulkThreshold {
			// Bulk: too many solutions to measure cheaply. Apply the
			// highest-frequency connectivity-safe kill (fast, trends down).
			// If the count stops improving, the descent has plateaued above
			// the endgame window — bail out and let the caller regrow.
			if cur < bestBulk {
				bestBulk = cur
				stagnant = 0
			} else if stagnant++; stagnant > bulkPatience {
				return false
			}
			if m, ok := greedyMove(*p, sol, alts, r); ok {
				p.Region[m.idx] = m.newReg
				continue
			}
			// No simple move: fall through to the guaranteed endgame kill.
		}
		// Endgame: strictly monotone. Apply only a move verified to lower the
		// (small) solution count, so the loop can never oscillate. Try cheap
		// boundary kills first, then repair-based kills of a specific alternate.
		if reduceOnce(p, sol, alts, cur, solver, r) {
			continue
		}
		return false // genuine local minimum: caller regrows
	}
	return solver.CountSolutions(*p, 2) == 1
}

const (
	altSampleSize = 48
	bulkThreshold = 150
	bulkPatience  = 8
	endgameProbes = 24
)

// reduceOnce applies one region edit that strictly lowers the solution count
// (currently cur), preferring cheap boundary kills and falling back to
// connectivity-repairing kills of a sampled alternate. Returns false if no
// count-reducing edit was found.
func reduceOnce(p *Puzzle, sol Solution, alts []Solution, cur int, solver *Solver, r *rand.Rand) bool {
	moves := safeKillMoves(*p, sol, alts)
	r.Shuffle(len(moves), func(i, j int) { moves[i], moves[j] = moves[j], moves[i] })
	tried := 0
	for _, m := range moves {
		if tried >= endgameProbes {
			break
		}
		tried++
		old := p.Region[m.idx]
		p.Region[m.idx] = m.newReg
		if solver.CountSolutions(*p, cur) < cur {
			return true
		}
		p.Region[m.idx] = old
	}
	// Repair-based kills: guaranteed to remove the targeted alternate.
	for i, alt := range alts {
		if i >= 4 {
			break
		}
		trialRegion := append([]int(nil), p.Region...)
		trial := Puzzle{N: p.N, Region: trialRegion}
		if killAlternate(&trial, sol, alt, r) && solver.CountSolutions(trial, cur) < cur {
			p.Region = trialRegion
			return true
		}
	}
	return false
}

// greedyMove picks a connectivity-safe kill move whose cell is a queen in the
// most sampled alternates, so applying it removes the largest batch at once.
// Cells are ranked by frequency and connectivity is checked lazily (only for
// the best few), keeping each bulk round cheap.
func greedyMove(p Puzzle, sol Solution, alts []Solution, r *rand.Rand) (regionMove, bool) {
	n := p.N
	freq := make([]int, n*n)
	for _, alt := range alts {
		for row := 0; row < n; row++ {
			col := alt.QueenAt[row]
			if col != sol.QueenAt[row] {
				freq[row*n+col]++
			}
		}
	}
	// Selection-sort the touched cells by descending frequency, breaking ties
	// randomly; walk them and take the first with a safe boundary move.
	type cf struct{ idx, f int }
	var cells []cf
	for idx, f := range freq {
		if f > 0 {
			cells = append(cells, cf{idx, f})
		}
	}
	r.Shuffle(len(cells), func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })
	for i := 0; i < len(cells); i++ {
		best := i
		for j := i + 1; j < len(cells); j++ {
			if cells[j].f > cells[best].f {
				best = j
			}
		}
		cells[i], cells[best] = cells[best], cells[i]
		idx := cells[i].idx
		from := p.Region[idx]
		if !regionStaysConnectedWithout(p.Region, n, from, idx) {
			continue
		}
		for _, nb := range engine.Neighbors4(engine.CellAt(idx, n), n, n) {
			if rg := p.Region[engine.Index(nb, n)]; rg != from {
				return regionMove{idx: idx, newReg: rg}, true
			}
		}
	}
	return regionMove{}, false
}

// safeKillMoves lists the connectivity-safe boundary kill moves for every
// alternate-queen cell across the sampled alternates.
func safeKillMoves(p Puzzle, sol Solution, alts []Solution) []regionMove {
	n := p.N
	seenCell := make(map[int]bool)
	var moves []regionMove
	for _, alt := range alts {
		for row := 0; row < n; row++ {
			col := alt.QueenAt[row]
			if col == sol.QueenAt[row] {
				continue
			}
			idx := row*n + col
			if seenCell[idx] {
				continue
			}
			seenCell[idx] = true
			from := p.Region[idx]
			if !regionStaysConnectedWithout(p.Region, n, from, idx) {
				continue
			}
			seenReg := map[int]bool{}
			for _, nb := range engine.Neighbors4(engine.CellAt(idx, n), n, n) {
				rg := p.Region[engine.Index(nb, n)]
				if rg == from || seenReg[rg] {
					continue
				}
				seenReg[rg] = true
				moves = append(moves, regionMove{idx: idx, newReg: rg})
			}
		}
	}
	return moves
}

// killAlternate removes the alternate solution alt while preserving sol. It
// moves one of alt's queen cells (which is never a sol queen) into a
// neighboring region, doubling that region's alt-queens so alt becomes invalid.
// If the move would disconnect the donor region it repairs connectivity by
// reassigning the orphaned fragment to an adjacent region. Returns false only
// if alt has no queen cell with a foreign neighbor (extremely rare; regrow).
func killAlternate(p *Puzzle, sol, alt Solution, r *rand.Rand) bool {
	n := p.N
	type cand struct {
		idx    int
		newReg int
		safe   bool
	}
	var cands []cand
	for row := 0; row < n; row++ {
		col := alt.QueenAt[row]
		if col == sol.QueenAt[row] {
			continue
		}
		idx := row*n + col
		from := p.Region[idx]
		safe := regionStaysConnectedWithout(p.Region, n, from, idx)
		seen := map[int]bool{}
		for _, nb := range engine.Neighbors4(engine.CellAt(idx, n), n, n) {
			rg := p.Region[engine.Index(nb, n)]
			if rg == from || seen[rg] {
				continue
			}
			seen[rg] = true
			cands = append(cands, cand{idx: idx, newReg: rg, safe: safe})
		}
	}
	if len(cands) == 0 {
		return false
	}
	r.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })
	// Prefer a move that needs no repair.
	for _, c := range cands {
		if c.safe {
			p.Region[c.idx] = c.newReg
			return true
		}
	}
	// All donors are cut vertices: apply the first and repair the fragment.
	c := cands[0]
	from := p.Region[c.idx]
	p.Region[c.idx] = c.newReg
	repairRegion(p, sol, from)
	return true
}

// repairRegion re-connects region reg after a cell was removed from it: it
// keeps the component holding reg's sol-queen and flood-reassigns every other
// (orphan) cell to an adjacent region. sol stays valid because orphan cells are
// never sol queens (reg's only sol queen is in the kept component).
func repairRegion(p *Puzzle, sol Solution, reg int) {
	n := p.N
	// Locate reg's sol-queen cell.
	anchor := -1
	for row := 0; row < n; row++ {
		idx := row*n + sol.QueenAt[row]
		if p.Region[idx] == reg {
			anchor = idx
			break
		}
	}
	if anchor == -1 {
		return
	}
	// BFS the kept component from the anchor.
	kept := map[int]bool{anchor: true}
	queue := []int{anchor}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range engine.Neighbors4(engine.CellAt(cur, n), n, n) {
			ni := engine.Index(nb, n)
			if p.Region[ni] == reg && !kept[ni] {
				kept[ni] = true
				queue = append(queue, ni)
			}
		}
	}
	// Flood-reassign orphan reg cells (not in kept) to an adjacent region.
	for {
		progress := false
		for i := 0; i < n*n; i++ {
			if p.Region[i] != reg || kept[i] {
				continue
			}
			for _, nb := range engine.Neighbors4(engine.CellAt(i, n), n, n) {
				rg := p.Region[engine.Index(nb, n)]
				if rg != reg {
					p.Region[i] = rg
					progress = true
					break
				}
			}
		}
		if !progress {
			break
		}
	}
}

// findAlternates returns up to limit distinct solutions of p that differ from
// sol (col-ascending order). Cheap: the search stops after limit are found.
func findAlternates(p Puzzle, sol Solution, limit int) []Solution {
	n := p.N
	forced := forcedCols(p)
	colUsed := make([]bool, n)
	regionUsed := make([]bool, n)
	placed := make([]int, n)
	var found []Solution

	var rec func(row int)
	rec = func(row int) {
		if len(found) >= limit {
			return
		}
		if row == n {
			for i := 0; i < n; i++ {
				if placed[i] != sol.QueenAt[i] {
					found = append(found, Solution{N: n, QueenAt: append([]int(nil), placed...)})
					return
				}
			}
			return
		}
		for col := 0; col < n; col++ {
			if forced[row] >= 0 && col != forced[row] {
				continue
			}
			if colUsed[col] {
				continue
			}
			reg := p.Region[row*n+col]
			if regionUsed[reg] {
				continue
			}
			if row > 0 && absInt(placed[row-1]-col) <= 1 {
				continue
			}
			placed[row] = col
			colUsed[col] = true
			regionUsed[reg] = true
			rec(row + 1)
			colUsed[col] = false
			regionUsed[reg] = false
			if len(found) >= limit {
				return
			}
		}
	}
	rec(0)
	return found
}

type regionMove struct {
	idx    int
	newReg int
}

// regionStaysConnectedWithout reports whether region reg remains 4-connected
// after removing cell index drop from it (and reg keeps at least one cell).
func regionStaysConnectedWithout(region []int, n, reg, drop int) bool {
	var cells []int
	for i := 0; i < n*n; i++ {
		if i != drop && region[i] == reg {
			cells = append(cells, i)
		}
	}
	if len(cells) == 0 {
		return false
	}
	want := make(map[int]bool, len(cells))
	for _, c := range cells {
		want[c] = true
	}
	seen := map[int]bool{cells[0]: true}
	queue := []int{cells[0]}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range engine.Neighbors4(engine.CellAt(cur, n), n, n) {
			ni := engine.Index(nb, n)
			if want[ni] && !seen[ni] {
				seen[ni] = true
				queue = append(queue, ni)
			}
		}
	}
	return len(seen) == len(cells)
}
