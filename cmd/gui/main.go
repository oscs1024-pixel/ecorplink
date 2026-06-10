package main

import (
	"embed"
	"log"

	"ecorplink/internal/embeddeddaemon"
	"ecorplink/internal/gui"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:assets
var assets embed.FS

//go:embed daemon/ecorplink-daemon
var daemonFS embed.FS

func main() {
	windowState := gui.LoadWindowState()
	daemonPath, err := embeddeddaemon.Ensure(embeddeddaemon.Source{
		FS:     daemonFS,
		Path:   "daemon/ecorplink-daemon",
		SHA256: embeddedDaemonSHA256,
	}, "")
	if err != nil {
		log.Printf("install embedded ecorplink daemon: %v", err)
	}

	app := application.New(application.Options{
		Name:        "ECorpLink",
		Description: "CorpLink VPN client with split routing",
		Services: []application.Service{
			application.NewService(gui.NewService(gui.Options{DaemonPath: daemonPath, AppVersion: Version})),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:           "ECorpLink",
		URL:             "/",
		Width:           windowState.Width,
		Height:          windowState.Height,
		MinWidth:        640,
		MinHeight:       360,
		DevToolsEnabled: true,
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
