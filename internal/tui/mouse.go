package tui

import "github.com/Jensen95/tui-games/internal/engine"

// Geometry is the layout metadata a BoardAdapter reports so the shell can
// translate a raw terminal coordinate into a grid cell without either side
// knowing about the other's internals (03-tui-design.md, "the board-adapter
// pattern"). All units are terminal cells (columns for X/width, rows for
// Y/height), relative to the whole terminal viewport.
type Geometry struct {
	// OriginX, OriginY are the screen coordinates of the top-left corner of
	// cell (0,0) — i.e. before any gutter.
	OriginX, OriginY int
	// CellWidth, CellHeight are the size of one cell's clickable body, not
	// counting gutters.
	CellWidth, CellHeight int
	// Rows, Cols are the grid dimensions.
	Rows, Cols int
	// ColGutter, RowGutter are the extra columns/rows of dead space between
	// cells (e.g. for Tango's inter-cell =/x markers or Zip's walls). Zero
	// means cells are drawn edge-to-edge.
	ColGutter, RowGutter int
}

// strideX/strideY are the pitch (cell body + gutter) used to step from one
// cell to the next.
func (g Geometry) strideX() int { return g.CellWidth + g.ColGutter }
func (g Geometry) strideY() int { return g.CellHeight + g.RowGutter }

// CellFromPoint maps a screen coordinate (x, y) to the grid cell it falls
// inside, per the hit-testing rule in 03-tui-design.md:
//
//	col = (mouseX - originX) / (cellWidth + colGutter)
//	row = (mouseY - originY) / (cellHeight + rowGutter)
//	if in bounds and not in a gutter -> CellRef{row,col} else Outside
//
// It is pure and terminal-independent so it's fully unit-testable (see
// mouse_test.go). The second return value is false whenever the point falls
// outside the grid entirely, in a gutter strip, or the geometry is
// degenerate (non-positive cell size or grid dimensions).
func CellFromPoint(geo Geometry, x, y int) (engine.Cell, bool) {
	if geo.CellWidth <= 0 || geo.CellHeight <= 0 || geo.Rows <= 0 || geo.Cols <= 0 {
		return engine.Cell{}, false
	}

	dx := x - geo.OriginX
	dy := y - geo.OriginY
	if dx < 0 || dy < 0 {
		return engine.Cell{}, false
	}

	strideX, strideY := geo.strideX(), geo.strideY()

	col := dx / strideX
	row := dy / strideY
	if col >= geo.Cols || row >= geo.Rows {
		return engine.Cell{}, false
	}

	// Inside the stride but past the cell body means the point landed in a
	// gutter, which is not part of any cell.
	if dx%strideX >= geo.CellWidth || dy%strideY >= geo.CellHeight {
		return engine.Cell{}, false
	}

	return engine.Cell{Row: row, Col: col}, true
}

// CellRef is a mouse-resolved grid cell handed to a BoardAdapter.HandleMouse.
// Valid is false when the mouse coordinate did not resolve to any cell
// (outside the grid, or inside a gutter); Cell is the zero value in that
// case, mirroring the "-1,-1 if outside" sentinel from 03-tui-design.md
// without relying on a magic number.
type CellRef struct {
	engine.Cell
	Valid bool
}

// CellRefFromPoint is CellFromPoint wrapped as a CellRef, the shape
// BoardAdapter.HandleMouse consumes.
func CellRefFromPoint(geo Geometry, x, y int) CellRef {
	c, ok := CellFromPoint(geo, x, y)
	return CellRef{Cell: c, Valid: ok}
}
