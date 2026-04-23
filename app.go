package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"

	"multi_model_router/internal/autostart"
	"multi_model_router/internal/config"
	"multi_model_router/internal/core"
	"multi_model_router/internal/router"
	"multi_model_router/internal/provider"
	"multi_model_router/internal/stats"
	"multi_model_router/internal/wintray"

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

// startup is called when the Wails app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	if err := a.core.Init(); err != nil {
		fmt.Printf("core init error: %v\n", err)
		return
	}

	// Start system tray on a dedicated OS thread
	go wintray.Run("Multi-Model Router", a.trayIconData,
		func() { wailsRuntime.WindowShow(a.ctx) },
		func() { a.QuitApp() },
	)

	// Build initial tray menu after a short delay (let tray init first)
	go func() {
		time.Sleep(500 * time.Millisecond)
		a.refreshTrayMenu()
	}()

	// Auto-start proxy if enabled
	if val, _ := a.core.DB().GetConfig("proxy_enabled"); val == "true" {
		port := a.core.Config().ProxyPort
		if v, _ := a.core.DB().GetConfig("proxy_port"); v != "" {
			fmt.Sscanf(v, "%d", &port)
		}
		_ = a.core.StartProxy(port)
	}
}

// shutdown is called when the Wails app closes.
func (a *App) shutdown(ctx context.Context) {
	a.core.Close()
	wintray.Quit()
}

// onBeforeClose intercepts the window close event.
func (a *App) onBeforeClose(ctx context.Context) bool {
	if a.isQuitting.Load() {
		return false
	}
	wailsRuntime.WindowHide(a.ctx)
	return true
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

func (a *App) QuitApp() {
	a.isQuitting.Store(true)
	a.core.Close()
	wintray.Quit()
	os.Exit(0)
}

// --- Auto-start methods ---

func (a *App) GetAutoStart() bool {
	return autostart.IsEnabled()
}

func (a *App) SetAutoStart(enabled bool) string {
	if enabled {
		if err := autostart.Enable(); err != nil {
			return "error: " + err.Error()
		}
		return "OK"
	}
	if err := autostart.Disable(); err != nil {
		return "error: " + err.Error()
	}
	return "OK"
}

// --- Window drag via Windows API ---

var (
	dragUser32              = windows.NewLazyDLL("user32.dll")
	procGetForegroundWindow = dragUser32.NewProc("GetForegroundWindow")
	procReleaseCapture      = dragUser32.NewProc("ReleaseCapture")
	procPostMessageW        = dragUser32.NewProc("PostMessageW")
)

const (
	WM_NCLBUTTONDOWN = 0x00A1
	HTCAPTION        = 0x0002
)

func (a *App) StartWindowDrag() {
	procReleaseCapture.Call()
	hwnd, _, _ := procGetForegroundWindow.Call()
	procPostMessageW.Call(hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0)
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
		tokensIn := 0
		tokensOut := 0
		if usage != nil {
			tokensIn = usage.InputTokens
			tokensOut = usage.OutputTokens
		}
		_ = a.core.LogRequest(&stats.RequestLog{
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
	}()

	return resp
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

// --- Tray menu ---

func (a *App) refreshTrayMenu() {
	lang := a.core.GetConfig("language")
	if lang == "" {
		lang = "en"
	}

	mode := a.core.GetProxyMode()
	status := a.core.GetProxyStatus()

	showLabel := "Show"
	quitLabel := "Quit"
	proxyStatusLabel := "API Proxy: Stopped"
	modeLabel := "Mode: Auto"
	langLabel := "Language: English"

	if lang == "zh" {
		showLabel = "显示"
		quitLabel = "退出"
		if status.Running {
			proxyStatusLabel = "API 代理: 运行中 ✓"
		} else {
			proxyStatusLabel = "API 代理: 已停止"
		}
		switch mode {
		case "auto":
			modeLabel = "模式: 自动"
		case "manual":
			modeLabel = "模式: 手动"
		case "race":
			modeLabel = "模式: 竞速"
		}
		langLabel = "语言: 中文"
	} else {
		if status.Running {
			proxyStatusLabel = "API Proxy: Running ✓"
		} else {
			proxyStatusLabel = "API Proxy: Stopped"
		}
		switch mode {
		case "auto":
			modeLabel = "Mode: Auto"
		case "manual":
			modeLabel = "Mode: Manual"
		case "race":
			modeLabel = "Mode: Race"
		}
		langLabel = "Language: English"
	}

	items := []wintray.MenuItem{
		{ID: "show", Label: showLabel, Handler: func() { wailsRuntime.WindowShow(a.ctx) }},
		{ID: "sep1", Sep: true},
		{ID: "proxy_status", Label: proxyStatusLabel},
		{ID: "mode", Label: modeLabel, Handler: func() {
			next := map[string]string{"auto": "manual", "manual": "race", "race": "auto"}
			newMode := next[mode]
			_ = a.core.SetProxyMode(newMode)
			a.refreshTrayMenu()
		}},
		{ID: "lang", Label: langLabel, Handler: func() {
			newLang := "en"
			if lang == "en" {
				newLang = "zh"
			}
			_ = a.core.SetConfig("language", newLang)
			a.refreshTrayMenu()
		}},
		{ID: "sep2", Sep: true},
		{ID: "quit", Label: quitLabel, Handler: func() { a.QuitApp() }},
	}

	wintray.UpdateMenu(items)
}

// marshalJSON is a helper to JSON-marshal a value, returning empty slice on error.
func marshalJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("[]")
	}
	return b
}
