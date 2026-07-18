package engine

import (
	"reflect"
	"testing"
)

func TestTransform_ApplyRoundTripsThroughInverse(t *testing.T) {
	// Applying a transform four times (rotations) or twice (flips) must return
	// to the identity on every cell of a non-square grid.
	const rows, cols = 3, 5
	inverses := map[Transform]Transform{
		Identity: Identity,
		Rot90:    Rot270,
		Rot180:   Rot180,
		Rot270:   Rot90,
		FlipH:    FlipH,
		FlipV:    FlipV,
		FlipMain: FlipMain,
		FlipAnti: FlipAnti,
	}
	for tr, inv := range inverses {
		tRows, tCols := tr.Dims(rows, cols)
		for r := 0; r < rows; r++ {
			for c := 0; c < cols; c++ {
				cell := Cell{r, c}
				img := tr.Apply(cell, rows, cols)
				if !InBounds(img, tRows, tCols) {
					t.Fatalf("%v.Apply(%v) = %v out of bounds for %dx%d", tr, cell, img, tRows, tCols)
				}
				back := inv.Apply(img, tRows, tCols)
				if back != cell {
					t.Errorf("%v then %v moved %v -> %v -> %v", tr, inv, cell, img, back)
				}
			}
		}
	}
}

func TestTransform_ApplyIsBijective(t *testing.T) {
	const rows, cols = 4, 4
	for _, tr := range AllTransforms {
		seen := map[Cell]bool{}
		for r := 0; r < rows; r++ {
			for c := 0; c < cols; c++ {
				img := tr.Apply(Cell{r, c}, rows, cols)
				if seen[img] {
					t.Fatalf("%v maps two cells onto %v", tr, img)
				}
				seen[img] = true
			}
		}
	}
}

func TestNeighbors(t *testing.T) {
	if got := len(Neighbors4(Cell{0, 0}, 3, 3)); got != 2 {
		t.Errorf("corner Neighbors4 = %d, want 2", got)
	}
	if got := len(Neighbors8(Cell{0, 0}, 3, 3)); got != 3 {
		t.Errorf("corner Neighbors8 = %d, want 3", got)
	}
	if got := len(Neighbors8(Cell{1, 1}, 3, 3)); got != 8 {
		t.Errorf("center Neighbors8 = %d, want 8", got)
	}
}

func TestRelabelFirstAppearance(t *testing.T) {
	got := RelabelFirstAppearance([]int{7, 7, 2, 9, 2})
	want := []int{0, 0, 1, 2, 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RelabelFirstAppearance = %v, want %v", got, want)
	}
}

func TestIndexCellAtRoundTrip(t *testing.T) {
	const cols = 6
	for i := 0; i < 36; i++ {
		if got := Index(CellAt(i, cols), cols); got != i {
			t.Fatalf("Index(CellAt(%d)) = %d", i, got)
		}
	}
}
