package main

import (
	"embed"
	"os"

	"multi_model_router/internal/cli"

	"github.com/wailsapp/wails/v2"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Disable GPU acceleration — fixes WebView2 GPU process crash
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--disable-gpu --disable-software-rasterizer")
}

func main() {
	// If any CLI arguments are passed, run in CLI mode.
	if len(os.Args) > 1 {
		if err := cli.NewRootCommand().Execute(); err != nil {
			os.Exit(1)
		}
		return
	}

	runWails()
}

func runWails() {
	app := NewApp()
	app.setTrayIcon(createTrayIcon())

	err := wails.Run(&options.App{
		Title:  "Multi-Model Router",
		Width:  1200,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:     app.startup,
		OnShutdown:    app.shutdown,
		OnBeforeClose: app.onBeforeClose,
		Frameless:     true,
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.ethanfly.multimodelrouter",
			OnSecondInstanceLaunch: func(si options.SecondInstanceData) {
				wailsRuntime.WindowShow(app.ctx)
			},
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
