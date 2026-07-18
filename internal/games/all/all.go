// Package all wires every game into the engine registry. It is the single
// place a new game gets registered; importing it (for side effects) gives a
// binary the full game lineup.
//
// This package may import the game packages, but nothing here may import
// internal/tui (depguard-enforced).
package all

import (
	"github.com/Jensen95/tui-games/internal/engine"
	"github.com/Jensen95/tui-games/internal/games/minisudoku"
	"github.com/Jensen95/tui-games/internal/games/patches"
	"github.com/Jensen95/tui-games/internal/games/queens"
	"github.com/Jensen95/tui-games/internal/games/tango"
	"github.com/Jensen95/tui-games/internal/games/zip"
)

func init() {
	engine.Register(tango.Entry())
	engine.Register(queens.Entry())
	engine.Register(zip.Entry())
	engine.Register(patches.Entry())
	engine.Register(minisudoku.Entry())
}
