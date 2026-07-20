package tango

import (
	"errors"
	"math/rand/v2"

	"github.com/Jensen95/tui-games/internal/engine"
)

// Generator implements engine.Generator[Puzzle, Board] per the
// solution-first recipe in docs/plan/docs/02-engine-and-generation.md and
// docs/plan/games/tango.md "Generation approach":
//
//  1. Build a full valid solution via randomized backtracking.
//  2. Derive candidate "="/"×" edges from the solution's adjacent pairs, and
//     start from a puzzle with ALL cells given and ALL edges present
//     (trivially unique).
//  3. Carve: remove givens/edges one at a time (in random order), keeping
//     each removal only if the puzzle stays uniquely solvable (complete
//     solver) AND — for the no-guess tiers (Easy/Medium/Hard) — still fully
//     closes under the no-guess ladder (logic solver). Expert deliberately
//     drops the closure requirement (see requireClosed) so carving can push
//     past the no-guess ceiling into puzzles that need genuine deduction.
//  4. Stop carving once a difficulty-appropriate clue budget is reached (or
//     no further candidate can be removed).
//
// The generation invariant is re-checked before Generate returns: always
// Solved(solution) and CountSolutions==1, plus LogicSolve closure for the
// no-guess tiers. On failure it retries with fresh randomness from r, up to a
// bounded number of attempts.
type Generator struct{}

var _ engine.Generator[Puzzle, Board] = Generator{}

const maxGenerateAttempts = 50

func (g Generator) Generate(diff engine.Difficulty, r *rand.Rand) (Puzzle, Board, error) {
	solver := Solver{}
	validator := Validator{}

	for attempt := 0; attempt < maxGenerateAttempts; attempt++ {
		solCells := generateSolution(N, r)
		solBoard := Board{N: N, Cells: solCells}
		if !validator.Solved(solBoard) {
			continue // defensive; a correct backtracking fill never hits this
		}

		seed := seedFromRand(r)
		full := fullCluePuzzle(N, solCells, diff, seed)
		puzzle := carve(full, r, targetClueCount(diff), requireClosed(diff))

		if solver.CountSolutions(puzzle, 2) != 1 {
			continue
		}
		// The no-guess tiers must additionally close under the logic ladder;
		// Expert only owes uniqueness (see requireClosed), so a puzzle needing
		// deduction beyond the ladder is accepted there rather than rejected.
		if requireClosed(diff) {
			if _, closed, _ := solver.LogicSolve(puzzle); !closed {
				continue
			}
		}
		return puzzle, solBoard, nil
	}
	return Puzzle{}, Board{}, errors.New("tango: exhausted generation attempts")
}

// seedFromRand pulls a stable per-puzzle seed value off r so the recorded
// Puzzle.Seed reflects this generation. It consumes one draw; kept small so
// determinism (same source state => same value) holds.
func seedFromRand(r *rand.Rand) int64 {
	return int64(r.Uint64() >> 1)
}

// generateSolution builds a full, valid n×n solution via randomized
// backtracking: fill row-major, trying Sun/Moon in random order per cell,
// respecting balance + no-three-in-a-row (edges don't exist yet at this
// stage). Fast on 6×6 per docs/plan/games/tango.md "Generation approach".
func generateSolution(n int, r *rand.Rand) []Symbol {
	b := Board{N: n, Cells: make([]Symbol, n*n)}
	if !fillSolution(&b, 0, r) {
		panic("tango: failed to construct a valid full solution (unexpected)")
	}
	return b.Cells
}

func fillSolution(b *Board, idx int, r *rand.Rand) bool {
	if idx == len(b.Cells) {
		return true
	}
	order := [2]Symbol{Sun, Moon}
	if r.IntN(2) == 1 {
		order[0], order[1] = order[1], order[0]
	}
	for _, sym := range order {
		b.Cells[idx] = sym
		if !violatesAt(b, idx) {
			if fillSolution(b, idx+1, r) {
				return true
			}
		}
		b.Cells[idx] = Empty
	}
	return false
}

// deriveEdges reads the candidate "="/"×" clues off a full solution: every
// orthogonally adjacent pair is Equal if they match, Cross otherwise.
func deriveEdges(cells []Symbol, n int) (h, v map[[2]int]Relation) {
	h = make(map[[2]int]Relation, n*(n-1))
	v = make(map[[2]int]Relation, n*(n-1))
	for row := 0; row < n; row++ {
		for col := 0; col < n-1; col++ {
			a := row*n + col
			b := a + 1
			rel := Cross
			if cells[a] == cells[b] {
				rel = Equal
			}
			h[[2]int{a, b}] = rel
		}
	}
	for row := 0; row < n-1; row++ {
		for col := 0; col < n; col++ {
			a := row*n + col
			b := a + n
			rel := Cross
			if cells[a] == cells[b] {
				rel = Equal
			}
			v[[2]int{a, b}] = rel
		}
	}
	return h, v
}

// fullCluePuzzle builds the (trivially unique) starting point for carving:
// every cell given, every adjacent pair's edge present.
func fullCluePuzzle(n int, cells []Symbol, diff engine.Difficulty, seed int64) Puzzle {
	givens := make(map[int]Symbol, n*n)
	for i, s := range cells {
		givens[i] = s
	}
	h, v := deriveEdges(cells, n)
	return Puzzle{N: n, Givens: givens, HEdges: h, VEdges: v, Diff: diff, SeedVal: seed}
}

