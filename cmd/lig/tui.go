package main

import (
	"fmt"

	"github.com/Jensen95/tui-games/internal/engine"
)

// runTUI launches the interactive Bubble Tea app. The real shell lands in
// Phase 2 (internal/tui); until then this reports status so the binary is
// honest about what works.
func runTUI() error {
	fmt.Println("lig — LinkedIn-style logic puzzles in your terminal")
	fmt.Println()
	if games := engine.All(); len(games) > 0 {
		fmt.Println("engines ready:")
		for _, e := range games {
			fmt.Printf("  %-12s %s\n", e.ID, e.Name)
		}
		fmt.Println()
	}
	fmt.Println("The interactive TUI is under construction (Phase 2).")
	fmt.Println("Meanwhile: `lig generate --game <id>` and `lig verify <file>` work headlessly.")
	return nil
}
