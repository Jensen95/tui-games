package engine

// Cell is a grid coordinate, zero-indexed from the top-left: Row grows down,
// Col grows right.
type Cell struct {
	Row, Col int
}

// Index converts a cell to a row-major slice index for a grid with cols columns.
func Index(c Cell, cols int) int { return c.Row*cols + c.Col }

// CellAt is the inverse of Index.
func CellAt(i, cols int) Cell { return Cell{Row: i / cols, Col: i % cols} }

// InBounds reports whether c lies inside a rows×cols grid.
func InBounds(c Cell, rows, cols int) bool {
	return c.Row >= 0 && c.Row < rows && c.Col >= 0 && c.Col < cols
}

// Dirs4 are the orthogonal neighbor offsets (up, down, left, right).
var Dirs4 = [4]Cell{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}

// Dirs8 are the 8-neighborhood offsets (orthogonal + diagonal).
var Dirs8 = [8]Cell{
	{-1, -1}, {-1, 0}, {-1, 1},
	{0, -1}, {0, 1},
	{1, -1}, {1, 0}, {1, 1},
}

// Neighbors4 returns the in-bounds orthogonal neighbors of c.
func Neighbors4(c Cell, rows, cols int) []Cell {
	out := make([]Cell, 0, 4)
	for _, d := range Dirs4 {
		n := Cell{c.Row + d.Row, c.Col + d.Col}
		if InBounds(n, rows, cols) {
			out = append(out, n)
		}
	}
	return out
}

// Neighbors8 returns the in-bounds 8-neighborhood of c (Queens adjacency).
func Neighbors8(c Cell, rows, cols int) []Cell {
	out := make([]Cell, 0, 8)
	for _, d := range Dirs8 {
		n := Cell{c.Row + d.Row, c.Col + d.Col}
		if InBounds(n, rows, cols) {
			out = append(out, n)
		}
	}
	return out
}

// Transform is one of the 8 dihedral symmetries of a rectangle. For non-square
// grids only the transforms with SwapsDims() == false keep the grid shape;
// canonicalizers on non-square grids must either skip dim-swapping transforms
// or compare across the swapped shape consistently.
type Transform uint8

const (
	Identity Transform = iota // (r, c)
	Rot90                     // 90° clockwise
	Rot180                    // 180°
	Rot270                    // 270° clockwise
	FlipH                     // mirror horizontally (reverse columns)
	FlipV                     // mirror vertically (reverse rows)
	FlipMain                  // transpose (main diagonal)
	FlipAnti                  // anti-transpose (anti-diagonal)
)

// AllTransforms is the full dihedral group of order 8.
var AllTransforms = [8]Transform{Identity, Rot90, Rot180, Rot270, FlipH, FlipV, FlipMain, FlipAnti}

// SwapsDims reports whether the transform maps a rows×cols grid to cols×rows.
func (t Transform) SwapsDims() bool {
	return t == Rot90 || t == Rot270 || t == FlipMain || t == FlipAnti
}

// Dims returns the grid dimensions after applying t to a rows×cols grid.
func (t Transform) Dims(rows, cols int) (int, int) {
	if t.SwapsDims() {
		return cols, rows
	}
	return rows, cols
}

// Apply maps cell c of a rows×cols grid to its image under t. The image lies
// in a grid with dimensions t.Dims(rows, cols).
func (t Transform) Apply(c Cell, rows, cols int) Cell {
	switch t {
	case Identity:
		return c
	case Rot90:
		return Cell{c.Col, rows - 1 - c.Row}
	case Rot180:
		return Cell{rows - 1 - c.Row, cols - 1 - c.Col}
	case Rot270:
		return Cell{cols - 1 - c.Col, c.Row}
	case FlipH:
		return Cell{c.Row, cols - 1 - c.Col}
	case FlipV:
		return Cell{rows - 1 - c.Row, c.Col}
	case FlipMain:
		return Cell{c.Col, c.Row}
	case FlipAnti:
		return Cell{cols - 1 - c.Col, rows - 1 - c.Row}
	default:
		panic("engine: unknown transform")
	}
}

// RelabelFirstAppearance renumbers arbitrary integer labels by order of first
// appearance, e.g. [7,7,2,9,2] → [0,0,1,2,1]. Canonicalizers use it to make
// region/color labels comparison-stable (color-agnostic dedup for Queens and
// Patches).
func RelabelFirstAppearance(labels []int) []int {
	next := 0
	seen := make(map[int]int, 8)
	out := make([]int, len(labels))
	for i, l := range labels {
		m, ok := seen[l]
		if !ok {
			m = next
			seen[l] = m
			next++
		}
		out[i] = m
	}
	return out
}
