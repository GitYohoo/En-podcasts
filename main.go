package main

import (
	"embed"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	winoptions "github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	webviewUserDataPath := webviewDataPath()

	err := wails.Run(&options.App{
		Title:     "中文语音转英文播客工作台",
		Width:     1480,
		Height:    960,
		MinWidth:  1180,
		MinHeight: 760,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 18, G: 13, B: 15, A: 1},
		Windows: &winoptions.Options{
			WebviewGpuIsDisabled:                true,
			WebviewDisableRendererCodeIntegrity: true,
			WebviewUserDataPath:                 webviewUserDataPath,
		},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

func webviewDataPath() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}

	return filepath.Join(filepath.Dir(exePath), "build", "webview2data")
}
