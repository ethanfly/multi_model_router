package stats

import (
	"fmt"
	"time"

	"multi_model_router/internal/db"

	"github.com/google/uuid"
)

// Collector records and queries usage statistics via the database.
type Collector struct {
	db *db.DB
}

// NewCollector creates a new stats Collector backed by the given database.
func NewCollector(database *db.DB) *Collector {
	return &Collector{db: database}
}

// RequestLog represents a single logged request.
type RequestLog struct {
	ID         string
	ModelID    string
	Source     string
	Complexity string
	RouteMode  string
	Status     string
	TokensIn   int
	TokensOut  int
	LatencyMs  int64
	ErrorMsg   string
	CreatedAt  time.Time
}

// LogRequest inserts a new request log entry into the database.
func (c *Collector) LogRequest(log *RequestLog) error {
	if log.ID == "" {
		log.ID = uuid.New().String()
	}
	_, err := c.db.Exec(
		`INSERT INTO request_logs (id, model_id, source, complexity, route_mode, status, tokens_in, tokens_out, latency_ms, error_msg, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.ID, log.ModelID, log.Source, log.Complexity, log.RouteMode, log.Status,
		log.TokensIn, log.TokensOut, log.LatencyMs, log.ErrorMsg, log.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert request log: %w", err)
	}
	return nil
}

// DailyStats holds aggregated statistics for a single day.
type DailyStats struct {
	TotalRequests int64
	TotalTokensIn int64
	TotalTokensOut int64
	AvgLatencyMs  float64
}

// GetDailyStats returns aggregated statistics for the given date.
func (c *Collector) GetDailyStats(date time.Time) (*DailyStats, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.Add(24 * time.Hour)

	var s DailyStats
	err := c.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(tokens_in), 0), COALESCE(SUM(tokens_out), 0), COALESCE(AVG(latency_ms), 0)
		 FROM request_logs
		 WHERE created_at >= ? AND created_at < ?`,
		start, end,
	).Scan(&s.TotalRequests, &s.TotalTokensIn, &s.TotalTokensOut, &s.AvgLatencyMs)
	if err != nil {
		return nil, fmt.Errorf("query daily stats: %w", err)
	}
	return &s, nil
}

// ModelUsage holds usage counts and percentages for a model.
type ModelUsage struct {
	ModelID    string  `json:"modelId"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// GetModelUsage returns per-model usage counts and percentages for the given date.
func (c *Collector) GetModelUsage(date time.Time) ([]ModelUsage, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.Add(24 * time.Hour)

	rows, err := c.db.Query(
		`SELECT model_id, COUNT(*) as count
		 FROM request_logs
		 WHERE created_at >= ? AND created_at < ?
		 GROUP BY model_id
		 ORDER BY count DESC`,
		start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("query model usage: %w", err)
	}
	defer rows.Close()

	var results []ModelUsage
	var total int64
	for rows.Next() {
		var mu ModelUsage
		if err := rows.Scan(&mu.ModelID, &mu.Count); err != nil {
			return nil, fmt.Errorf("scan model usage: %w", err)
		}
		total += mu.Count
		results = append(results, mu)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate model usage: %w", err)
	}

	// Calculate percentages.
	for i := range results {
		if total > 0 {
			results[i].Percentage = float64(results[i].Count) / float64(total) * 100
		}
	}

	return results, nil
}

// GetComplexityDistribution returns counts grouped by complexity level for the given date.
// It always returns keys "simple", "medium", "complex" with zero as default.
func (c *Collector) GetComplexityDistribution(date time.Time) (map[string]int64, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.Add(24 * time.Hour)

	dist := map[string]int64{
		"simple":  0,
		"medium":  0,
		"complex": 0,
	}

	rows, err := c.db.Query(
		`SELECT complexity, COUNT(*) as count
		 FROM request_logs
		 WHERE created_at >= ? AND created_at < ?
		 GROUP BY complexity`,
		start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("query complexity distribution: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var complexity string
		var count int64
		if err := rows.Scan(&complexity, &count); err != nil {
			return nil, fmt.Errorf("scan complexity: %w", err)
		}
		// Normalize: map unknown values to "medium".
		key := complexity
		if _, ok := dist[key]; !ok {
			key = "medium"
		}
		dist[key] += count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate complexity: %w", err)
	}

	return dist, nil
}

// RecentLog is a trimmed view of a request log for display.
type RecentLog struct {
	ID         string    `json:"id"`
	ModelID    string    `json:"modelId"`
	Source     string    `json:"source"`
	Complexity string    `json:"complexity"`
	TokensIn   int       `json:"tokensIn"`
	TokensOut  int       `json:"tokensOut"`
	LatencyMs  int64     `json:"latencyMs"`
	CreatedAt  time.Time `json:"createdAt"`
}

// GetRecentLogs returns the most recent logs ordered by created_at descending.
func (c *Collector) GetRecentLogs(limit int) ([]RecentLog, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := c.db.Query(
		`SELECT id, model_id, source, complexity, tokens_in, tokens_out, latency_ms, created_at
		 FROM request_logs
		 ORDER BY created_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent logs: %w", err)
	}
	defer rows.Close()

	var logs []RecentLog
	for rows.Next() {
		var l RecentLog
		if err := rows.Scan(&l.ID, &l.ModelID, &l.Source, &l.Complexity,
			&l.TokensIn, &l.TokensOut, &l.LatencyMs, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan recent log: %w", err)
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent logs: %w", err)
	}

	return logs, nil
}
