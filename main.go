//go:build desktop

package main

import (
	"embed"
	"log"
	"os"
	"slices"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/zonnhq/node-gui/internal/autostart"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "Zonn Node",
		Width:            1120,
		Height:           760,
		MinWidth:         900,
		MinHeight:        600,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 26, G: 23, B: 32, A: 255}, // Drapery Drama navy #1A1720
		// Closing the window keeps the node contributing in the background;
		// the tray (or, on macOS, the app menu) reopens it or quits.
		HideWindowOnClose: true,
		// Started at sign in, the app stays in the background.
		StartHidden: slices.Contains(os.Args[1:], autostart.HiddenFlag),
		// Launching the app again opens the running one's window instead of
		// starting a second node.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "com.zonnhq.node",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.showWindow() },
		},
		Menu:       app.appMenu(),
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind:       []any{app},
	})
	if err != nil {
		log.Fatalf("wails: %v", err)
	}
}
