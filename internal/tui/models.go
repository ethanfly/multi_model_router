package tui

import (
	"fmt"
	"strings"

	"multi_model_router/internal/core"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// modelsDataMsg carries refreshed models tab data.
type modelsDataMsg struct {
	tab modelsTab
}

type modelsTab struct {
	core   *core.Core
	models []core.ModelJSON
	cursor int
}

func newModelsTab(c *core.Core) modelsTab {
	return modelsTab{core: c}
}

func refreshModels(c *core.Core) tea.Cmd {
	return func() tea.Msg {
		return modelsDataMsg{
			tab: modelsTab{
				core:   c,
				models: c.GetModels(),
			},
		}
	}
}

func (t modelsTab) Update(msg tea.Msg) (modelsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case keys.Up, "k":
			if t.cursor > 0 {
				t.cursor--
			}
		case keys.Down, "j":
			if t.cursor < len(t.models)-1 {
				t.cursor++
			}
		}
	}
	return t, nil
}

func (t modelsTab) View(width int) string {
	var b strings.Builder

	if len(t.models) == 0 {
		b.WriteString(dimText("No models configured. Use the GUI to add models."))
		return b.String()
	}

	// Table header
	b.WriteString(fmt.Sprintf("  %-4s %-20s %-12s %-24s %-8s\n",
		tableHeaderStyle.Render("#"),
		tableHeaderStyle.Render("Name"),
		tableHeaderStyle.Render("Provider"),
		tableHeaderStyle.Render("Model ID"),
		tableHeaderStyle.Render("Active"),
	))

	b.WriteString(strings.Repeat("─", min(72, width)) + "\n")

	// Table rows
	for i, m := range t.models {
		active := "✗"
		if m.IsActive {
			active = "✓"
		}

		name := truncate(m.Name, 20)
		provider := truncate(m.Provider, 12)
		modelID := truncate(m.ModelID, 24)

		row := fmt.Sprintf("  %-4d %-20s %-12s %-24s %-8s",
			i+1, name, provider, modelID, active,
		)

		if i == t.cursor {
			b.WriteString(tableSelectedStyle.Render(row) + "\n")
		} else {
			b.WriteString(tableRowStyle.Render(row) + "\n")
		}
	}

	// Scores for selected model
	if t.cursor < len(t.models) {
		m := t.models[t.cursor]
		b.WriteString(fmt.Sprintf("\n  Scores: Reasoning=%d Coding=%d Creativity=%d Speed=%d CostEff=%d",
			m.Reasoning, m.Coding, m.Creativity, m.Speed, m.CostEfficiency))
	}

	// Navigation hints
	b.WriteString(fmt.Sprintf("\n\n  %s/%s navigate  %s delete  %s test",
		keyStyle.Render("↑"), keyStyle.Render("↓"),
		keyStyle.Render("[d]"), keyStyle.Render("[t]"),
	))

	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func dimText(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorDimText)).Render(s)
}
