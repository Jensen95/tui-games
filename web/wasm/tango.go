//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"

	"github.com/Jensen95/tui-games/internal/engine"
	"github.com/Jensen95/tui-games/internal/games/tango"
)

func init() { registerAdapter(tangoAdapter{}) }

type tangoAdapter struct{}

func (tangoAdapter) id() string   { return string(tango.GameID) }
func (tangoAdapter) name() string { return "Tango" }

// tangoBoardWire is the UI-facing board contract for Tango. See
// web/js/api.md "Tango" for the full documentation of every field.
type tangoBoardWire struct {
	Rows   int      `json:"rows"`
	Cols   int      `json:"cols"`
	Cells  [][]int  `json:"cells"`
	Givens [][]bool `json:"givens"`
	HEdges [][]int  `json:"hEdges"`
	VEdges [][]int  `json:"vEdges"`
}

// tangoSolutionWire is the shape of the "solution" JSON returned by
// generate(): the fully solved grid, same cell encoding as the board.
type tangoSolutionWire struct {
	Cells [][]int `json:"cells"`
}

// tangoBoardIn is the minimal shape read back from the UI on
// violations()/solved() calls: only Cells is consulted. Edge constraints
// and dimensions are sourced from the decoded puzzle, never trusted from
// board JSON, because they are immutable puzzle data — see api.md.
type tangoBoardIn struct {
	Cells [][]int `json:"cells"`
}

func tangoCellsGrid(cells []tango.Symbol, n int) [][]int {
	out := make([][]int, n)
	for r := 0; r < n; r++ {
		row := make([]int, n)
		for c := 0; c < n; c++ {
			row[c] = int(cells[r*n+c])
		}
		out[r] = row
	}
	return out
}

func tangoGivensGrid(givens map[int]tango.Symbol, n int) [][]bool {
	out := make([][]bool, n)
	for r := range out {
		out[r] = make([]bool, n)
	}
	for idx := range givens {
		c := engine.CellAt(idx, n)
		out[c.Row][c.Col] = true
	}
	return out
}

// tangoHEdgesGrid reshapes the puzzle's horizontal-edge map into a rows x
// (cols-1) grid: entry [r][c] is the relation between cells (r,c)-(r,c+1).
func tangoHEdgesGrid(m map[[2]int]tango.Relation, n int) [][]int {
	out := make([][]int, n)
	for r := range out {
		out[r] = make([]int, n-1)
	}
	for pair, rel := range m {
		ca, cb := engine.CellAt(pair[0], n), engine.CellAt(pair[1], n)
		col := ca.Col
		if cb.Col < col {
			col = cb.Col
		}
		out[ca.Row][col] = int(rel)
	}
	return out
}

// tangoVEdgesGrid reshapes the puzzle's vertical-edge map into a (rows-1) x
// cols grid: entry [r][c] is the relation between cells (r,c)-(r+1,c).
func tangoVEdgesGrid(m map[[2]int]tango.Relation, n int) [][]int {
	out := make([][]int, n-1)
	for r := range out {
		out[r] = make([]int, n)
	}
	for pair, rel := range m {
		ca, cb := engine.CellAt(pair[0], n), engine.CellAt(pair[1], n)
		row := ca.Row
		if cb.Row < row {
			row = cb.Row
		}
		out[row][ca.Col] = int(rel)
	}
	return out
}

func (tangoAdapter) generate(diff engine.Difficulty, r *rand.Rand) (gameResult, error) {
	gen := tango.Generator{}
	p, solBoard, err := gen.Generate(diff, r)
	if err != nil {
		return gameResult{}, fmt.Errorf("tango: generate: %w", err)
	}
	encoded, err := tango.Encode(p)
	if err != nil {
		return gameResult{}, fmt.Errorf("tango: encode: %w", err)
	}

	n := p.N
	initCells := make([]tango.Symbol, n*n)
	for idx, sym := range p.Givens {
		initCells[idx] = sym
	}
	board := tangoBoardWire{
		Rows:   n,
		Cols:   n,
		Cells:  tangoCellsGrid(initCells, n),
		Givens: tangoGivensGrid(p.Givens, n),
		HEdges: tangoHEdgesGrid(p.HEdges, n),
		VEdges: tangoVEdgesGrid(p.VEdges, n),
	}
	boardJSON, err := json.Marshal(board)
	if err != nil {
		return gameResult{}, fmt.Errorf("tango: marshal board: %w", err)
	}
	sol := tangoSolutionWire{Cells: tangoCellsGrid(solBoard.Cells, n)}
	solJSON, err := json.Marshal(sol)
	if err != nil {
		return gameResult{}, fmt.Errorf("tango: marshal solution: %w", err)
	}
	return gameResult{Puzzle: json.RawMessage(encoded), Solution: solJSON, Board: boardJSON}, nil
}

