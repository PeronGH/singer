package tui

import "charm.land/lipgloss/v2"

var (
	tabBar = lipgloss.NewStyle().
		Padding(0, 1).
		Background(lipgloss.Color("236"))
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("31"))
	inactiveTabStyle = lipgloss.NewStyle().
				Faint(true).
				Padding(0, 1)

	helpBar = lipgloss.NewStyle().
		Faint(true).
		Padding(0, 1).
		Background(lipgloss.Color("236"))

	helpScreen  = lipgloss.NewStyle().Padding(1, 2)
	helpTitle   = lipgloss.NewStyle().Bold(true)
	helpSection = lipgloss.NewStyle().
			Bold(true).
			Faint(true)

	errStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).Padding(0, 1)
	logStart = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	logStop  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	logEmpty = lipgloss.NewStyle().Faint(true).Padding(1, 2)
)
