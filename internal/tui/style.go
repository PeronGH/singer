package tui

import "charm.land/lipgloss/v2"

var (
	logHeader = lipgloss.NewStyle().Faint(true).Padding(0, 1)
	errStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).Padding(0, 1)
)
