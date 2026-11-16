package ui

import "charm.land/lipgloss/v2"

// Colours always come with a text or a marker: state is never conveyed by
// colour alone.
var (
	styleRunning   = lipgloss.NewStyle().Foreground(lipgloss.Green)
	stylePartial   = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	styleStopped   = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	styleFailed    = lipgloss.NewStyle().Foreground(lipgloss.Red)
	styleCursor    = lipgloss.NewStyle().Reverse(true)
	styleTagged    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Yellow)
	styleColHeader = lipgloss.NewStyle().Reverse(true)
	styleTabActive = lipgloss.NewStyle().Reverse(true).Bold(true)
	styleKeyLabel  = lipgloss.NewStyle().Reverse(true)
	styleKeyName   = lipgloss.NewStyle().Bold(true)
	styleError     = lipgloss.NewStyle().Foreground(lipgloss.Red).Bold(true)
	styleMeterBar  = lipgloss.NewStyle().Foreground(lipgloss.Green)
	styleMeterText = lipgloss.NewStyle().Bold(true)
	styleBox       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)
