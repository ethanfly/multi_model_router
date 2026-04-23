package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"multi_model_router/internal/config"
	"multi_model_router/internal/crypto"
	"multi_model_router/internal/db"
	"multi_model_router/internal/provider"
	"multi_model_router/internal/proxy"
	"multi_model_router/internal/router"
	"multi_model_router/internal/stats"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the main Wails binding struct.
type App struct {
	ctx        context.Context
	config     *config.Config
	db         *db.DB
	engine     *router.Engine
	classifier *router.Classifier
	collector  *stats.Collector
	proxy      *proxy.Server
}

// NewApp creates a new App instance with default config.
func NewApp() *App {
	return &App{config: config.Default()}
}

// startup is called when the Wails app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	var err error
	a.db, err = db.New(a.config.AppDataDir)
	if err != nil {
		fmt.Printf("db init error: %v\n", err)
		return
	}

	a.collector = stats.NewCollector(a.db)
	a.classifier = router.NewClassifier(nil)
	a.engine = router.NewEngine(a.classifier)
	a.loadModels()

	// Auto-start proxy if enabled
	if val, _ := a.db.GetConfig("proxy_enabled"); val == "true" {
		port := a.config.ProxyPort
		if v, _ := a.db.GetConfig("proxy_port"); v != "" {
			fmt.Sscanf(v, "%d", &port)
		}
		a.startProxy(port)
	}
}

// shutdown is called when the Wails app closes.
func (a *App) shutdown(ctx context.Context) {
	if a.proxy != nil {
		a.proxy.Stop()
	}
	if a.db != nil {
		a.db.Close()
	}
}

// loadModels loads all models from the database into the engine.
func (a *App) loadModels() {
	if a.db == nil {
		return
	}

	rows, err := a.db.Query(
		`SELECT id, name, provider, base_url, api_key, model_id,
		        reasoning, coding, creativity, speed, cost_efficiency,
		        max_rpm, max_tpm, is_active
		 FROM models`,
	)
	if err != nil {
		fmt.Printf("load models error: %v\n", err)
		return
	}
	defer rows.Close()

	var models []*router.ModelConfig
	for rows.Next() {
		var m router.ModelConfig
		var isActive int
		if err := rows.Scan(
			&m.ID, &m.Name, &m.Provider, &m.BaseURL, &m.APIKey, &m.ModelID,
			&m.Reasoning, &m.Coding, &m.Creativity, &m.Speed, &m.CostEfficiency,
			&m.MaxRPM, &m.MaxTPM, &isActive,
		); err != nil {
			fmt.Printf("scan model error: %v\n", err)
			continue
		}
		m.IsActive = isActive == 1

		// Decrypt API key
		if m.APIKey != "" {
			decrypted, err := crypto.Decrypt(m.APIKey)
			if err != nil {
				fmt.Printf("decrypt key error for %s: %v\n", m.ID, err)
				continue
			}
			m.APIKey = decrypted
		}

		a.ensureProvider(m.Provider, m.BaseURL, m.APIKey)
		models = append(models, &m)
	}

	if len(models) > 0 {
		a.engine.SetModels(models)
	}
}

// reloadModels clears and reloads all models from the database.
func (a *App) reloadModels() {
	a.engine.SetModels(nil)
	a.loadModels()
}

// ensureProvider creates and registers a provider if it doesn't already exist.
func (a *App) ensureProvider(providerName, baseURL, apiKey string) {
	// The engine stores providers in a map; we add the provider regardless
	// to ensure the key is populated (the engine uses a sync map internally).
	switch providerName {
	case "openai":
		a.engine.AddProvider(providerName, provider.NewOpenAI(baseURL, apiKey))
	case "anthropic":
		a.engine.AddProvider(providerName, provider.NewAnthropic(baseURL, apiKey))
	}
}

// --- Frontend-bound types ---

// ModelJSON is the frontend-safe representation of a model (no API key).
type ModelJSON struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Provider       string `json:"provider"`
	BaseURL        string `json:"baseUrl"`
	APIKey         string `json:"apiKey"`
	ModelID        string `json:"modelId"`
	Reasoning      int    `json:"reasoning"`
	Coding         int    `json:"coding"`
	Creativity     int    `json:"creativity"`
	Speed          int    `json:"speed"`
	CostEfficiency int    `json:"costEfficiency"`
	MaxRPM         int    `json:"maxRpm"`
	MaxTPM         int    `json:"maxTpm"`
	IsActive       bool   `json:"isActive"`
}

// ChatMessage represents a single message in a chat request from the frontend.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is a chat request from the frontend.
type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
	Mode     string        `json:"mode"`
	ModelID  string        `json:"modelId"`
}

