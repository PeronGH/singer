package tui

import "charm.land/lipgloss/v2"

var (
	helpBar = lipgloss.NewStyle().
		Faint(true).
		Padding(0, 1).
		Background(lipgloss.Color("236"))

	helpScreen  = lipgloss.NewStyle().Padding(1, 2)
	helpTitle   = lipgloss.NewStyle().Bold(true)
	helpSection = lipgloss.NewStyle().
			Bold(true).
			Faint(true)

	logHeader = lipgloss.NewStyle().Faint(true).Padding(0, 1)
	errStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).Padding(0, 1)
)
