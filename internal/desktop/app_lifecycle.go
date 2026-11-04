package desktop

import (
	"context"

	"snaptrans/internal/codex"
)

func (a *App) lifetimeContext() context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lifetimeContextLocked()
}

func (a *App) lifetimeContextLocked() context.Context {
	if a.lifetimeCtx == nil {
		a.lifetimeCtx, a.lifetimeStop = context.WithCancel(context.Background())
		if a.shuttingDown {
			a.lifetimeStop()
		}
	}
	return a.lifetimeCtx
}

func (a *App) startCaptureContextLocked() {
	if a.captureCancel != nil {
		a.captureCancel()
	}
	a.captureCtx, a.captureCancel = context.WithCancel(a.lifetimeContextLocked())
	a.captureMode = ""
}

func (a *App) captureContext(epoch uint64) context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.captureContextLocked(epoch)
}

func (a *App) captureContextLocked(epoch uint64) context.Context {
	if !a.shuttingDown && a.captureEpoch == epoch && a.captureCtx != nil {
		return a.captureCtx
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func (a *App) stopBackgroundWork() {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	a.mu.Lock()
	a.shuttingDown = true
	a.lifetimeContextLocked()
	a.lifetimeStop()
	a.mu.Unlock()
	a.cancelTranslationWarmups()
	a.invalidateCaptureRequest()
	a.cancelProcessing()
	a.cancelTextAction()
	codex.CloseIdle()
}

// Standalone editor OCR can run alongside translation, but it still belongs
// to the current capture and must stop when that capture is dismissed.
func (a *App) newExtractionContext(epoch uint64) (context.Context, context.CancelFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.shuttingDown || a.captureEpoch != epoch {
		return nil, nil
	}
	if a.extractionCtx == nil {
		a.extractionCtx, a.extractionCancel = context.WithCancel(a.lifetimeContextLocked())
	}
	return context.WithCancel(a.extractionCtx)
}
