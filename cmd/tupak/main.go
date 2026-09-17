// Command tupak is a Bazaar-like Flatpak TUI built on Charm Bubble Tea v2.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/HuntedRaven7/tupak/internal/tui"
)

func main() {
	p := tea.NewProgram(tui.New())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tupak:", err)
		os.Exit(1)
	}
}
