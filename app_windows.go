//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"

	"multi_model_router/internal/autostart"
	"multi_model_router/internal/wintray"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

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
