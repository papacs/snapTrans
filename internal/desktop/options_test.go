package desktop

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSharedWindowAllowsSettingsScreenshots(t *testing.T) {
	appOptions := NewOptions(NewApp(nil), nil)

	require.NotNil(t, appOptions.Windows)
	require.True(t, appOptions.StartHidden)
	require.False(t, appOptions.Windows.ContentProtection, "settings share the capture window and must remain capturable")
	require.False(t, appOptions.Windows.WebviewIsTransparent)
	require.False(t, appOptions.Windows.WindowIsTranslucent)
	require.Equal(t, uint8(255), appOptions.BackgroundColour.A)
	require.NotNil(t, appOptions.AssetServer)
	require.NotNil(t, appOptions.AssetServer.Handler)
}
