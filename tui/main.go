package main

import (
	"fmt"
	"os"

	"github.com/SAVE-Labs/roundtable/tui/internal"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	m := internal.New()

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
