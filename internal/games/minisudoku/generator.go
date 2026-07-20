package minisudoku

import (
	"errors"
	"math/rand/v2"

	"github.com/Jensen95/tui-games/internal/engine"
)

// Generator implements engine.Generator[Puzzle, Solution] per the
// solution-first recipe in docs/plan/docs/02-engine-and-generation.md and
// docs/plan/games/mini-sudoku.md "Generation approach":
//
//  1. Build a full valid solution via randomized backtracking.
//  2. Start from a puzzle with ALL cells given (trivially unique).
//  3. Carve: remove givens one at a time (in random order), keeping each
//     removal only if the puzzle stays uniquely solvable (complete solver)
//     AND fully closes under the no-guess ladder (logic solver).
//  4. Stop carving once a difficulty-appropriate clue budget is reached (or
//     no further candidate can be removed without breaking the invariant).
//
// The generation invariant (Solved(solution), CountSolutions==1, LogicSolve
// closes) is re-checked before Generate returns; on failure it retries with
// fresh randomness from r, up to a bounded number of attempts.
//
// Difficulty labeling (docs/plan/docs/02-engine-and-generation.md "Difficulty
// labeling"): the label is an OUTPUT, never a stamped-through copy of the
// request. For the no-guess tiers (Easy/Medium/Hard) Generate confirms the
// carved puzzle's label against LogicSolve's deepest technique (see
// bandForTechnique in logicsolve.go) before returning. On a mismatch it
// retries the carve — a fresh attempt draws a fresh solution grid and a
// fresh carve order from r, so this reuses the same bounded attempt budget
// as the uniqueness/closure retries above rather than a separate counter.
// If no attempt lands exactly in the requested band within the budget,
// Generate stays total: it relaxes to the closest band any attempt actually
// achieved (ties broken by first-encountered, so behavior stays deterministic
// per seed) instead of erroring.
//
// Expert takes the opposite gate. The no-guess ladder tops out at
// pointing-pair (== Hard in bandForTechnique), so a puzzle that closes under
// LogicSolve can never be harder than Hard. To keep Expert genuinely above
// the ladder, Generate ACCEPTS an Expert attempt only when it is unique AND
// LogicSolve does NOT close (the puzzle provably requires search) — the
// engine contract lets Expert demand guessing (see engine.go). Carving to a
// sparser floor with the closure gate relaxed (see carve/targetClueCount)
// makes search-requiring boards the common case, so this gate is usually met
// on the first attempt. If, within the attempt budget, every unique Expert
// carve happened to stay no-guess, Generate stays total by falling back to
// the sparsest such puzzle it saw (deterministic per seed) rather than
// erroring — an honest, still-unique Expert puzzle even if search wasn't
// forced that run.
type Generator struct{}

var _ engine.Generator[Puzzle, Solution] = Generator{}

const maxGenerateAttempts = 50

// candidate records a fully-valid (unique + logic-closed) puzzle produced
// during Generate whose confirmed band didn't match the request, kept as a
// possible relaxation fallback if no attempt ever matches exactly.
type candidate struct {
	puzzle   Puzzle
	solution Solution
	// For no-guess tiers: |achieved band - requested band|, smaller is
	// closer. For the Expert search-required gate: the puzzle's clue count,
	// so the sparsest fallback wins. Either way, smaller is preferred.
	dist int
}

// bandDistance measures how far apart two difficulty bands are on the
// Easy<Medium<Hard ordinal scale, used to pick the "nearest achievable band"
// fallback when Generate can't hit the requested one exactly.
func bandDistance(a, b engine.Difficulty) int {
	d := int(a) - int(b)
	if d < 0 {
		d = -d
	}
	return d
}

