package minisudoku

import (
	"math/bits"

	"github.com/Jensen95/tui-games/internal/engine"
)

// Technique name constants for the no-guess deduction ladder, ordered
// cheapest to most expensive per docs/plan/games/mini-sudoku.md "Deduction
// ladder". TechniqueGiven is the baseline reported when a puzzle needed no
// deduction at all (e.g. every cell already given).
const (
	TechniqueGiven        engine.Technique = "given"
	TechniqueNakedSingle  engine.Technique = "naked-single"
	TechniqueHiddenSingle engine.Technique = "hidden-single"
	TechniqueNakedPair    engine.Technique = "naked-pair"
	TechniqueHiddenPair   engine.Technique = "hidden-pair"
	TechniquePointingPair engine.Technique = "pointing-pair"
)

// techniqueRank orders techniques cheapest to most expensive so bumpTechnique
// can track the single deepest technique used across a whole solve.
var techniqueRank = map[engine.Technique]int{
	TechniqueGiven:        0,
	TechniqueNakedSingle:  1,
	TechniqueHiddenSingle: 2,
	TechniqueNakedPair:    3,
	TechniqueHiddenPair:   3,
	TechniquePointingPair: 4,
}

func bumpTechnique(cur *engine.Technique, t engine.Technique) {
	if techniqueRank[t] > techniqueRank[*cur] {
		*cur = t
	}
}

// ladder holds the candidate-propagation state used by LogicSolve: the
// current (partial) grid plus, for every still-empty cell, the bitmask of
// digits (bit d-1 => digit d) not yet ruled out by an already-placed peer.
type ladder struct {
	n, boxH, boxW int
	cells         []int
	cand          []uint16
	rows, cols    [][]int // n units each, cell indices
	boxes         [][]int // n units each, cell indices
	units         [][]int // rows ++ cols ++ boxes, for unit-generic techniques
}

// newLadder builds the initial candidate-propagation state for p.
func newLadder(p Puzzle) *ladder {
	n, boxH, boxW := p.N, p.BoxH, p.BoxW
	if n == 0 {
		n, boxH, boxW = N, BoxH, BoxW
	}
	l := &ladder{
		n: n, boxH: boxH, boxW: boxW,
		cells: make([]int, n*n),
		cand:  make([]uint16, n*n),
	}
	for idx, val := range p.Givens {
		l.cells[idx] = val
	}

	l.rows = make([][]int, n)
	l.cols = make([][]int, n)
	l.boxes = make([][]int, n)
	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			idx := row*n + col
			l.rows[row] = append(l.rows[row], idx)
			l.cols[col] = append(l.cols[col], idx)
			box := boxID(row, col, boxH, boxW, n)
			l.boxes[box] = append(l.boxes[box], idx)
		}
	}
	l.units = append(l.units, l.rows...)
	l.units = append(l.units, l.cols...)
	l.units = append(l.units, l.boxes...)

	full := uint16(1)<<uint(n) - 1
	for i := 0; i < n*n; i++ {
		if l.cells[i] != 0 {
			continue
		}
		row, col := i/n, i%n
		box := boxID(row, col, boxH, boxW, n)
		mask := full
		for _, idx := range l.rows[row] {
			if v := l.cells[idx]; v != 0 {
				mask &^= 1 << uint(v-1)
			}
		}
		for _, idx := range l.cols[col] {
			if v := l.cells[idx]; v != 0 {
				mask &^= 1 << uint(v-1)
			}
		}
		for _, idx := range l.boxes[box] {
			if v := l.cells[idx]; v != 0 {
				mask &^= 1 << uint(v-1)
			}
		}
		l.cand[i] = mask
	}
	return l
}

// digitFromMask returns the single digit set in a one-bit mask.
func (l *ladder) digitFromMask(mask uint16) int {
	for d := 1; d <= l.n; d++ {
		if mask&(1<<uint(d-1)) != 0 {
			return d
		}
	}
	return 0
}

// assign places digit d at cell i and propagates the elimination to every
// row/col/box peer's candidate set.
func (l *ladder) assign(i, d int) {
	l.cells[i] = d
	l.cand[i] = 0
	bit := uint16(1) << uint(d-1)
	row, col := i/l.n, i%l.n
	box := boxID(row, col, l.boxH, l.boxW, l.n)
	clear := func(idx int) {
		if l.cells[idx] == 0 {
			l.cand[idx] &^= bit
		}
	}
	for _, idx := range l.rows[row] {
		clear(idx)
	}
	for _, idx := range l.cols[col] {
		clear(idx)
	}
	for _, idx := range l.boxes[box] {
		clear(idx)
	}
}