func (tangoAdapter) decode(puzzleJSON, boardJSON []byte) (tango.Puzzle, tango.Board, error) {
	p, err := tango.Decode(puzzleJSON)
	if err != nil {
		return tango.Puzzle{}, tango.Board{}, fmt.Errorf("tango: decode puzzle: %w", err)
	}
	var in tangoBoardIn
	if err := json.Unmarshal(boardJSON, &in); err != nil {
		return tango.Puzzle{}, tango.Board{}, fmt.Errorf("tango: decode board: %w", err)
	}
	flat := make([]tango.Symbol, p.N*p.N)
	if len(in.Cells) != p.N {
		return tango.Puzzle{}, tango.Board{}, fmt.Errorf("tango: board has %d rows, want %d", len(in.Cells), p.N)
	}
	for r, row := range in.Cells {
		if len(row) != p.N {
			return tango.Puzzle{}, tango.Board{}, fmt.Errorf("tango: board row %d has %d cols, want %d", r, len(row), p.N)
		}
		for c, v := range row {
			flat[r*p.N+c] = tango.Symbol(v)
		}
	}
	b := tango.Board{N: p.N, Cells: flat, HEdges: p.HEdges, VEdges: p.VEdges}
	return p, b, nil
}

func (a tangoAdapter) violations(puzzleJSON, boardJSON []byte) ([]violationJSON, error) {
	_, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return nil, err
	}
	return violationsToJSON(tango.Validator{}.Violations(b)), nil
}

func (a tangoAdapter) solved(puzzleJSON, boardJSON []byte) (bool, error) {
	_, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return false, err
	}
	return tango.Validator{}.Solved(b), nil
}

func tangoSymbolName(v int) string {
	switch tango.Symbol(v) {
	case tango.Sun:
		return "sun"
	case tango.Moon:
		return "moon"
	default:
		return "empty"
	}
}

// tangoOpposite returns the other symbol (Sun<->Moon); it is only ever called
// on a concrete Sun/Moon value (a solution cell), never on Empty.
func tangoOpposite(s tango.Symbol) tango.Symbol {
	if s == tango.Sun {
		return tango.Moon
	}
	return tango.Sun
}

// tangoEdgeRel looks up the relation on the edge between two horizontally- or
// vertically-adjacent cell indices, in whichever order the puzzle stored it.
func tangoEdgeRel(edges map[[2]int]tango.Relation, a, b int) (tango.Relation, bool) {
	if rel, ok := edges[[2]int{a, b}]; ok {
		return rel, true
	}
	if rel, ok := edges[[2]int{b, a}]; ok {
		return rel, true
	}
	return tango.None, false
}

