package tui

import (
	"fmt"
	"strings"

	"multi_model_router/internal/core"

	tea "github.com/charmbracelet/bubbletea"
)

// statusDataMsg carries refreshed status tab data.
type statusDataMsg struct {
	tab statusTab
}

type statusTab struct {
	core       *core.Core
	running    bool
	port       int
	mode       string
	numModels  int
	requests   int
	tokensIn   int
	tokensOut  int
	avgLatency float64
}

func newStatusTab(c *core.Core) statusTab {
	return statusTab{core: c}
}

func refreshStatus(c *core.Core) tea.Cmd {
	return func() tea.Msg {
		ps := c.GetProxyStatus()
		models := c.GetModels()

		stats := c.GetDashboardStats()
		reqs, _ := stats["total_requests"].(int)
		tIn, _ := stats["total_tokens_in"].(int)
		tOut, _ := stats["total_tokens_out"].(int)
		lat, _ := stats["avg_latency"].(float64)

		return statusDataMsg{
			tab: statusTab{
				core:       c,
				running:    ps.Running,
				port:       ps.Port,
				mode:       ps.Mode,
				numModels:  len(models),
				requests:   reqs,
				tokensIn:   tIn,
				tokensOut:  tOut,
				avgLatency: lat,
			},
		}
	}
}

func (t statusTab) Update(msg tea.Msg) (statusTab, tea.Cmd) {
	return t, nil
}

func (t statusTab) View(width int) string {
	var b strings.Builder

	// Proxy status
	statusText := "Stopped"
	statusDot := statusDotStopped.Render("●")
	if t.running {
		statusText = fmt.Sprintf("Running on :%d", t.port)
		statusDot = statusDotRunning.Render("●")
	}

	b.WriteString(fmt.Sprintf("%s %s\n\n", statusDot, statusText))

	// Key metrics
	modeStr := t.mode
	if modeStr == "" {
		modeStr = "auto"
	}
	rows := []struct {
		label string
		value string
	}{
		{"Mode", modeStr},
		{"Models", fmt.Sprintf("%d", t.numModels)},
		{"Today Requests", fmt.Sprintf("%d", t.requests)},
		{"Tokens In", fmt.Sprintf("%d", t.tokensIn)},
		{"Tokens Out", fmt.Sprintf("%d", t.tokensOut)},
		{"Avg Latency", fmt.Sprintf("%.1f ms", t.avgLatency)},
	}

	for _, r := range rows {
		b.WriteString(fmt.Sprintf("  %s %s\n",
			labelStyle.Render(r.label+":"),
			valueStyle.Render(r.value),
		))
	}

	// Action hint
	b.WriteString(fmt.Sprintf("\n  %s start/stop proxy  %s cycle mode  %s reload models",
		keyStyle.Render("[s]"),
		keyStyle.Render("[m]"),
		keyStyle.Render("[r]"),
	))

	return b.String()
}
