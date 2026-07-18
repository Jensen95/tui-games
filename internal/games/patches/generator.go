package patches

import (
	"errors"
	"math/rand/v2"

	"github.com/Jensen95/tui-games/internal/engine"
)

// Generator produces fresh Patches puzzles. See
// engine.Generator[*Puzzle, *Solution].
type Generator struct{}

// NewGenerator creates a generator.
func NewGenerator() *Generator {
	return &Generator{}
}

var _ engine.Generator[*Puzzle, *Solution] = (*Generator)(nil)

const (
	// genR, genC fix the grid at the spec's shipped default (5×5). The spec
	// allows parameterizing R×C for clones (4×4–7×7); a fixed default keeps
	// the solver's search space small and the generator well inside the
	// perf budget, and no test requires size variation.
	genR = 5
	genC = 5

	// genAttempts bounds the outer solution-first retry loop: build a random
	// partition, try to carve it into a unique, logic-solvable puzzle; if
	// that fails, discard and try a fresh partition.
	genAttempts = 500

	// uniquenessCap matches the engine invariant: CountSolutions(p, 2) == 1.
	uniquenessCap = 2
)

// freeProbability biases how often a rectangle's shape clue is loosened from
// its tightest label (Square/Wide/Tall) to Free, per docs/plan/games/patches.md
// "Difficulty targeting": more Free clues raise difficulty by widening the
// candidate search each clue admits.
func freeProbability(diff engine.Difficulty) float64 {
	switch diff {
	case engine.Easy:
		return 0.10
	case engine.Medium:
		return 0.35
	case engine.Hard:
		return 0.60
	default: // Expert and any future tier
		return 0.75
	}
}

// seedFromRand pulls a stable per-puzzle seed value off r so the recorded
// Puzzle.Seed reflects this generation, independent of the requested
// difficulty (so the same starting r yields the same SeedVal regardless of
// diff — see determinism_test.go).
func seedFromRand(r *rand.Rand) int64 {
	return int64(r.Uint64() >> 1)
}

// genClue tracks one rectangle's derived clue alongside the ground-truth
// shape its geometry actually satisfies, so tightening (Free -> specific)
// never needs to re-derive anything.
type genClue struct {
	idx       int  // anchor cell index
	rect      Rect // ground-truth rectangle from the solution-first partition
	trueShape Shape
	label     Shape // shape currently published in the puzzle's clue (may be Free)
}

// Generate returns a puzzle guaranteed valid + uniquely solvable at ~diff,
// together with its solution. All randomness comes from r.
//
// Strategy is solution-first per docs 02: randomly partition the grid into
// rectangles, derive one clue per rectangle (number = area, shape = its true
// shape or loosened to Free per freeProbability), then tighten Free clues
// back to their true shape — cheapest ones first via random order — only as
// far as needed to reach a unique solution, and further still if that isn't
// yet logic-solvable. Retries with a fresh partition if either goal can't be
// reached.
func (g *Generator) Generate(diff engine.Difficulty, r *rand.Rand) (*Puzzle, *Solution, error) {
	seedVal := seedFromRand(r)
	fp := freeProbability(diff)

	for attempt := 0; attempt < genAttempts; attempt++ {
		rects := partitionGrid(genR, genC, r)
		gcs := deriveClues(rects, genC, fp, r)

		clues := make(map[int]Clue, len(gcs))
		for _, gc := range gcs {
			clues[gc.idx] = Clue{Number: gc.rect.W * gc.rect.H, Shape: gc.label}
		}
		p := &Puzzle{R: genR, C: genC, Clues: clues, SeedVal: seedVal, Diff: diff}
		solver := NewSolver(p)

		if !achieveUniqueness(p, gcs, solver) {
			continue
		}
		if _, closed, _ := solver.LogicSolve(p); !closed {
			tightenAllFree(p, gcs)
			if _, closed2, _ := solver.LogicSolve(p); !closed2 {
				continue
			}
		}

		sol := &Solution{Rects: append([]Rect(nil), rects...)}
		return p, sol, nil
	}
	return nil, nil, errors.New("patches: exhausted generation attempts")
}

// partitionGrid randomly tiles an R×C grid into axis-aligned rectangles:
// scan cells in row-major order, and whenever a cell is still uncovered,
// grow a random rectangle anchored there (bounded by the grid and by
// already-covered neighbors) and commit it. Always terminates because the
// 1×1 rectangle is always available as a fallback.
func partitionGrid(r, c int, rnd *rand.Rand) []Rect {
	owner := make([]int, r*c)
	for i := range owner {
		owner[i] = -1
	}
	var rects []Rect
	id := 0
	for i := 0; i < r*c; i++ {
		if owner[i] != -1 {
			continue
		}
		row, col := i/c, i%c
		rect := growRandomRect(owner, r, c, row, col, rnd)
		for rr := rect.R0; rr < rect.R0+rect.H; rr++ {
			for cc := rect.C0; cc < rect.C0+rect.W; cc++ {
				owner[rr*c+cc] = id
			}
		}
		rects = append(rects, rect)
		id++
	}
	return rects
}

