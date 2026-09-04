package main

import (
	"embed"
	"errors"
	"log"

	"snaptrans/internal/desktop"
	"snaptrans/internal/singleinstance"

	"github.com/wailsapp/wails/v2"
	"golang.org/x/sys/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

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

	app := desktop.NewApp(trayIcon)

	runErr := wails.Run(desktop.NewOptions(app, assets))
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