// tangoHintReason derives, from the CURRENT board state, a truthful one-line
// explanation for why the still-empty cell at idx is forced to val, plus the
// name of the deduction technique that pins it (one of tango's own ladder
// technique constants). It re-checks each single-step rule locally — it does
// not trust the solver — and returns an empty clause (and TechniqueGiven,
// meaning "no single-step reason found") when no local rule forces the cell
// on its own yet. That honest fallback matters because the hint reveals the
// first empty cell in row-major order, which is not necessarily the cell the
// deduction ladder would resolve first; only when a rule genuinely fires do we
// claim it.
//
// The rules mirror internal/games/tango/logicsolve.go exactly:
//   - edge-propagation: a "="/"×" edge to an already-filled neighbor.
//   - gap-sandwich: an X _ X run whose middle (this cell) must be the opposite.
//   - pair-doublet: an adjacent X X pair whose outer flank (this cell) must be
//     the opposite, else three in a row.
//   - line-count: this cell's row or column already holds all N/2 of the
//     other symbol, so every remaining cell (this one included) must be val.
func tangoHintReason(b tango.Board, idx int, val tango.Symbol) (string, engine.Technique) {
	n := b.N
	cell := engine.CellAt(idx, n)
	row, col := cell.Row, cell.Col
	other := tangoOpposite(val)
	valName, otherName := tangoSymbolName(int(val)), tangoSymbolName(int(other))

	// Edge propagation: a filled neighbor across a "="/"×" edge.
	type nb struct {
		other  int
		edges  map[[2]int]tango.Relation
		coordR int
		coordC int
	}
	neighbors := []nb{}
	if col > 0 {
		neighbors = append(neighbors, nb{idx - 1, b.HEdges, row, col - 1})
	}
	if col < n-1 {
		neighbors = append(neighbors, nb{idx + 1, b.HEdges, row, col + 1})
	}
	if row > 0 {
		neighbors = append(neighbors, nb{idx - n, b.VEdges, row - 1, col})
	}
	if row < n-1 {
		neighbors = append(neighbors, nb{idx + n, b.VEdges, row + 1, col})
	}
	for _, e := range neighbors {
		if b.Cells[e.other] == tango.Empty {
			continue
		}
		rel, ok := tangoEdgeRel(e.edges, idx, e.other)
		if !ok || rel == tango.None {
			continue
		}
		forced := b.Cells[e.other]
		if rel == tango.Cross {
			forced = tangoOpposite(forced)
		}
		if forced != val {
			continue
		}
		if rel == tango.Equal {
			return fmt.Sprintf("the = link to r%dc%d (a %s) forces the same symbol", e.coordR+1, e.coordC+1, tangoSymbolName(int(b.Cells[e.other]))), tango.TechniqueEdgePropagation
		}
		return fmt.Sprintf("the × link to r%dc%d (a %s) forces the opposite symbol", e.coordR+1, e.coordC+1, tangoSymbolName(int(b.Cells[e.other]))), tango.TechniqueEdgePropagation
	}

	// Gap-sandwich: X _ X with this cell as the middle (both ends == other).
	if col > 0 && col < n-1 && b.Cells[idx-1] == other && b.Cells[idx+1] == other {
		return fmt.Sprintf("it sits between two %ss, so it must be a %s to avoid three in a row", otherName, valName), tango.TechniqueGapSandwich
	}
	if row > 0 && row < n-1 && b.Cells[idx-n] == other && b.Cells[idx+n] == other {
		return fmt.Sprintf("it sits between two %ss, so it must be a %s to avoid three in a row", otherName, valName), tango.TechniqueGapSandwich
	}

	// Pair-doublet: an X X pair on one side, this cell its outer flank.
	if col >= 2 && b.Cells[idx-1] == other && b.Cells[idx-2] == other {
		return fmt.Sprintf("two %ss already sit to its left; a third in a row is illegal, so it must be a %s", otherName, valName), tango.TechniquePairDoublet
	}
	if col <= n-3 && b.Cells[idx+1] == other && b.Cells[idx+2] == other {
		return fmt.Sprintf("two %ss already sit to its right; a third in a row is illegal, so it must be a %s", otherName, valName), tango.TechniquePairDoublet
	}
	if row >= 2 && b.Cells[idx-n] == other && b.Cells[idx-2*n] == other {
		return fmt.Sprintf("two %ss already sit above it; a third in a row is illegal, so it must be a %s", otherName, valName), tango.TechniquePairDoublet
	}
	if row <= n-3 && b.Cells[idx+n] == other && b.Cells[idx+2*n] == other {
		return fmt.Sprintf("two %ss already sit below it; a third in a row is illegal, so it must be a %s", otherName, valName), tango.TechniquePairDoublet
	}

	// Line-count: the row or column already holds all N/2 of the other symbol.
	half := n / 2
	rowOther := 0
	for c := 0; c < n; c++ {
		if b.Cells[row*n+c] == other {
			rowOther++
		}
	}
	if rowOther == half {
		return fmt.Sprintf("row %d already holds all %d %ss, so every remaining cell is a %s", row+1, half, otherName, valName), tango.TechniqueLineCount
	}
	colOther := 0
	for r := 0; r < n; r++ {
		if b.Cells[r*n+col] == other {
			colOther++
		}
	}
	if colOther == half {
		return fmt.Sprintf("column %d already holds all %d %ss, so every remaining cell is a %s", col+1, half, otherName, valName), tango.TechniqueLineCount
	}

	return "", tango.TechniqueGiven
}

// hint mirrors internal/tui/boards/tango.go's Hint(): the first empty cell
// (in row-major order) is filled with the recorded solution's value there.
// The move itself always follows the recorded solution; on top of that, we
// try to derive — from the current board — the single-step rule that forces
// it, so the message can teach the "why" (see tangoHintReason). When no local
// rule fires on this particular cell we say so honestly rather than inventing
// a reason, and leave technique empty.
func (a tangoAdapter) hint(puzzleJSON, boardJSON, solutionJSON []byte) (hintResultJSON, error) {
	p, b, err := a.decode(puzzleJSON, boardJSON)
	if err != nil {
		return hintResultJSON{}, err
	}
	var sol tangoSolutionWire
	if err := json.Unmarshal(solutionJSON, &sol); err != nil {
		return hintResultJSON{}, fmt.Errorf("tango: decode solution: %w", err)
	}
	solFlat, ferr := flattenIntGrid(sol.Cells, p.N, p.N)
	if ferr != nil {
		return hintResultJSON{}, fmt.Errorf("tango: decode solution: %w", ferr)
	}

	for idx, sym := range b.Cells {
		if sym != tango.Empty {
			continue
		}
		cell := engine.CellAt(idx, p.N)
		val := solFlat[idx]
		base := fmt.Sprintf("hint: r%dc%d = %s", cell.Row+1, cell.Col+1, tangoSymbolName(val))
		clause, tech := tangoHintReason(b, idx, tango.Symbol(val))
		msg := base + " — forced by the puzzle's unique solution"
		technique := ""
		if clause != "" {
			msg = base + " — " + clause
			technique = string(tech)
		}
		return hintResultJSON{
			Message:   msg,
			Technique: technique,
			Cells:     []cellJSON{{Row: cell.Row, Col: cell.Col}},
			Apply:     marshalApply(cellsApply{Cells: []cellWrite{{Row: cell.Row, Col: cell.Col, Value: val}}}),
		}, nil
	}
	return hintResultJSON{Done: true, Message: "board is already full"}, nil
}
