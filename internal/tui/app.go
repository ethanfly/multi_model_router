package tui

import (
	"fmt"
	"time"

	"multi_model_router/internal/core"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	tabStatus = 0
	tabModels = 1
	tabStats  = 2
)

// tickMsg is sent every 2 seconds to refresh data.
type tickMsg time.Time

// Model is the top-level Bubble Tea model.
type Model struct {
	core       *core.Core
	port       int
	activeTab  int
	width      int
	height     int
	quitting   bool

	// Sub-models for each tab
	status statusTab
	models modelsTab
	stats  statsTab
}

// New creates a new TUI Model.
func New(c *core.Core, port int) Model {
	return Model{
		core:      c,
		port:      port,
		status:    newStatusTab(c),
		models:    newModelsTab(c),
		stats:     newStatsTab(c),
	}
}

// Init starts the first data refresh and periodic ticks.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshAll(), tickEvery())
}

// Update handles incoming messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case keys.Quit, "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case keys.Tab1:
			m.activeTab = tabStatus
			return m, m.refreshAll()
		case keys.Tab2:
			m.activeTab = tabModels
			return m, m.refreshAll()
		case keys.Tab3:
			m.activeTab = tabStats
			return m, m.refreshAll()
		case keys.Toggle:
			m.toggleProxy()
			return m, m.refreshAll()
		case keys.Reload:
			m.core.ReloadModels()
			return m, m.refreshAll()
		}

	case tickMsg:
		return m, tea.Batch(m.refreshAll(), tickEvery())

	case statusDataMsg:
		m.status = msg.tab
	case modelsDataMsg:
		m.models = msg.tab
	case statsDataMsg:
		m.stats = msg.tab
	}

	// Pass keys to active sub-tab for navigation
	var cmd tea.Cmd
	switch m.activeTab {
	case tabModels:
		m.models, cmd = m.models.Update(msg)
	case tabStats:
		m.stats, cmd = m.stats.Update(msg)
	}
	return m, cmd
}

// View renders the TUI.
func (m Model) View() string {
	if m.quitting {
		return "Shutting down...\n"
	}

	// Header
	header := titleStyle.Render("Multi-Model Router")
	tabs := m.renderTabs()

	// Tab content
	var content string
	switch m.activeTab {
	case tabStatus:
		content = m.status.View(m.width)
	case tabModels:
		content = m.models.View(m.width)
	case tabStats:
		content = m.stats.View(m.width)
	}

	// Help bar
	help := helpStyle.Render(HelpText())

	// Layout
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		tabs,
		"",
		content,
		"",
		help,
	)
}

func (m Model) renderTabs() string {
	tabs := []string{" Status ", " Models ", " Stats "}
	rendered := make([]string, 3)
	for i, t := range tabs {
		if i == m.activeTab {
			rendered[i] = tabActiveStyle.Render(t)
		} else {
			rendered[i] = tabInactiveStyle.Render(t)
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Bottom, rendered...)
}

func (m Model) toggleProxy() {
	s := m.core.GetProxyStatus()
	if s.Running {
		_ = m.core.StopProxy()
	} else {
		port := m.port
		if port <= 0 {
			port = m.core.Config().ProxyPort
		}
		_ = m.core.StartProxy(port)
	}
}

func (m Model) refreshAll() tea.Cmd {
	return tea.Batch(
		refreshStatus(m.core),
		refreshModels(m.core),
		refreshStats(m.core),
	)
}

func tickEvery() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Run starts the TUI program.
func Run(c *core.Core, port int) error {
	p := tea.NewProgram(New(c, port), tea.WithAltScreen())
	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