// growRandomRect picks a random axis-aligned rectangle anchored at (r0,c0)
// (its top-left corner) that stays in bounds and covers only uncovered
// cells. Extents are drawn uniformly (see pickExtent) so the partition
// produces a healthy size mix without a large chance of any single
// rectangle collapsing to (or dominating) the whole remaining grid.
func growRandomRect(owner []int, rows, cols, r0, c0 int, rnd *rand.Rand) Rect {
	maxH := rows - r0
	h := pickExtent(maxH, rnd)
	for {
		maxW := cols - c0
		for dr := 0; dr < h; dr++ {
			row := r0 + dr
			run := 0
			for c := c0; c < cols && owner[row*cols+c] == -1; c++ {
				run++
			}
			maxW = min(maxW, run)
		}
		if maxW >= 1 {
			w := pickExtent(maxW, rnd)
			return Rect{R0: r0, C0: c0, W: w, H: h}
		}
		h-- // h=1 always has maxW>=1 since (r0,c0) itself is uncovered
	}
}

// pickExtent chooses a uniformly random length in [1, max]. Plain uniform
// (rather than biasing toward the full extent) keeps any one rectangle from
// having an outsized chance of swallowing the whole remaining grid, which
// would both violate the spec's "avoid degenerate tilings" guidance and
// collapse puzzle diversity (many seeds converging on the same single-clue
// puzzle, breaking fingerprint distinctness).
func pickExtent(max int, rnd *rand.Rand) int {
	if max <= 1 {
		return 1
	}
	return 1 + rnd.IntN(max)
}

// deriveClues assigns each partition rectangle its clue: number = area,
// shape = its true shape (Square/Wide/Tall) unless loosened to Free with
// probability freeProb, and anchor = a uniformly random cell inside it.
func deriveClues(rects []Rect, cols int, freeProb float64, rnd *rand.Rand) []genClue {
	gcs := make([]genClue, len(rects))
	for i, rect := range rects {
		w, h := rect.W, rect.H
		var trueShape Shape
		switch {
		case w == h:
			trueShape = Square
		case w > h:
			trueShape = Wide
		default:
			trueShape = Tall
		}
		label := trueShape
		if rnd.Float64() < freeProb {
			label = Free
		}
		row := rect.R0 + rnd.IntN(h)
		col := rect.C0 + rnd.IntN(w)
		gcs[i] = genClue{idx: row*cols + col, rect: rect, trueShape: trueShape, label: label}
	}
	return gcs
}

// achieveUniqueness tightens Free-labeled clues (in random order) back to
// their true shape, one at a time, re-checking CountSolutions after each,
// until the puzzle is uniquely solvable or every Free clue has been
// tightened. Tightening only ever narrows a clue's candidate set, so it can
// never turn a solvable puzzle unsolvable, and once unique it stays unique.
func achieveUniqueness(p *Puzzle, gcs []genClue, solver *Solver) bool {
	if solver.CountSolutions(p, uniquenessCap) == 1 {
		return true
	}
	order := make([]int, 0, len(gcs))
	for i, gc := range gcs {
		if gc.label == Free {
			order = append(order, i)
		}
	}
	for _, gi := range order {
		gcs[gi].label = gcs[gi].trueShape
		p.Clues[gcs[gi].idx] = Clue{Number: gcs[gi].rect.W * gcs[gi].rect.H, Shape: gcs[gi].trueShape}
		if solver.CountSolutions(p, uniquenessCap) == 1 {
			return true
		}
	}
	return solver.CountSolutions(p, uniquenessCap) == 1
}

// tightenAllFree converts every remaining Free-labeled clue to its true
// shape. Called as a last-resort fallback when a unique puzzle isn't yet
// logic-solvable: maximally specific labels only ever make the logic
// solver's job easier, never harder.
func tightenAllFree(p *Puzzle, gcs []genClue) {
	for i := range gcs {
		if gcs[i].label == Free {
			gcs[i].label = gcs[i].trueShape
			p.Clues[gcs[i].idx] = Clue{Number: gcs[i].rect.W * gcs[i].rect.H, Shape: gcs[i].trueShape}
		}
	}
}
