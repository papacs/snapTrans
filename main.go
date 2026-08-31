package main

import (
	"embed"
	"errors"
	"log"

	"snaptrans/internal/singleinstance"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailswindows "github.com/wailsapp/wails/v2/pkg/options/windows"
	"golang.org/x/sys/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

const instanceMutexName = "snapTrans.single-instance"

func main() {
	// Wails binding generation only reflects methods; it does not run the app.
	if !generatingBindings {
		instance, err := singleinstance.Acquire(instanceMutexName)
		if err != nil {
			if errors.Is(err, singleinstance.ErrAlreadyRunning) {
				showAlreadyRunningMessage()
				return
			}
			log.Fatal(err)
		}
		defer instance.Release()
	}

	app := NewApp()

	runErr := wails.Run(newAppOptions(app))
	if runErr != nil {
		log.Fatal(runErr)
	}
}

func newAppOptions(app *App) *options.App {
	return &options.App{
		Title:            "snapTrans",
		Width:            1280,
		Height:           800,
		Frameless:        true,
		AlwaysOnTop:      true,
		StartHidden:      true,
		DisableResize:    true,
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 255},
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: app.captureAssets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &wailswindows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
			// Settings share this window. Capture hides it before reading the desktop;
			// global content protection would black out settings screenshots.
			ContentProtection: false,
		},
	}
}

func showAlreadyRunningMessage() {
	caption, _ := windows.UTF16PtrFromString("snapTrans")
	text, _ := windows.UTF16PtrFromString("snapTrans is already running. Look for its icon in the system tray.")
	ret, _ := windows.MessageBox(0, text, caption, windows.MB_OK|windows.MB_ICONINFORMATION)
	_ = ret
}