// ChatResponse is sent back to the frontend after routing a chat.
type ChatResponse struct {
	ModelID    string `json:"modelId"`
	ModelName  string `json:"modelName"`
	Provider   string `json:"provider"`
	Complexity string `json:"complexity"`
	RouteMode  string `json:"routeMode"`
	Status     string `json:"status"`
	Error      string `json:"error"`
}

// --- Frontend-bound methods ---

// GetModels returns all models from the database for the frontend.
func (a *App) GetModels() []ModelJSON {
	if a.db == nil {
		return nil
	}

	rows, err := a.db.Query(
		`SELECT id, name, provider, base_url, api_key, model_id,
		        reasoning, coding, creativity, speed, cost_efficiency,
		        max_rpm, max_tpm, is_active
		 FROM models
		 ORDER BY name`,
	)
	if err != nil {
		fmt.Printf("GetModels error: %v\n", err)
		return nil
	}
	defer rows.Close()

	var models []ModelJSON
	for rows.Next() {
		var m ModelJSON
		var isActive int
		if err := rows.Scan(
			&m.ID, &m.Name, &m.Provider, &m.BaseURL, &m.APIKey, &m.ModelID,
			&m.Reasoning, &m.Coding, &m.Creativity, &m.Speed, &m.CostEfficiency,
			&m.MaxRPM, &m.MaxTPM, &isActive,
		); err != nil {
			continue
		}
		m.IsActive = isActive == 1
		// Mask the API key for frontend display
		if len(m.APIKey) > 8 {
			m.APIKey = m.APIKey[:4] + "..." + m.APIKey[len(m.APIKey)-4:]
		}
		models = append(models, m)
	}

	return models
}

