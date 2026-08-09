package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCaptureAssetServesVersionedImageWithoutCaching(t *testing.T) {
	assets := newCaptureAssets()
	first := []byte("first-png")
	second := []byte("second-png")
	firstURL := assets.Store(first)
	secondURL := assets.Store(second)

	request := httptest.NewRequest(http.MethodGet, firstURL, nil)
	response := httptest.NewRecorder()
	assets.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "image/png", response.Header().Get("Content-Type"))
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	body, err := io.ReadAll(response.Result().Body)
	require.NoError(t, err)
	require.Equal(t, first, body)
	require.NotEqual(t, firstURL, secondURL)
}

func TestCaptureAssetRejectsUnknownVersion(t *testing.T) {
	assets := newCaptureAssets()
	request := httptest.NewRequest(http.MethodGet, "/capture/current.png?v=404", nil)
	response := httptest.NewRecorder()

	assets.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code)
}
