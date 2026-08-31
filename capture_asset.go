package main

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
)

const captureAssetPath = "/capture/current.png"

type captureAssets struct {
	mu       sync.RWMutex
	nextID   uint64
	images   map[uint64][]byte
	bytes    int
	maxBytes int
}

func newCaptureAssets() *captureAssets {
	return &captureAssets{images: make(map[uint64][]byte), maxBytes: 96 << 20}
}

// Store retains a few recent frames so a newer hotkey press cannot replace
// an image while the WebView is still decoding the preceding capture event.
func (assets *captureAssets) Store(image []byte) string {
	assets.mu.Lock()
	defer assets.mu.Unlock()

	assets.nextID++
	id := assets.nextID
	assets.images[id] = image
	assets.bytes += len(image)
	// Manual scrolling capture stores a current frame and a stitched preview
	// per wheel gesture. Keep enough versions for the WebView to finish decoding
	// a preceding gesture while the next one is already being processed.
	// Keep the newest frame even when one exceptionally large capture exceeds
	// the budget. Evict oldest frames first, bounding both bytes and count.
	for len(assets.images) > 1 && (len(assets.images) > 8 || assets.bytes > assets.maxBytes) {
		oldest := id
		for candidate := range assets.images {
			if candidate < oldest {
				oldest = candidate
			}
		}
		assets.bytes -= len(assets.images[oldest])
		delete(assets.images, oldest)
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

func (assets *captureAssets) Clear() {
	assets.mu.Lock()
	defer assets.mu.Unlock()
	assets.images = make(map[uint64][]byte)
	assets.bytes = 0
	// Do not reset nextID: stale requests must never receive a different image.
}