func (g Generator) Generate(diff engine.Difficulty, r *rand.Rand) (Puzzle, Solution, error) {
	solver := Solver{}
	validator := Validator{}

	// Only the no-guess tiers get a confirmed-label guarantee; Expert takes
	// the search-required gate below instead.
	confirmBand := diff == engine.Easy || diff == engine.Medium || diff == engine.Hard

	var fallback *candidate

	for attempt := 0; attempt < maxGenerateAttempts; attempt++ {
		solCells := generateSolution(r)
		solBoard := Board{Cells: solCells}
		if !validator.Solved(solBoard) {
			continue // defensive; a correct backtracking fill never hits this
		}

		seed := seedFromRand(r)
		full := fullCluePuzzle(solCells, diff, seed)
		puzzle := carve(full, r, targetClueCount(diff))

		if solver.CountSolutions(puzzle, 2) != 1 {
			continue
		}
		_, closed, deepest := solver.LogicSolve(puzzle)

		if diff == engine.Expert {
			// Expert must genuinely need search: accept only if the no-guess
			// ladder does NOT close (see the type doc comment). A closed
			// puzzle is still unique and valid, so keep the sparsest one seen
			// as a total-function fallback and retry for a search-requiring
			// carve.
			if !closed {
				return puzzle, Solution{Cells: solCells}, nil
			}
			if fallback == nil || len(puzzle.Givens) < len(fallback.puzzle.Givens) {
				fallback = &candidate{puzzle: puzzle, solution: Solution{Cells: solCells}, dist: len(puzzle.Givens)}
			}
			continue
		}

		if !closed {
			continue
		}

		if !confirmBand {
			return puzzle, Solution{Cells: solCells}, nil
		}

		achieved := bandForTechnique(deepest)
		if achieved == diff {
			return puzzle, Solution{Cells: solCells}, nil
		}

		// Valid puzzle, but its confirmed band misses the request: keep it as
		// a fallback candidate (closest band wins) and retry the carve.
		dist := bandDistance(achieved, diff)
		if fallback == nil || dist < fallback.dist {
			relabeled := puzzle
			relabeled.Diff = achieved // honest label: what LogicSolve confirmed
			fallback = &candidate{puzzle: relabeled, solution: Solution{Cells: solCells}, dist: dist}
		}
	}

	if fallback != nil {
		return fallback.puzzle, fallback.solution, nil
	}
	return Puzzle{}, Solution{}, errors.New("minisudoku: exhausted generation attempts")
}

// seedFromRand pulls a stable per-puzzle seed value off r so the recorded
// Puzzle.Seed reflects this generation. It consumes one draw; kept small so
// determinism (same source state => same value) holds.
func seedFromRand(r *rand.Rand) int64 {
	return int64(r.Uint64() >> 1)
}

// generateSolution builds a full, valid N×N solution via randomized
// backtracking: fill row-major, trying digits 1..N in random order per
// cell, respecting row/column/box constraints. Trivial performance at 6×6
// per docs/plan/games/mini-sudoku.md "Generation approach".
func generateSolution(r *rand.Rand) []int {
	cells := make([]int, N*N)
	rowMask := make([]uint16, N)
	colMask := make([]uint16, N)
	boxMask := make([]uint16, N)

	var fill func(pos int) bool
	fill = func(pos int) bool {
		if pos == N*N {
			return true
		}
		row, col := pos/N, pos%N
		box := boxID(row, col, BoxH, BoxW, N)
		used := rowMask[row] | colMask[col] | boxMask[box]

		digits := [N]int{1, 2, 3, 4, 5, 6}
		r.Shuffle(N, func(i, j int) { digits[i], digits[j] = digits[j], digits[i] })

		for _, d := range digits {
			bit := uint16(1) << uint(d-1)
			if used&bit != 0 {
				continue
			}
			cells[pos] = d
			rowMask[row] |= bit
			colMask[col] |= bit
			boxMask[box] |= bit
			if fill(pos + 1) {
				return true
			}
			cells[pos] = 0
			rowMask[row] &^= bit
			colMask[col] &^= bit
			boxMask[box] &^= bit
		}
		return false
	}
	if !fill(0) {
		panic("minisudoku: failed to construct a valid full solution (unexpected)")
	}
	return cells
}

// fullCluePuzzle builds the (trivially unique) starting point for carving:
// every cell given.
func fullCluePuzzle(cells []int, diff engine.Difficulty, seed int64) Puzzle {
	givens := make(map[int]int, N*N)
	for i, v := range cells {
		givens[i] = v
	}
	return Puzzle{N: N, BoxH: BoxH, BoxW: BoxW, Givens: givens, Diff: diff, SeedVal: seed}
}