// applyNakedSingles assigns every empty cell with exactly one candidate.
func applyNakedSingles(l *ladder, deepest *engine.Technique) bool {
	progress := false
	for i := 0; i < l.n*l.n; i++ {
		if l.cells[i] != 0 {
			continue
		}
		if bits.OnesCount16(l.cand[i]) == 1 {
			l.assign(i, l.digitFromMask(l.cand[i]))
			progress = true
			bumpTechnique(deepest, TechniqueNakedSingle)
		}
	}
	return progress
}

// applyHiddenSingles assigns, per unit, any digit whose candidates are
// confined to exactly one cell in that unit.
func applyHiddenSingles(l *ladder, deepest *engine.Technique) bool {
	progress := false
	for _, unit := range l.units {
		for d := 1; d <= l.n; d++ {
			bit := uint16(1) << uint(d-1)
			count, pos := 0, -1
			for _, idx := range unit {
				if l.cells[idx] == 0 && l.cand[idx]&bit != 0 {
					count++
					pos = idx
				}
			}
			if count == 1 && l.cells[pos] == 0 {
				l.assign(pos, d)
				progress = true
				bumpTechnique(deepest, TechniqueHiddenSingle)
			}
		}
	}
	return progress
}

// applyNakedPairs finds, per unit, two cells sharing an identical 2-digit
// candidate set and eliminates those digits from every other cell in the
// unit.
func applyNakedPairs(l *ladder, deepest *engine.Technique) bool {
	progress := false
	for _, unit := range l.units {
		type twoCand struct {
			idx  int
			mask uint16
		}
		var twos []twoCand
		for _, idx := range unit {
			if l.cells[idx] == 0 && bits.OnesCount16(l.cand[idx]) == 2 {
				twos = append(twos, twoCand{idx, l.cand[idx]})
			}
		}
		for i := 0; i < len(twos); i++ {
			for j := i + 1; j < len(twos); j++ {
				if twos[i].mask != twos[j].mask {
					continue
				}
				pairMask := twos[i].mask
				for _, idx := range unit {
					if idx == twos[i].idx || idx == twos[j].idx || l.cells[idx] != 0 {
						continue
					}
					before := l.cand[idx]
					after := before &^ pairMask
					if after != before {
						l.cand[idx] = after
						progress = true
						bumpTechnique(deepest, TechniqueNakedPair)
					}
				}
			}
		}
	}
	return progress
}

// applyHiddenPairs finds, per unit, two digits whose candidates are each
// confined to the same two cells, and restricts those two cells' candidates
// to exactly that pair.
func applyHiddenPairs(l *ladder, deepest *engine.Technique) bool {
	progress := false
	for _, unit := range l.units {
		posOf := make([][]int, l.n+1)
		for _, idx := range unit {
			if l.cells[idx] != 0 {
				continue
			}
			for d := 1; d <= l.n; d++ {
				if l.cand[idx]&(1<<uint(d-1)) != 0 {
					posOf[d] = append(posOf[d], idx)
				}
			}
		}
		for d1 := 1; d1 <= l.n; d1++ {
			if len(posOf[d1]) != 2 {
				continue
			}
			for d2 := d1 + 1; d2 <= l.n; d2++ {
				if len(posOf[d2]) != 2 {
					continue
				}
				if posOf[d1][0] != posOf[d2][0] || posOf[d1][1] != posOf[d2][1] {
					continue
				}
				mask := uint16(1)<<uint(d1-1) | uint16(1)<<uint(d2-1)
				for _, idx := range posOf[d1] {
					before := l.cand[idx]
					after := before & mask
					if after != before {
						l.cand[idx] = after
						progress = true
						bumpTechnique(deepest, TechniqueHiddenPair)
					}
				}
			}
		}
	}
	return progress
}

// containsIdx reports whether unit holds cell idx.
func containsIdx(unit []int, idx int) bool {
	for _, u := range unit {
		if u == idx {
			return true
		}
	}
	return false
}

