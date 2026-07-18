package patches

import (
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// TestValidator_ValidTiling tests that a correct hand-built tiling is valid.
func TestValidator_ValidTiling(t *testing.T) {
	// 2x2 grid with a simple 2x1 rectangle with clue "2 wide" and a 2x1 rectangle with clue "2 wide"
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 2, Shape: Wide}, // top-left cell (row 0, col 0)
			2: {Number: 2, Shape: Wide}, // bottom-left cell (row 1, col 0)
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	b := &Board{
		P:     p,
		Cells: []int{0, 0, 1, 1}, // Two rectangles covering the grid: [0,0]-[1,0] (area 2) and [0,1]-[1,1] (area 2)
	}

	v := NewValidator(p)
	violations := v.Violations(b)

	if len(violations) > 0 {
		t.Errorf("expected no violations for valid tiling, got %v", violations)
	}
	if !v.Solved(b) {
		t.Error("expected Solved() to return true for valid tiling")
	}
}

// TestValidator_OverlappingRectangles tests exact-cover violation from overlaps.
func TestValidator_OverlappingRectangles(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 2, Shape: Wide},
			2: {Number: 2, Shape: Wide},
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	// Cell 1 (top-right) belongs to both rectangle 0 and rectangle 1 (overlap)
	b := &Board{
		P:     p,
		Cells: []int{0, 0, 0, 1}, // Cell 1 overlaps with cell 0
	}

	v := NewValidator(p)
	violations := v.Violations(b)

	// Must have at least an exact-cover violation
	hasExactCoverViolation := false
	for _, viol := range violations {
		if viol.Rule == "exact-cover" {
			hasExactCoverViolation = true
			break
		}
	}

	if !hasExactCoverViolation {
		t.Errorf("expected exact-cover violation for overlapping rectangles, got %v", violations)
	}
	if v.Solved(b) {
		t.Error("expected Solved() to return false for overlapping rectangles")
	}
}

// TestValidator_Gap tests exact-cover violation from gaps (uncovered cells).
func TestValidator_Gap(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 2, Shape: Wide},
			2: {Number: 1, Shape: Free},
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	// Cell 3 is uncovered (-1)
	b := &Board{
		P:     p,
		Cells: []int{0, 0, 1, -1},
	}

	v := NewValidator(p)
	violations := v.Violations(b)

	// Must have at least an exact-cover violation
	hasExactCoverViolation := false
	for _, viol := range violations {
		if viol.Rule == "exact-cover" {
			hasExactCoverViolation = true
			break
		}
	}

	if !hasExactCoverViolation {
		t.Errorf("expected exact-cover violation for gap, got %v", violations)
	}
	if v.Solved(b) {
		t.Error("expected Solved() to return false for grid with gaps")
	}
}

// TestValidator_MultipleCluesInRectangle tests one-clue violation.
func TestValidator_MultipleCluesInRectangle(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 2, Shape: Free},
			1: {Number: 2, Shape: Free},
			2: {Number: 2, Shape: Wide},
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	// Cells 0 and 1 (both clues) are in rectangle 0
	b := &Board{
		P:     p,
		Cells: []int{0, 0, 1, 1},
	}

	v := NewValidator(p)
	violations := v.Violations(b)

	// Must have one-clue violation
	hasOneClueViolation := false
	for _, viol := range violations {
		if viol.Rule == "one-clue" {
			hasOneClueViolation = true
			break
		}
	}

	if !hasOneClueViolation {
		t.Errorf("expected one-clue violation for multiple clues in rectangle, got %v", violations)
	}
	if v.Solved(b) {
		t.Error("expected Solved() to return false")
	}
}

