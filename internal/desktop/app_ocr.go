package desktop

import (
	"context"
	"crypto/sha256"
	"errors"
	"snaptrans/internal/config"
	"snaptrans/internal/ocr"
	"sync"
	"time"
)

type ocrWorkerSettings struct {
	path    string
	timeout int
}

func workerSettings(cfg config.Config) ocrWorkerSettings {
	return ocrWorkerSettings{cfg.RapidOCRPath, cfg.RapidOCRTimeoutSeconds}
}

func (a *App) syncOCRWorker(cfg config.Config) {
	a.mu.Lock()
	previous := a.ocrWorker
	settings := workerSettings(cfg)
	if cfg.PersistentOCR && previous != nil && a.ocrWorkerConfig == settings {
		a.mu.Unlock()
		return
	}
	var next *ocr.Worker
	if cfg.PersistentOCR {
		next = ocr.NewRapidOCRWorker(cfg.RapidOCRPath, time.Duration(cfg.RapidOCRTimeoutSeconds)*time.Second)
	}
	a.ocrWorker, a.ocrWorkerConfig = next, settings
	a.mu.Unlock()
	a.ocrCache.Clear()
	if previous != nil {
		previous.Close()
	}
	if next != nil {
		go a.warmOCRWorker(next)
	}
}

func (a *App) warmOCRWorker(worker *ocr.Worker) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := worker.Start(ctx); err != nil {
		// A background warm-up failure must not replace the active translation UI.
		// Foreground OCR will retry and surface any actionable failure.
		a.mu.Lock()
		current := a.ocrWorker == worker
		a.mu.Unlock()
		if current && a.log != nil {
			a.log.Errorf("OCR warm-up failed: %v", err)
		}
	}
}

// Keep only the most recent successful result; never retain screenshot bytes.
type ocrCacheKey struct {
	image    [32]byte
	settings ocrWorkerSettings
}
type ocrResultCache struct {
	mu     sync.Mutex
	key    ocrCacheKey
	result ocr.Result
	valid  bool
	epoch  uint64
}

func (c *ocrResultCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.valid = false
	c.result = ocr.Result{}
	c.epoch++
}
func (c *ocrResultCache) Run(ctx context.Context, cfg config.Config, data string, extract func() (ocr.Result, error)) (ocr.Result, error) {
	if err := ctx.Err(); err != nil {
		return ocr.Result{}, err
	}
	key := ocrCacheKey{sha256.Sum256([]byte(data)), workerSettings(cfg)}
	c.mu.Lock()
	epoch := c.epoch
	if c.valid && c.key == key {
		result := c.result
		result.Blocks = append([]ocr.Block(nil), result.Blocks...)
		c.mu.Unlock()
		return result, nil
	}
	c.mu.Unlock()
	result, err := extract()
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return ocr.Result{}, err
	}
	if result.Text != "" {
		c.mu.Lock()
		if c.epoch == epoch {
			c.key, c.result, c.valid = key, result, true
			c.result.Blocks = append([]ocr.Block(nil), result.Blocks...)
		}
		c.mu.Unlock()
	}
	return result, nil
}

func (a *App) runOCR(ctx context.Context, cfg config.Config, data string) (ocr.Result, error) {
	// One timeout covers worker startup, recognition, and any one-shot fallback.
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.RapidOCRTimeoutSeconds)*time.Second)
	defer cancel()
	return a.ocrCache.Run(ctx, cfg, data, func() (ocr.Result, error) {
		a.mu.Lock()
		worker := a.ocrWorker
		if !cfg.PersistentOCR || a.ocrWorkerConfig != workerSettings(cfg) {
			worker = nil
		}
		a.mu.Unlock()
		if worker != nil {
			result, err := worker.Run(ctx, data)
			if err == nil {
				return result, nil
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return ocr.Result{}, err
			}
		}
		fallback := ocr.NewRapidOCR(cfg.RapidOCRPath, time.Duration(cfg.RapidOCRTimeoutSeconds)*time.Second)
		return fallback.ExtractResult(ctx, data)
	})
}
