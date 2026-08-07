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
	instance, err := singleinstance.Acquire(instanceMutexName)
	if err != nil {
		if errors.Is(err, singleinstance.ErrAlreadyRunning) {
			showAlreadyRunningMessage()
			return
		}
		log.Fatal(err)
	}
	defer instance.Release()

	app := NewApp()

	runErr := wails.Run(&options.App{
		Title:            "snapTrans",
		Width:            1280,
		Height:           800,
		Frameless:        true,
		AlwaysOnTop:      true,
		StartHidden:      true,
		DisableResize:    true,
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &wailswindows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			DisableWindowIcon:    false,
			ContentProtection:    true,
		},
	})
	if runErr != nil {
		log.Fatal(runErr)
	}
}

func showAlreadyRunningMessage() {
	caption, _ := windows.UTF16PtrFromString("snapTrans")
	text, _ := windows.UTF16PtrFromString("snapTrans is already running. Look for its icon in the system tray.")
	ret, _ := windows.MessageBox(0, text, caption, windows.MB_OK|windows.MB_ICONINFORMATION)
	_ = ret
}