// TestValidator_NoClueInRectangle tests one-clue violation (zero clues).
func TestValidator_NoClueInRectangle(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 4, Shape: Free},
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	// Two rectangles: rectangle 0 (cells 0,1) holds the sole clue at index 0;
	// rectangle 1 (cells 2,3) has no clue inside it at all.
	//
	// NOTE(green-impl): the original board here (`{0, 0, 0, 0}`) put every
	// cell — including the puzzle's only clue — into a single rectangle,
	// which is a fully valid one-clue rectangle and can never exhibit the
	// "rectangle with no clue" case this test claims to cover (self-
	// contradictory: the comment says "no clue inside" while the code
	// includes the clue). Fixed minimally to actually construct that case,
	// matching the test's own stated intent and RuleOneClue's definition.
	b := &Board{
		P:     p,
		Cells: []int{0, 0, 1, 1},
	}

	v := NewValidator(p)
	violations := v.Violations(b)

	// Must have one-clue violation (no clues in some rectangle)
	hasOneClueViolation := false
	for _, viol := range violations {
		if viol.Rule == "one-clue" {
			hasOneClueViolation = true
			break
		}
	}

	if !hasOneClueViolation {
		t.Errorf("expected one-clue violation for rectangle with no clue, got %v", violations)
	}
	if v.Solved(b) {
		t.Error("expected Solved() to return false")
	}
}

// TestValidator_AreaMismatch tests area violation.
func TestValidator_AreaMismatch(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 3, Shape: Free}, // Claims area 3
			2: {Number: 1, Shape: Free}, // Claims area 1
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	// Rectangle 0 covers cells 0,1 (area 2, but clue says 3)
	b := &Board{
		P:     p,
		Cells: []int{0, 0, 1, 1},
	}

	v := NewValidator(p)
	violations := v.Violations(b)

	// Must have area violation
	hasAreaViolation := false
	for _, viol := range violations {
		if viol.Rule == "area" {
			hasAreaViolation = true
			break
		}
	}

	if !hasAreaViolation {
		t.Errorf("expected area violation, got %v", violations)
	}
	if v.Solved(b) {
		t.Error("expected Solved() to return false")
	}
}

// TestValidator_WideClueAsTall tests shape violation.
func TestValidator_WideClueAsTall(t *testing.T) {
	p := &Puzzle{
		R: 3,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 2, Shape: Wide}, // Must be wide (w > h)
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	// Rectangle covering cells 0 and 2 (tall: height 2, width 1)
	// Cell indices: [0,1; 2,3; 4,5] (3x2 grid)
	b := &Board{
		P:     p,
		Cells: []int{0, 1, 0, 1, -1, -1},
	}

	v := NewValidator(p)
	violations := v.Violations(b)

	// Must have shape violation
	hasShapeViolation := false
	for _, viol := range violations {
		if viol.Rule == "shape" {
			hasShapeViolation = true
			break
		}
	}

	if !hasShapeViolation {
		t.Errorf("expected shape violation for Wide clue realized as tall, got %v", violations)
	}
	if v.Solved(b) {
		t.Error("expected Solved() to return false")
	}
}

