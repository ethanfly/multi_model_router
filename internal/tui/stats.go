package tui

import (
	"fmt"
	"sort"
	"strings"

	"multi_model_router/internal/core"
	"multi_model_router/internal/stats"

	tea "github.com/charmbracelet/bubbletea"
)

// statsDataMsg carries refreshed stats tab data.
type statsDataMsg struct {
	tab statsTab
}

type statsTab struct {
	core           *core.Core
	totalReqs      int
	tokensIn       int
	tokensOut      int
	avgLatency     float64
	modelUsage     []stats.ModelUsage
	complexityDist map[string]float64
	recentLogs     []stats.RecentLog
	cursor         int
}

func newStatsTab(c *core.Core) statsTab {
	return statsTab{core: c}
}

func refreshStats(c *core.Core) tea.Cmd {
	return func() tea.Msg {
		data := c.GetDashboardStats()

		reqs, _ := data["total_requests"].(int)
		tIn, _ := data["total_tokens_in"].(int)
		tOut, _ := data["total_tokens_out"].(int)
		lat, _ := data["avg_latency"].(float64)

		mu, _ := data["model_usage"].([]stats.ModelUsage)
		cd, _ := data["complexity_dist"].(map[string]float64)
		rl, _ := data["recent_logs"].([]stats.RecentLog)

		return statsDataMsg{
			tab: statsTab{
				core:           c,
				totalReqs:      reqs,
				tokensIn:       tIn,
				tokensOut:      tOut,
				avgLatency:     lat,
				modelUsage:     mu,
				complexityDist: cd,
				recentLogs:     rl,
			},
		}
	}
}

func (t statsTab) Update(msg tea.Msg) (statsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case keys.Up, "k":
			if t.cursor > 0 {
				t.cursor--
			}
		case keys.Down, "j":
			maxScroll := max(0, len(t.recentLogs)-5)
			if t.cursor < maxScroll {
				t.cursor++
			}
		}
	}
	return t, nil
}

func (t statsTab) View(width int) string {
	var b strings.Builder

	// Summary line
	b.WriteString(valueStyle.Render("Today's Summary") + "\n")
	b.WriteString(fmt.Sprintf("  %s %d   %s %d   %s %d   %s %.1fms\n",
		labelStyle.Render("Requests:"), t.totalReqs,
		labelStyle.Render("Tokens In:"), t.tokensIn,
		labelStyle.Render("Tokens Out:"), t.tokensOut,
		labelStyle.Render("Avg Latency:"), t.avgLatency,
	))

	// Model usage bar chart
	if len(t.modelUsage) > 0 {
		b.WriteString("\n" + valueStyle.Render("Model Usage") + "\n")

		maxCount := int64(0)
		for _, u := range t.modelUsage {
			if u.Count > maxCount {
				maxCount = u.Count
			}
		}

		barWidth := min(30, width-20)
		for _, u := range t.modelUsage {
			barLen := 0
			if maxCount > 0 {
				barLen = int(float64(u.Count) / float64(maxCount) * float64(barWidth))
			}
			filled := strings.Repeat("█", barLen)
			empty := strings.Repeat("░", barWidth-barLen)
			b.WriteString(fmt.Sprintf("  %-16s %s%s %d\n",
				truncate(u.ModelID, 16),
				barStyle.Render(filled),
				barEmptyStyle.Render(empty),
				u.Count,
			))
		}
	}

	// Complexity distribution
	if len(t.complexityDist) > 0 {
		b.WriteString("\n" + valueStyle.Render("Complexity Distribution") + "\n")
		cks := make([]string, 0, len(t.complexityDist))
		for k := range t.complexityDist {
			cks = append(cks, k)
		}
		sort.Strings(cks)
		for _, k := range cks {
			v := t.complexityDist[k]
			b.WriteString(fmt.Sprintf("  %-12s %.1f%%\n", k+":", v))
		}
	}

	// Recent requests
	if len(t.recentLogs) > 0 {
		b.WriteString("\n" + valueStyle.Render("Recent Requests") + "\n")
		start := t.cursor
		end := min(start+5, len(t.recentLogs))
		for i := start; i < end; i++ {
			log := t.recentLogs[i]
			b.WriteString(fmt.Sprintf("  %-16s %d/%d tok  %dms\n",
				truncate(log.ModelID, 16),
				log.TokensIn,
				log.TokensOut,
				log.LatencyMs,
			))
		}
		b.WriteString(fmt.Sprintf("\n  %s/%s scroll",
			keyStyle.Render("↑"), keyStyle.Render("↓"),
		))
	}

	return b.String()
}
