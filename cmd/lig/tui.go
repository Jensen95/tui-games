package main

import "github.com/Jensen95/tui-games/internal/tui"

// runTUI launches the interactive Bubble Tea app. The shell (menu -> pick
// game+difficulty -> generate -> play -> win) lives in internal/tui; this
// file only wires it into the binary. It runs correctly with zero board
// adapters registered — games without one show as "engine ready — adapter
// coming soon" in the menu and simply can't be entered yet.
func runTUI() error {
	return tui.Run()
}
