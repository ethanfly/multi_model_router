package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	app.SetTrayIcon(createTrayIcon())

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