// applyPointingPairs implements both directions of box-line reduction:
// pointing (a digit confined, within a box, to one row/column eliminates it
// from the rest of that row/column outside the box) and the converse
// box-line reduction (a digit confined, within a row/column, to one box
// eliminates it from the rest of that box outside the row/column).
func applyPointingPairs(l *ladder, deepest *engine.Technique) bool {
	progress := false
	n := l.n

	// Pointing: box -> line.
	for _, box := range l.boxes {
		for d := 1; d <= n; d++ {
			bit := uint16(1) << uint(d-1)
			var cellsWithD []int
			for _, idx := range box {
				if l.cells[idx] == 0 && l.cand[idx]&bit != 0 {
					cellsWithD = append(cellsWithD, idx)
				}
			}
			if len(cellsWithD) == 0 {
				continue
			}
			sameRow, sameCol := true, true
			row0, col0 := cellsWithD[0]/n, cellsWithD[0]%n
			for _, idx := range cellsWithD {
				if idx/n != row0 {
					sameRow = false
				}
				if idx%n != col0 {
					sameCol = false
				}
			}
			if sameRow {
				for _, idx := range l.rows[row0] {
					if l.cells[idx] != 0 || containsIdx(box, idx) {
						continue
					}
					before := l.cand[idx]
					after := before &^ bit
					if after != before {
						l.cand[idx] = after
						progress = true
						bumpTechnique(deepest, TechniquePointingPair)
					}
				}
			}
			if sameCol {
				for _, idx := range l.cols[col0] {
					if l.cells[idx] != 0 || containsIdx(box, idx) {
						continue
					}
					before := l.cand[idx]
					after := before &^ bit
					if after != before {
						l.cand[idx] = after
						progress = true
						bumpTechnique(deepest, TechniquePointingPair)
					}
				}
			}
		}
	}

	// Box-line reduction: row/col -> box.
	lineToBox := func(lines [][]int) {
		for _, line := range lines {
			for d := 1; d <= n; d++ {
				bit := uint16(1) << uint(d-1)
				var cellsWithD []int
				for _, idx := range line {
					if l.cells[idx] == 0 && l.cand[idx]&bit != 0 {
						cellsWithD = append(cellsWithD, idx)
					}
				}
				if len(cellsWithD) == 0 {
					continue
				}
				box0 := boxID(cellsWithD[0]/n, cellsWithD[0]%n, l.boxH, l.boxW, n)
				sameBox := true
				for _, idx := range cellsWithD {
					if boxID(idx/n, idx%n, l.boxH, l.boxW, n) != box0 {
						sameBox = false
						break
					}
				}
				if !sameBox {
					continue
				}
				for _, idx := range l.boxes[box0] {
					if l.cells[idx] != 0 || containsIdx(line, idx) {
						continue
					}
					before := l.cand[idx]
					after := before &^ bit
					if after != before {
						l.cand[idx] = after
						progress = true
						bumpTechnique(deepest, TechniquePointingPair)
					}
				}
			}
		}
	}
	lineToBox(l.rows)
	lineToBox(l.cols)

	return progress
}

// LogicSolve attempts a no-guess solve using the deduction ladder from
// docs/plan/games/mini-sudoku.md "Deduction ladder": naked singles, hidden
// singles, naked/hidden pairs, then pointing pairs / box-line reduction,
// iterated to a fixpoint. It returns the resulting solution (partial if it
// didn't close, in which case unfilled cells are 0), whether the board
// fully closed, and the single deepest technique needed anywhere during the
// solve.
func (s Solver) LogicSolve(p Puzzle) (Solution, bool, engine.Technique) {
	l := newLadder(p)
	deepest := TechniqueGiven

	for {
		progress := false
		if applyNakedSingles(l, &deepest) {
			progress = true
		}
		if applyHiddenSingles(l, &deepest) {
			progress = true
		}
		if applyNakedPairs(l, &deepest) {
			progress = true
		}
		if applyHiddenPairs(l, &deepest) {
			progress = true
		}
		if applyPointingPairs(l, &deepest) {
			progress = true
		}
		if !progress {
			break
		}
	}

	closed := true
	for _, c := range l.cells {
		if c == 0 {
			closed = false
			break
		}
	}
	return Solution{Cells: append([]int(nil), l.cells...)}, closed, deepest
}