// targetClueCount picks how many givens carving should aim to leave, biasing
// harder difficulties toward fewer clues (and so, in practice, a deeper
// technique ladder) per docs/plan/games/mini-sudoku.md "Difficulty
// targeting". Carving still never accepts a removal that breaks uniqueness
// or no-guess closure, so this is a target (the bias Generate's carve
// confirms/relaxes against), not a guarantee.
//
// These counts were tuned empirically against bandForTechnique (see
// logicsolve.go) rather than picked arbitrarily: on this package's 6×6
// carver, 20/16/12 (the original values) left "Easy" landing on
// hidden-single (Medium band) ~93% of the time. 27/16/9 below shift the
// per-attempt hit rate toward the requested band (roughly 65% naked-single
// at 27 clues, ~95%+ hidden-single at 16, ~60% pointing-pair at 9) while
// Generate's bounded retry-then-relax (generator.go) covers the remainder
// honestly instead of mislabeling.
//
// Expert targets 6 — strictly below Hard's 9 — a floor only reachable now
// that Expert carving no longer requires no-guess closure (see carve). With
// the ladder unable to exceed Hard, the point of Expert is a sparser board
// that forces search; a lower floor than Hard is what makes Expert
// consistently land there rather than degenerating into a sparse Hard clone.
// Uniqueness still bounds how far carving actually gets, so 6 is the aim, not
// a promise, and Generate's acceptance gate confirms the result needs search.
func targetClueCount(diff engine.Difficulty) int {
	switch diff {
	case engine.Easy:
		return 27
	case engine.Medium:
		return 16
	case engine.Hard:
		return 9
	default: // Expert (and any future tier): carve to a strictly lower floor.
		return 6
	}
}

// clonePuzzle deep-copies a puzzle's Givens map so carve can try a removal
// and cheaply roll it back if it breaks the invariant.
func clonePuzzle(p Puzzle) Puzzle {
	givens := make(map[int]int, len(p.Givens))
	for k, v := range p.Givens {
		givens[k] = v
	}
	return Puzzle{N: p.N, BoxH: p.BoxH, BoxW: p.BoxW, Givens: givens, Diff: p.Diff, SeedVal: p.SeedVal}
}

// carve removes givens from base one at a time, in an order shuffled by r,
// keeping each removal only if the resulting puzzle is still uniquely
// solvable (the complete solver, ground truth) AND — for the no-guess tiers
// (Easy/Medium/Hard) — fully closes under the no-guess ladder (the logic
// solver), the cross-validation invariant from
// docs/plan/docs/02-engine-and-generation.md. Stops once the clue budget
// (targetClues) is reached or every candidate has been tried once.
//
// Expert (base.Diff == engine.Expert) deliberately DROPS the logic-closure
// requirement and keeps only uniqueness: the deepest implemented technique is
// pointing-pair (== Hard's ceiling in bandForTechnique), so a carve that must
// stay no-guess can never exceed Hard. Per engine.go, Expert only guarantees
// a unique solution — it is permitted (and, here, intended) to require search
// beyond the ladder. Relaxing the gate lets Expert remove clues Hard could
// not, reaching sparser boards that genuinely need guessing.
func carve(base Puzzle, r *rand.Rand, targetClues int) Puzzle {
	// The no-guess tiers cross-validate every removal against LogicSolve;
	// Expert keeps only the uniqueness invariant (see doc comment above).
	requireClosure := base.Diff != engine.Expert

	idxOrder := make([]int, 0, len(base.Givens))
	for idx := range base.Givens {
		idxOrder = append(idxOrder, idx)
	}
	// Sort first so the shuffle below is reproducible independent of the
	// map's (randomized-per-process) iteration order.
	for i := 1; i < len(idxOrder); i++ {
		for j := i; j > 0 && idxOrder[j-1] > idxOrder[j]; j-- {
			idxOrder[j-1], idxOrder[j] = idxOrder[j], idxOrder[j-1]
		}
	}
	r.Shuffle(len(idxOrder), func(i, j int) { idxOrder[i], idxOrder[j] = idxOrder[j], idxOrder[i] })

	cur := clonePuzzle(base)
	solver := Solver{}

	for _, idx := range idxOrder {
		if len(cur.Givens) <= targetClues {
			break
		}
		trial := clonePuzzle(cur)
		delete(trial.Givens, idx)
		if solver.CountSolutions(trial, 2) != 1 {
			continue
		}
		if requireClosure {
			if _, closed, _ := solver.LogicSolve(trial); !closed {
				continue
			}
		}
		cur = trial
	}
	return cur
}
