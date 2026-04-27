package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"multi_model_router/internal/agentconfig"
	"multi_model_router/internal/config"
	"multi_model_router/internal/core"
	"multi_model_router/internal/provider"
	"multi_model_router/internal/router"
	"multi_model_router/internal/stats"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the main Wails binding struct.
type App struct {
	core         *core.Core
	ctx          context.Context
	isQuitting   atomic.Bool
	trayIconData []byte
}

// NewApp creates a new App instance with default config.
func NewApp() *App {
	return &App{core: core.New(config.Default())}
}

// setTrayIcon sets the system tray icon PNG data (internal, not exposed to frontend).
func (a *App) setTrayIcon(data []byte) {
	a.trayIconData = data
}

// --- Window control methods ---

func (a *App) MinimizeWindow() {
	wailsRuntime.WindowMinimise(a.ctx)
}

func (a *App) ToggleMaximizeWindow() bool {
	if wailsRuntime.WindowIsMaximised(a.ctx) {
		wailsRuntime.WindowUnmaximise(a.ctx)
		return false
	}
	wailsRuntime.WindowMaximise(a.ctx)
	return true
}

func (a *App) IsWindowMaximized() bool {
	return wailsRuntime.WindowIsMaximised(a.ctx)
}

func (a *App) HideWindow() {
	wailsRuntime.WindowHide(a.ctx)
}

// --- Frontend-bound methods (delegate to Core) ---

func (a *App) GetModels() []core.ModelJSON {
	return a.core.GetModels()
}

func (a *App) SaveModel(m core.ModelJSON) error {
	return a.core.SaveModel(m)
}

func (a *App) DeleteModel(id string) error {
	return a.core.DeleteModel(id)
}

func (a *App) TestModel(m core.ModelJSON) string {
	result, _ := a.core.TestModel(a.ctx, m)
	return result
}

func (a *App) ExportModels(password string) (string, error) {
	return a.core.ExportModels(password)
}

func (a *App) ImportModels(jsonData, password string) (string, error) {
	count, err := a.core.ImportModels(jsonData, password)
	if err != nil {
		return fmt.Sprintf("%d", count), err
	}
	return fmt.Sprintf("%d", count), nil
}

// SaveExportFile opens a save dialog and writes the export JSON to the chosen file.
func (a *App) SaveExportFile(jsonData string) error {
	filePath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		DefaultFilename: "models_export.json",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "JSON Files", Pattern: "*.json"},
		},
	})
	if err != nil || filePath == "" {
		return fmt.Errorf("no file selected")
	}
	return os.WriteFile(filePath, []byte(jsonData), 0644)
}

// ReadImportFile opens a file dialog and returns the file content.
func (a *App) ReadImportFile() (string, error) {
	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "JSON Files", Pattern: "*.json"},
		},
	})
	if err != nil || filePath == "" {
		return "", fmt.Errorf("no file selected")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(data), nil
}

// SendChat routes a chat request through the engine and streams results back
// to the frontend via Wails events.
func (a *App) SendChat(req core.ChatRequest) core.ChatResponse {
	if a.core.Engine() == nil {
		return core.ChatResponse{Status: "error", Error: "engine not initialized"}
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

	result := a.core.Engine().Route(a.ctx, routeReq)
	if result == nil {
		return core.ChatResponse{Status: "error", Error: "no result from router"}
	}

	resp := core.ChatResponse{
		ModelName:       result.ModelName,
		Provider:        result.Provider,
		Complexity:      router.Complexity(result.Complexity).String(),
		Status:          result.Status,
		Error:           result.ErrorMsg,
		RouteMode:       router.RouteMode(result.RouteMode).String(),
		Diagnostics:     diagnosticsSummary(result.Diagnostics),
		DiagnosticsJSON: diagnosticsJSON(result.Diagnostics),
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
			"content":         fullContent,
			"model":           result.ModelName,
			"diagnostics":     diagnosticsSummary(result.Diagnostics),
			"diagnosticsJson": diagnosticsJSON(result.Diagnostics),
		})

		// Log to stats
		tokensIn := 0
		tokensOut := 0
		if usage != nil {
			tokensIn = usage.InputTokens
			tokensOut = usage.OutputTokens
		}
		_ = a.core.LogRequest(&stats.RequestLog{
			ID:              router.NewUUID(),
			ModelID:         result.ModelName,
			Source:          "gui",
			Complexity:      router.Complexity(result.Complexity).String(),
			RouteMode:       resp.RouteMode,
			Status:          "success",
			TokensIn:        tokensIn,
			TokensOut:       tokensOut,
			LatencyMs:       result.LatencyMs,
			Diagnostics:     diagnosticsSummary(result.Diagnostics),
			DiagnosticsJSON: diagnosticsJSON(result.Diagnostics),
			CreatedAt:       time.Now(),
		})
	}()

	return resp
}

