// Package all wires every game into the engine registry. It is the single
// place a new game gets registered; importing it (for side effects) gives a
// binary the full game lineup.
//
// This package may import the game packages, but nothing here may import
// internal/tui (depguard-enforced).
package all

// Each Phase 1 game track adds its registration here once its engine passes
// its exit gate, e.g.:
//
//	import "github.com/Jensen95/tui-games/internal/games/tango"
//
//	func init() { engine.Register(tango.Entry()) }
