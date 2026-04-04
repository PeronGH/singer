package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/PeronGH/singer/internal/store"
	"github.com/PeronGH/singer/internal/tui"
)

func main() {
	s, err := store.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	m, err := tui.NewApp(s)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
