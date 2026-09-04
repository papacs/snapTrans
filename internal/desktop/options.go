package desktop

import (
	"io/fs"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailswindows "github.com/wailsapp/wails/v2/pkg/options/windows"
)

// NewOptions wires the desktop controller to the shared Wails window.
func NewOptions(app *App, assets fs.FS) *options.App {
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