func diagnosticsSummary(d *router.RouteDiagnostics) string {
	if d == nil {
		return ""
	}
	return d.Summary
}

func diagnosticsJSON(d *router.RouteDiagnostics) string {
	if d == nil {
		return ""
	}
	return d.ToJSON()
}

func (a *App) GetDashboardStats() map[string]any {
	return a.core.GetDashboardStats()
}

func (a *App) GetDashboardLogs(page, pageSize int) map[string]any {
	return a.core.GetDashboardLogs(page, pageSize)
}

func (a *App) GetProxyStatus() map[string]any {
	s := a.core.GetProxyStatus()
	return map[string]any{
		"running": s.Running,
		"port":    s.Port,
		"mode":    s.Mode,
	}
}

func (a *App) StartProxy(port int) string {
	if err := a.core.StartProxy(port); err != nil {
		return "FAIL: " + err.Error()
	}
	return "OK"
}

func (a *App) StopProxy() string {
	_ = a.core.StopProxy()
	return "OK"
}

func (a *App) GetProxyMode() string {
	return a.core.GetProxyMode()
}

func (a *App) SetProxyMode(mode string) string {
	if err := a.core.SetProxyMode(mode); err != nil {
		return "error: " + err.Error()
	}
	go a.refreshTrayMenu()
	return "OK"
}

func (a *App) GetClassifierConfig() string {
	cfg := a.core.GetClassifierConfig()
	return cfg.ToJSON()
}

func (a *App) GetDefaultClassifierConfig() string {
	return router.DefaultClassifierConfig().ToJSON()
}

func (a *App) SetClassifierConfig(jsonStr string) string {
	cfg := router.ParseClassifierConfig(jsonStr)
	if err := a.core.SetClassifierConfig(cfg); err != nil {
		return "error: " + err.Error()
	}
	return "OK"
}

func (a *App) GetActiveModels() string {
	models := a.core.GetModels()
	var active []core.ModelJSON
	for _, m := range models {
		if m.IsActive {
			active = append(active, m)
		}
	}
	return string(marshalJSON(active))
}

func (a *App) GetConfig(key string) string {
	return a.core.GetConfig(key)
}

func (a *App) SetConfig(key, value string) string {
	if err := a.core.SetConfig(key, value); err != nil {
		return "error: " + err.Error()
	}
	return "OK"
}

func (a *App) PreviewAgentConfig(port int, apiKey, model string) string {
	return a.configureAgents(port, apiKey, model, true)
}

func (a *App) ApplyAgentConfig(port int, apiKey, model string) string {
	return a.configureAgents(port, apiKey, model, false)
}

func (a *App) configureAgents(port int, apiKey, model string, dryRun bool) string {
	if port <= 0 {
		status := a.core.GetProxyStatus()
		port = status.Port
		if port <= 0 {
			port = a.core.Config().ProxyPort
		}
	}
	if apiKey == "" {
		apiKey = a.core.GetConfig("proxy_api_key")
	}
	if model == "" {
		model = "auto"
	}

	result, err := agentconfig.Configure(agentconfig.Options{
		Apps:   []string{"all"},
		Port:   port,
		APIKey: apiKey,
		Model:  model,
		DryRun: dryRun,
	})
	if err != nil {
		return string(marshalJSON(map[string]any{
			"error": err.Error(),
		}))
	}
	return string(marshalJSON(result))
}

// marshalJSON is a helper to JSON-marshal a value, returning empty slice on error.
func marshalJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("[]")
	}
	return b
}