// targetClueCount picks how many clues (givens+edges combined) carving
// should aim to leave, biasing harder difficulties toward fewer clues (and
// so, in practice, a deeper technique ladder) per docs/plan/games/tango.md
// "Difficulty targeting". Carving still never accepts a removal that breaks
// uniqueness (nor, for the no-guess tiers, closure), so this is a target, not
// a guarantee.
//
// Expert targets 0 — i.e. "carve as far as uniqueness alone permits". Because
// Expert also drops the closure requirement (see requireClosed), the binding
// constraint is uniqueness rather than the no-guess ladder, so this floor is
// what lets Expert push meaningfully below the ~10-clue ceiling the ladder
// otherwise pins Hard (and, previously, Expert) to.
func targetClueCount(diff engine.Difficulty) int {
	switch diff {
	case engine.Easy:
		return 24
	case engine.Medium:
		return 16
	case engine.Hard:
		return 10
	default: // Expert (and any future tier): carve as far as possible.
		return 0
	}
}

// requireClosed reports whether the carve/accept steps must keep the puzzle
// solvable by the no-guess deduction ladder. Easy/Medium/Hard are contracted
// to be no-guess (engine.Difficulty doc comment: they are "guaranteed
// logic-solvable without guessing"), so closure is required there. Expert is
// contracted to guarantee only a unique solution, so closure is NOT required:
// relaxing it is precisely the lever that makes Expert harder, because it lets
// carving remove clues whose loss would break the ladder — producing puzzles
// that genuinely require deduction past it, while CountSolutions==1 still
// holds unconditionally.
func requireClosed(diff engine.Difficulty) bool {
	return diff != engine.Expert
}

// clueKind distinguishes the three kinds of removable clues.
type clueKind uint8

const (
	clueGiven clueKind = iota
	clueHEdge
	clueVEdge
)

// clueRef names one removable clue: either a given cell or an edge pair.
type clueRef struct {
	kind clueKind
	idx  int    // valid when kind == clueGiven
	pair [2]int // valid when kind == clueHEdge/clueVEdge
}

// buildClueRefs enumerates every possible clue in a fixed, deterministic
// order (never derived from map iteration, which is randomized per-process)
// so that shuffling it with a seeded r.Shuffle stays reproducible.
func buildClueRefs(n int) []clueRef {
	refs := make([]clueRef, 0, n*n+2*n*(n-1))
	for i := 0; i < n*n; i++ {
		refs = append(refs, clueRef{kind: clueGiven, idx: i})
	}
	for row := 0; row < n; row++ {
		for col := 0; col < n-1; col++ {
			a := row*n + col
			refs = append(refs, clueRef{kind: clueHEdge, pair: [2]int{a, a + 1}})
		}
	}
	for row := 0; row < n-1; row++ {
		for col := 0; col < n; col++ {
			a := row*n + col
			refs = append(refs, clueRef{kind: clueVEdge, pair: [2]int{a, a + n}})
		}
	}
	return refs
}

// clonePuzzle deep-copies a puzzle's maps so carve can try a removal and
// cheaply roll it back if it breaks the invariant.
func clonePuzzle(p Puzzle) Puzzle {
	givens := make(map[int]Symbol, len(p.Givens))
	for k, v := range p.Givens {
		givens[k] = v
	}
	h := make(map[[2]int]Relation, len(p.HEdges))
	for k, v := range p.HEdges {
		h[k] = v
	}
	v := make(map[[2]int]Relation, len(p.VEdges))
	for k, val := range p.VEdges {
		v[k] = val
	}
	return Puzzle{N: p.N, Givens: givens, HEdges: h, VEdges: v, Diff: p.Diff, SeedVal: p.SeedVal}
}

func clueCount(p Puzzle) int {
	return len(p.Givens) + len(p.HEdges) + len(p.VEdges)
}

// carve removes clues from base one at a time, in an order shuffled by r,
// keeping each removal only if the resulting puzzle is still uniquely
// solvable (the complete solver, ground truth). When requireClosed is set
// (the no-guess tiers) a removal is additionally rejected unless the puzzle
// still fully closes under the no-guess ladder (the logic solver) — the
// cross-validation invariant from docs/plan/docs/02-engine-and-generation.md.
// Expert passes requireClosed=false so carving may drop clues past the
// no-guess ceiling, stopping only when uniqueness would break (or the clue
// budget targetClues is reached, or every candidate has been tried once).
func carve(base Puzzle, r *rand.Rand, targetClues int, requireClosed bool) Puzzle {
	refs := buildClueRefs(base.N)
	r.Shuffle(len(refs), func(i, j int) { refs[i], refs[j] = refs[j], refs[i] })

	cur := clonePuzzle(base)
	solver := Solver{}

	for _, ref := range refs {
		if clueCount(cur) <= targetClues {
			break
		}
		trial := clonePuzzle(cur)
		switch ref.kind {
		case clueGiven:
			delete(trial.Givens, ref.idx)
		case clueHEdge:
			delete(trial.HEdges, ref.pair)
		case clueVEdge:
			delete(trial.VEdges, ref.pair)
		}
		if solver.CountSolutions(trial, 2) != 1 {
			continue
		}
		if requireClosed {
			if _, closed, _ := solver.LogicSolve(trial); !closed {
				continue
			}
		}
		cur = trial
	}
	return cur
}
