package core

import (
	"fmt"
	"time"

	"multi_model_router/internal/stats"
)

// GetDashboardStats returns today's aggregated statistics.
func (c *Core) GetDashboardStats() map[string]any {
	result := map[string]any{
		"total_requests":   0,
		"total_tokens_in":  0,
		"total_tokens_out": 0,
		"avg_latency":      0.0,
		"model_usage":      []stats.ModelUsage{},
		"complexity_dist":  map[string]int64{},
		"recent_logs":      []stats.RecentLog{},
	}

	if c.collector == nil {
		return result
	}

	today := time.Now()

	ds, err := c.collector.GetDailyStats(today)
	if err == nil && ds != nil {
		result["total_requests"] = ds.TotalRequests
		result["total_tokens_in"] = ds.TotalTokensIn
		result["total_tokens_out"] = ds.TotalTokensOut
		result["avg_latency"] = ds.AvgLatencyMs
	}

	mu, err := c.collector.GetModelUsage(today)
	if err == nil {
		result["model_usage"] = mu
	}

	cd, err := c.collector.GetComplexityDistribution(today)
	if err == nil {
		result["complexity_dist"] = cd
	}

	rl, err := c.collector.GetRecentLogs(20)
	if err == nil {
		result["recent_logs"] = rl
	}

	return result
}

// GetConfig returns a config value by key.
func (c *Core) GetConfig(key string) string {
	if c.db == nil {
		return ""
	}
	val, _ := c.db.GetConfig(key)
	return val
}

// SetConfig sets a config key/value pair.
func (c *Core) SetConfig(key, value string) error {
	if c.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return c.db.SetConfig(key, value)
}
