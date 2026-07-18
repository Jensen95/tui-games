package tui

import (
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

// geo3x3 is a 3x3 grid, cells 3 columns wide x 1 row tall, origin at (2,4),
// with a 1-column gutter between columns and a 1-row gutter between rows.
// Cell 0 spans x in [2,5), cell 1 in [6,9), cell 2 in [10,13); the columns
// x=5 and x=9 are gutters. Rows work the same way vertically.
func geo3x3() Geometry {
	return Geometry{
		OriginX: 2, OriginY: 4,
		CellWidth: 3, CellHeight: 1,
		Rows: 3, Cols: 3,
		ColGutter: 1, RowGutter: 1,
	}
}

func TestCellFromPoint(t *testing.T) {
	geo := geo3x3()

	tests := []struct {
		name   string
		geo    Geometry
		x, y   int
		want   engine.Cell
		wantOK bool
	}{
		// --- centers of every cell ---
		{"center (0,0)", geo, 3, 4, engine.Cell{Row: 0, Col: 0}, true},
		{"center (0,1)", geo, 7, 4, engine.Cell{Row: 0, Col: 1}, true},
		{"center (0,2)", geo, 11, 4, engine.Cell{Row: 0, Col: 2}, true},
		{"center (1,0)", geo, 3, 6, engine.Cell{Row: 1, Col: 0}, true},
		{"center (2,2)", geo, 11, 8, engine.Cell{Row: 2, Col: 2}, true},

		// --- cell body borders (first/last pixel still inside the cell) ---
		{"cell 0 left edge", geo, 2, 4, engine.Cell{Row: 0, Col: 0}, true},
		{"cell 0 right edge (last pixel before gutter)", geo, 4, 4, engine.Cell{Row: 0, Col: 0}, true},
		{"cell 1 left edge", geo, 6, 4, engine.Cell{Row: 0, Col: 1}, true},
		{"cell top edge", geo, 3, 4, engine.Cell{Row: 0, Col: 0}, true},
		{"cell bottom row top edge", geo, 3, 8, engine.Cell{Row: 2, Col: 0}, true},

		// --- gutters: strictly between two cells, must miss ---
		{"column gutter after cell 0", geo, 5, 4, engine.Cell{}, false},
		{"column gutter after cell 1", geo, 9, 4, engine.Cell{}, false},
		{"row gutter after row 0", geo, 3, 5, engine.Cell{}, false},
		{"row gutter after row 1", geo, 3, 7, engine.Cell{}, false},
		{"gutter both axes at once", geo, 5, 5, engine.Cell{}, false},

		// --- out of bounds ---
		{"left of origin", geo, 1, 4, engine.Cell{}, false},
		{"above origin", geo, 3, 3, engine.Cell{}, false},
		{"exactly at origin minus one, both axes", geo, 1, 3, engine.Cell{}, false},
		{"past last column", geo, 13, 4, engine.Cell{}, false},
		{"past last row", geo, 3, 9, engine.Cell{}, false},
		{"far outside, negative", geo, -100, -100, engine.Cell{}, false},
		{"far outside, positive", geo, 10_000, 10_000, engine.Cell{}, false},

		// --- degenerate geometry ---
		{"zero cell width", Geometry{Rows: 3, Cols: 3, CellWidth: 0, CellHeight: 1}, 0, 0, engine.Cell{}, false},
		{"zero cell height", Geometry{Rows: 3, Cols: 3, CellWidth: 1, CellHeight: 0}, 0, 0, engine.Cell{}, false},
		{"zero rows", Geometry{Rows: 0, Cols: 3, CellWidth: 1, CellHeight: 1}, 0, 0, engine.Cell{}, false},
		{"zero cols", Geometry{Rows: 3, Cols: 0, CellWidth: 1, CellHeight: 1}, 0, 0, engine.Cell{}, false},
		{"negative cell width", Geometry{Rows: 3, Cols: 3, CellWidth: -1, CellHeight: 1}, 0, 0, engine.Cell{}, false},

		// --- no-gutter grid (stride == cell size, every pixel lands) ---
		{
			"no gutter, edge-to-edge cell 0",
			Geometry{OriginX: 0, OriginY: 0, CellWidth: 2, CellHeight: 2, Rows: 2, Cols: 2},
			0, 0, engine.Cell{Row: 0, Col: 0}, true,
		},
		{
			"no gutter, edge-to-edge cell 1 boundary",
			Geometry{OriginX: 0, OriginY: 0, CellWidth: 2, CellHeight: 2, Rows: 2, Cols: 2},
			2, 0, engine.Cell{Row: 0, Col: 1}, true,
		},
		{
			"no gutter, last cell last pixel",
			Geometry{OriginX: 0, OriginY: 0, CellWidth: 2, CellHeight: 2, Rows: 2, Cols: 2},
			3, 3, engine.Cell{Row: 1, Col: 1}, true,
		},
		{
			"no gutter, one past the grid",
			Geometry{OriginX: 0, OriginY: 0, CellWidth: 2, CellHeight: 2, Rows: 2, Cols: 2},
			4, 0, engine.Cell{}, false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CellFromPoint(tt.geo, tt.x, tt.y)
			if ok != tt.wantOK {
				t.Fatalf("CellFromPoint(%d,%d) ok = %v, want %v", tt.x, tt.y, ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Fatalf("CellFromPoint(%d,%d) = %+v, want %+v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

func TestCellRefFromPoint(t *testing.T) {
	geo := geo3x3()

	if ref := CellRefFromPoint(geo, 3, 4); !ref.Valid || ref.Cell != (engine.Cell{Row: 0, Col: 0}) {
		t.Fatalf("CellRefFromPoint hit = %+v, want valid {0,0}", ref)
	}
	if ref := CellRefFromPoint(geo, 5, 4); ref.Valid {
		t.Fatalf("CellRefFromPoint in gutter = %+v, want invalid", ref)
	}
	if ref := CellRefFromPoint(geo, -1, -1); ref.Valid || ref.Cell != (engine.Cell{}) {
		t.Fatalf("CellRefFromPoint outside = %+v, want invalid zero cell", ref)
	}
}
