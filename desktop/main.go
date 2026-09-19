// Command pastlog-desktop is the Wails bootstrap of pastlog Desktop: window
// config, asset server, and the bound app services. All data behavior lives
// in internal/* — this file is glue only (TASK-DESKTOP.md §4.1).
//
// Untagged on purpose: the wails CLI's binding-generation pass builds this
// package without custom tags (R-D14). The frontend/dist embed target is
// kept non-empty by the committed .gitkeep placeholder.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wrinfotel/pastlog/desktop/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	service := app.New()
	if err := wails.Run(&options.App{
		Title:     "pastlog Desktop",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Bind: []interface{}{service},
	}); err != nil {
		log.Fatal(err)
	}
}
