package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors matching GUI dark theme
	colorPrimary   = "#6C63FF"
	colorSuccess   = "#4CAF50"
	colorDanger    = "#FF5252"
	colorWarning   = "#FFC107"
	colorText      = "#E0E0E0"
	colorDimText   = "#888888"
	colorSurface   = "#1E1E2E"
	colorBorder    = "#333344"
	colorHighlight = "#2A2A3C"

	// Base styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorPrimary)).
			MarginBottom(1)

	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorPrimary)).
			Padding(0, 2).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(lipgloss.Color(colorPrimary))

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorDimText)).
				Padding(0, 2)

	statusDotRunning = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorSuccess)).
				Bold(true)

	statusDotStopped = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorDanger)).
				Bold(true)

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorDimText)).
			Width(16)

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorDimText)).
			MarginTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorDanger))

	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorPrimary))

	tableRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText))

	tableSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color(colorPrimary))

	barStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPrimary))

	barEmptyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorBorder))

	keyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorPrimary))

	descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorDimText))
)
