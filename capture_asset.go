package main

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
)

const captureAssetPath = "/capture/current.png"

type captureAssets struct {
	mu     sync.RWMutex
	nextID uint64
	images map[uint64][]byte
}

func newCaptureAssets() *captureAssets {
	return &captureAssets{images: make(map[uint64][]byte)}
}

// Store retains a few recent frames so a newer hotkey press cannot replace
// an image while the WebView is still decoding the preceding capture event.
func (assets *captureAssets) Store(image []byte) string {
	assets.mu.Lock()
	defer assets.mu.Unlock()

	assets.nextID++
	id := assets.nextID
	assets.images[id] = image
	if id > 3 {
		delete(assets.images, id-3)
	}
	return fmt.Sprintf("%s?v=%d", captureAssetPath, id)
}

func (assets *captureAssets) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if request.URL.Path != captureAssetPath {
		http.NotFound(response, request)
		return
	}

	id, err := strconv.ParseUint(request.URL.Query().Get("v"), 10, 64)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	assets.mu.RLock()
	image, ok := assets.images[id]
	assets.mu.RUnlock()
	if !ok {
		http.NotFound(response, request)
		return
	}

	response.Header().Set("Content-Type", "image/png")
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Content-Length", strconv.Itoa(len(image)))
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(image)
}