// SaveModel inserts or updates a model and reloads the engine.
func (a *App) SaveModel(m ModelJSON) error {
	if a.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Encrypt the API key before storing
	encryptedKey := m.APIKey
	if m.APIKey != "" && !(len(m.APIKey) > 8 && m.APIKey[4:7] == "...") {
		enc, err := crypto.Encrypt(m.APIKey)
		if err != nil {
			return fmt.Errorf("encrypt api key: %w", err)
		}
		encryptedKey = enc
	}

	activeInt := 0
	if m.IsActive {
		activeInt = 1
	}

	_, err := a.db.Exec(
		`INSERT OR REPLACE INTO models
		 (id, name, provider, base_url, api_key, model_id,
		  reasoning, coding, creativity, speed, cost_efficiency,
		  max_rpm, max_tpm, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Name, m.Provider, m.BaseURL, encryptedKey, m.ModelID,
		m.Reasoning, m.Coding, m.Creativity, m.Speed, m.CostEfficiency,
		m.MaxRPM, m.MaxTPM, activeInt,
	)
	if err != nil {
		return fmt.Errorf("save model: %w", err)
	}

	a.reloadModels()
	return nil
}

// DeleteModel removes a model and reloads the engine.
func (a *App) DeleteModel(id string) error {
	if a.db == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := a.db.Exec("DELETE FROM models WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete model: %w", err)
	}

	a.reloadModels()
	return nil
}

// TestModel creates a temporary provider and checks its health.
func (a *App) TestModel(m ModelJSON) string {
	var p provider.Provider
	switch m.Provider {
	case "openai":
		p = provider.NewOpenAI(m.BaseURL, m.APIKey)
	case "anthropic":
		p = provider.NewAnthropic(m.BaseURL, m.APIKey)
	default:
		return "FAIL: unknown provider " + m.Provider
	}

	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()

	if err := p.HealthCheck(ctx); err != nil {
		return "FAIL: " + err.Error()
	}
	return "OK"
}

// SendChat routes a chat request through the engine and streams results back
// to the frontend via Wails events.
func (a *App) SendChat(req ChatRequest) ChatResponse {
	if a.engine == nil {
		return ChatResponse{Status: "error", Error: "engine not initialized"}
	}

	// Convert frontend messages to provider messages
	msgs := make([]provider.Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = provider.Message{Role: m.Role, Content: m.Content}
	}

	mode := router.RouteModeFromString(req.Mode)

	routeReq := &router.RouteRequest{
		Messages: msgs,
		Mode:     mode,
		ModelID:  req.ModelID,
		Source:   "gui",
	}

	result := a.engine.Route(a.ctx, routeReq)
	if result == nil {
		return ChatResponse{Status: "error", Error: "no result from router"}
	}

	resp := ChatResponse{
		ModelName: result.ModelName,
		Provider:  result.Provider,
		Status:    result.Status,
		Error:     result.ErrorMsg,
		RouteMode: router.RouteMode(result.RouteMode).String(),
	}

	if result.Status != "success" {
		return resp
	}

	// Consume the stream in a goroutine, emitting events to the frontend.
	go func() {
		var fullContent string
		var usage *provider.Usage

		for chunk := range result.Stream {
			if chunk.Error != nil {
				wailsRuntime.EventsEmit(a.ctx, "chat:error", chunk.Error.Error())
				break
			}
			fullContent += chunk.Content
			if chunk.Usage != nil {
				usage = chunk.Usage
			}

			wailsRuntime.EventsEmit(a.ctx, "chat:chunk", map[string]string{
				"content": chunk.Content,
				"model":   chunk.Model,
			})

			if chunk.Done {
				break
			}
		}

		wailsRuntime.EventsEmit(a.ctx, "chat:done", map[string]string{
			"content": fullContent,
			"model":   result.ModelName,
		})

		// Log to stats
		if a.collector != nil {
			tokensIn := 0
			tokensOut := 0
			if usage != nil {
				tokensIn = usage.InputTokens
				tokensOut = usage.OutputTokens
			}
			_ = a.collector.LogRequest(&stats.RequestLog{
				ID:         router.NewUUID(),
				ModelID:    result.ModelName,
				Source:     "gui",
				Complexity: router.Complexity(result.Complexity).String(),
				RouteMode:  resp.RouteMode,
				Status:     "success",
				TokensIn:   tokensIn,
				TokensOut:  tokensOut,
				LatencyMs:  result.LatencyMs,
				CreatedAt:  time.Now(),
			})
		}
	}()

	return resp
}

// GetDashboardStats returns today's aggregated statistics for the dashboard.
func (a *App) GetDashboardStats() map[string]any {
	result := map[string]any{
		"total_requests":     0,
		"total_tokens_in":    0,
		"total_tokens_out":   0,
		"avg_latency":        0.0,
		"model_usage":        []stats.ModelUsage{},
		"complexity_dist":    map[string]int64{},
		"recent_logs":        []stats.RecentLog{},
	}

	if a.collector == nil {
		return result
	}

	today := time.Now()

	ds, err := a.collector.GetDailyStats(today)
	if err == nil && ds != nil {
		result["total_requests"] = ds.TotalRequests
		result["total_tokens_in"] = ds.TotalTokensIn
		result["total_tokens_out"] = ds.TotalTokensOut
		result["avg_latency"] = ds.AvgLatencyMs
	}

	mu, err := a.collector.GetModelUsage(today)
	if err == nil {
		result["model_usage"] = mu
	}

	cd, err := a.collector.GetComplexityDistribution(today)
	if err == nil {
		result["complexity_dist"] = cd
	}

	rl, err := a.collector.GetRecentLogs(20)
	if err == nil {
		result["recent_logs"] = rl
	}

	return result
}

// GetProxyStatus returns whether the proxy is running and on which port.
func (a *App) GetProxyStatus() map[string]any {
	status := map[string]any{
		"running": false,
		"port":    0,
	}
	if a.proxy != nil {
		status["running"] = true
		status["port"] = a.proxy.Port()
	}
	return status
}

// StartProxy starts the proxy server on the given port.
func (a *App) StartProxy(port int) string {
	return a.startProxy(port)
}

func (a *App) startProxy(port int) string {
	// Stop existing proxy if running
	if a.proxy != nil {
		a.proxy.Stop()
		a.proxy = nil
	}

	a.proxy = proxy.New(port, a.engine)
	if err := a.proxy.Start(); err != nil {
		return "FAIL: " + err.Error()
	}

	// Save proxy config
	if a.db != nil {
		a.db.SetConfig("proxy_enabled", "true")
		a.db.SetConfig("proxy_port", fmt.Sprintf("%d", port))
	}

	return "OK"
}

// StopProxy stops the proxy server.
func (a *App) StopProxy() string {
	if a.proxy != nil {
		a.proxy.Stop()
		a.proxy = nil
	}
	if a.db != nil {
		a.db.SetConfig("proxy_enabled", "false")
	}
	return "OK"
}

// GetConfig returns a config value by key.
func (a *App) GetConfig(key string) string {
	if a.db == nil {
		return ""
	}
	val, _ := a.db.GetConfig(key)
	return val
}

// SetConfig sets a config key/value pair.
func (a *App) SetConfig(key, value string) string {
	if a.db == nil {
		return "error: database not initialized"
	}
	if err := a.db.SetConfig(key, value); err != nil {
		return "error: " + err.Error()
	}
	return "OK"
}

// marshalJSON is a helper to JSON-marshal a value, returning empty slice on error.
func marshalJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("[]")
	}
	return b
}
