package tui

import (
	"fmt"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/Jensen95/tui-games/internal/engine"
)

// MouseEventType distinguishes the phase of a mouse interaction handed to a
// BoardAdapter, independent of where on the grid it landed (that's CellRef's
// job). This is what lets an adapter implement press -> motion(while held)
// -> release drag state machines (03-tui-design.md: Zip's path drawing,
// Queens' drag-to-mark, Patches' rectangle drag).
type MouseEventType int

const (
	MouseEventPress MouseEventType = iota
	MouseEventMotion
	MouseEventRelease
	MouseEventWheel
)

func (t MouseEventType) String() string {
	switch t {
	case MouseEventPress:
		return "press"
	case MouseEventMotion:
		return "motion"
	case MouseEventRelease:
		return "release"
	case MouseEventWheel:
		return "wheel"
	default:
		return fmt.Sprintf("MouseEventType(%d)", int(t))
	}
}

// MouseEvent is a mouse message with its screen coordinates already stripped
// out — the shell resolves those to a CellRef via GridGeometry before
// calling HandleMouse, so adapters never need to know about screen layout.
type MouseEvent struct {
	Type   MouseEventType
	Button tea.MouseButton
	Mod    tea.KeyMod
}

// BoardAdapter is the seam between the game-agnostic shell and one game's
// board. The shell owns the frame (title, timer, help line, borders,
// screen-state transitions); the adapter owns the grid. Adding a game means
// writing one adapter and calling Register — no shell changes
// (03-tui-design.md, "the board-adapter pattern").
//
// Implementations must keep HandleKey/HandleMouse pure with respect to the
// model (no direct terminal writes) so they stay unit-testable without a
// terminal (04-testing-strategy.md).
type BoardAdapter interface {
	// View renders the current board to a Lip Gloss-styled string, themed
	// and focus-aware. The shell places this inside its frame.
	View(theme Theme) string

	// HandleKey handles a key press not already claimed by the shared
	// keymap (movement, symbol cycling, game-specific keys). It reports
	// whether the board changed, so the shell knows to re-check
	// Violations/Solved.
	HandleKey(k tea.KeyPressMsg) (changed bool)

	// HandleMouse handles a mouse event already resolved to a grid cell (or
	// an invalid CellRef if outside the grid/in a gutter).
	HandleMouse(ev MouseEvent, cell CellRef) (changed bool)

	// Violations delegates to the engine's validator for live feedback; the
	// shell styles the offending cells in the error style.
	Violations() []engine.Violation
	// Solved delegates to the engine's validator; true triggers the shell's
	// win transition.
	Solved() bool

	// GridGeometry reports layout metadata so the shell's mouse code can
	// map screen coordinates to cells without knowing the board's internal
	// rendering.
	GridGeometry() Geometry

	// Hint reveals one forced move (and, conventionally, logs/exposes which
	// technique it used) via the engine's logic solver.
	Hint()
	// Undo reverts the last move.
	Undo()
	// Reset restores the puzzle to its initial (ungenerated-move) state.
	Reset()
}

// AdapterFactory builds a BoardAdapter from a freshly generated puzzle. Each
// game package registers one against its engine.GameID.
type AdapterFactory func(gen engine.Generated) BoardAdapter

var (
	adapterMu sync.RWMutex
	adapters  = map[engine.GameID]AdapterFactory{}
)

// Register adds a board-adapter factory for a game id, mirroring
// internal/engine's registry style. It panics on a duplicate id or an
// incomplete registration — both are wiring bugs caught at init time, not
// runtime conditions.
func Register(id engine.GameID, factory AdapterFactory) {
	if id == "" || factory == nil {
		panic(fmt.Sprintf("tui: incomplete adapter registration for %q", id))
	}
	adapterMu.Lock()
	defer adapterMu.Unlock()
	if _, dup := adapters[id]; dup {
		panic(fmt.Sprintf("tui: duplicate board adapter for game id %q", id))
	}
	adapters[id] = factory
}

// Lookup returns the adapter factory registered for a game id, if any. The
// shell's menu uses the ok return to distinguish "engine ready, adapter
// coming soon" games from playable ones.
func Lookup(id engine.GameID) (AdapterFactory, bool) {
	adapterMu.RLock()
	defer adapterMu.RUnlock()
	f, ok := adapters[id]
	return f, ok
}
