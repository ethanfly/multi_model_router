//go:build !windows

package main

import (
	"context"
	"fmt"
	"os"

	"multi_model_router/internal/autostart"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// startup is called when the Wails app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	if err := a.core.Init(); err != nil {
		fmt.Printf("core init error: %v\n", err)
		return
	}

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

// --- Window drag (no-op on non-Windows) ---

func (a *App) StartWindowDrag() {
	// No-op on non-Windows platforms
}

// --- Tray menu (no-op on non-Windows) ---

func (a *App) refreshTrayMenu() {
	// No-op on non-Windows platforms (system tray not available)
}
