package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWindowContentRemainsCapturable(t *testing.T) {
	appOptions := newAppOptions(NewApp())

	require.NotNil(t, appOptions.Windows)
	require.True(t, appOptions.StartHidden)
	require.False(t, appOptions.Windows.ContentProtection)
	require.False(t, appOptions.Windows.WebviewIsTransparent)
	require.False(t, appOptions.Windows.WindowIsTranslucent)
	require.Equal(t, uint8(255), appOptions.BackgroundColour.A)
	require.NotNil(t, appOptions.AssetServer)
	require.NotNil(t, appOptions.AssetServer.Handler)
}