// TestValidator_SquareClueAs2x3 tests shape violation (non-square).
func TestValidator_SquareClueAs2x3(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 3,
		Clues: map[int]Clue{
			0: {Number: 6, Shape: Square}, // Must be square (6 cannot be square area)
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	// Rectangle covering all 6 cells (2x3)
	b := &Board{
		P:     p,
		Cells: []int{0, 0, 0, 0, 0, 0},
	}

	v := NewValidator(p)
	violations := v.Violations(b)

	// Must have shape violation
	hasShapeViolation := false
	for _, viol := range violations {
		if viol.Rule == "shape" {
			hasShapeViolation = true
			break
		}
	}

	if !hasShapeViolation {
		t.Errorf("expected shape violation for Square clue as 2x3, got %v", violations)
	}
	if v.Solved(b) {
		t.Error("expected Solved() to return false")
	}
}

// TestValidator_PrimeClueValid tests that a prime clue realized as 1×n is valid.
func TestValidator_PrimeClueValid(t *testing.T) {
	p := &Puzzle{
		R: 1,
		C: 5,
		Clues: map[int]Clue{
			0: {Number: 5, Shape: Free}, // Prime 5
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	// Rectangle as 1x5 (area 5, no shape constraint for Free)
	b := &Board{
		P:     p,
		Cells: []int{0, 0, 0, 0, 0},
	}

	v := NewValidator(p)
	violations := v.Violations(b)

	if len(violations) > 0 {
		t.Errorf("expected no violations for prime clue as 1x5, got %v", violations)
	}
	if !v.Solved(b) {
		t.Error("expected Solved() to return true")
	}
}

// TestValidator_StrictWideInequality tests that a square is NOT wide.
func TestValidator_StrictWideInequality(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 4, Shape: Wide}, // Strict inequality: width > height
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	// 2x2 square (w == h, NOT wide)
	b := &Board{
		P:     p,
		Cells: []int{0, 0, 0, 0},
	}

	v := NewValidator(p)
	violations := v.Violations(b)

	// Must have shape violation because 2x2 is not wide
	hasShapeViolation := false
	for _, viol := range violations {
		if viol.Rule == "shape" {
			hasShapeViolation = true
			break
		}
	}

	if !hasShapeViolation {
		t.Errorf("expected shape violation for square as Wide, got %v", violations)
	}
	if v.Solved(b) {
		t.Error("expected Solved() to return false")
	}
}

// TestValidator_StrictTallInequality tests that a square is NOT tall.
func TestValidator_StrictTallInequality(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 4, Shape: Tall}, // Strict inequality: height > width
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	// 2x2 square (h == w, NOT tall)
	b := &Board{
		P:     p,
		Cells: []int{0, 0, 0, 0},
	}

	v := NewValidator(p)
	violations := v.Violations(b)

	// Must have shape violation because 2x2 is not tall
	hasShapeViolation := false
	for _, viol := range violations {
		if viol.Rule == "shape" {
			hasShapeViolation = true
			break
		}
	}

	if !hasShapeViolation {
		t.Errorf("expected shape violation for square as Tall, got %v", violations)
	}
	if v.Solved(b) {
		t.Error("expected Solved() to return false")
	}
}

// TestValidator_TallClueAsWide tests shape violation (opposite).
func TestValidator_TallClueAsWide(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 3,
		Clues: map[int]Clue{
			0: {Number: 2, Shape: Tall}, // Must be tall (h > w)
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	// Rectangle covering cells 0,1 (wide: width 2, height 1)
	b := &Board{
		P:     p,
		Cells: []int{0, 0, 1, 1, -1, -1},
	}

	v := NewValidator(p)
	violations := v.Violations(b)

	// Must have shape violation
	hasShapeViolation := false
	for _, viol := range violations {
		if viol.Rule == "shape" {
			hasShapeViolation = true
			break
		}
	}

	if !hasShapeViolation {
		t.Errorf("expected shape violation for Tall clue as wide, got %v", violations)
	}
	if v.Solved(b) {
		t.Error("expected Solved() to return false")
	}
}

// TestValidator_PartialBoard tests that a partial board doesn't falsely claim to be solved.
func TestValidator_PartialBoard(t *testing.T) {
	p := &Puzzle{
		R: 2,
		C: 2,
		Clues: map[int]Clue{
			0: {Number: 2, Shape: Free},
			2: {Number: 2, Shape: Free},
		},
		SeedVal: 1,
		Diff:    engine.Easy,
	}

	// Partial board: only cells 0,1 covered
	b := &Board{
		P:     p,
		Cells: []int{0, 0, -1, -1},
	}

	v := NewValidator(p)
	if v.Solved(b) {
		t.Error("expected Solved() to return false for partial board")
	}
}

// TestValidator_FreeClueCanBeAnyShape tests that Free clues accept any shape.
func TestValidator_FreeClueCanBeAnyShape(t *testing.T) {
	tests := []struct {
		name string
		w, h int // rectangle dimensions
	}{
		{"1x4", 1, 4}, // tall
		{"4x1", 4, 1}, // wide
		{"2x2", 2, 2}, // square
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Puzzle{
				R: 4,
				C: 1,
				Clues: map[int]Clue{
					0: {Number: tt.w * tt.h, Shape: Free},
				},
				SeedVal: 1,
				Diff:    engine.Easy,
			}

			b := &Board{
				P:     p,
				Cells: make([]int, 4),
			}
			for i := range b.Cells {
				b.Cells[i] = 0
			}

			v := NewValidator(p)
			violations := v.Violations(b)

			// Free should not cause shape violations
			hasShapeViolation := false
			for _, viol := range violations {
				if viol.Rule == "shape" {
					hasShapeViolation = true
					break
				}
			}

			if hasShapeViolation {
				t.Errorf("expected no shape violation for Free clue with %dx%d rectangle", tt.w, tt.h)
			}
		})
	}
}
