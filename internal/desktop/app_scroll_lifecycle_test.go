package desktop

import (
	"context"
	"errors"
	"image"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"snaptrans/internal/capture"
)

func TestWindowTransitionsWaitForScrollCleanupAlreadyInProgress(t *testing.T) {
	for _, test := range []struct {
		name      string
		run       func(*App) error
		wantError string
	}{
		{"cancel", (*App).CancelScrollingScreenshot, ""},
		{"finish", func(a *App) error { _, err := a.FinishScrollingScreenshot(); return err }, "no scrolling capture"},
		{"hide", (*App).HideWindow, ""},
		{"settings", (*App).ShowSettings, ""},
		{"capture", func(a *App) error {
			_, _, started := a.beginCapture()
			if !started {
				return errors.New("new capture was dropped while the old scroll cleanup was pending")
			}
			return nil
		}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := NewApp(nil)
			frame := solidTestFrame(48, 64)
			finalCaptureStarted := make(chan struct{})
			releaseCapture := make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(releaseCapture) }) }
			t.Cleanup(release)
			calls := 0
			session, err := capture.StartManualScrollCaptureWithSource(context.Background(), frame.Bounds(), capture.ManualScrollOptions{}, func(image.Rectangle) (image.Image, error) {
				calls++
				if calls == 2 {
					close(finalCaptureStarted)
					<-releaseCapture
				}
				return frame, nil
			})
			require.NoError(t, err)
			app.scrollCapture = session
			app.captureInFlight = true
			app.windowVisible = true
			finished := make(chan error, 1)
			go func() { _, err := app.FinishScrollingScreenshot(); finished <- err }()
			select {
			case <-finalCaptureStarted:
			case <-time.After(5 * time.Second):
				t.Fatal("finish did not reach the final capture")
			}

			// Finish has taken the session, but native cleanup has not happened.
			// A nil session must not let another window transition bypass it.
			app.mu.Lock()
			taken := app.scrollCapture == nil
			app.mu.Unlock()
			require.True(t, taken)
			transitionStarted := make(chan struct{})
			transitioned := make(chan error, 1)
			go func() { close(transitionStarted); transitioned <- test.run(app) }()
			<-transitionStarted
			select {
			case err := <-transitioned:
				t.Fatalf("window transition bypassed pending scroll cleanup: %v", err)
			case <-time.After(25 * time.Millisecond):
			}
			release()
			select {
			case err := <-transitioned:
				if test.wantError == "" {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, test.wantError)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("window transition remained blocked after cleanup")
			}
			select {
			case err := <-finished:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("finished screenshot did not complete")
			}
		})
	}
}
