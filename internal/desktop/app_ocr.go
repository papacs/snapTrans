package desktop

import (
	"context"
	"crypto/sha256"
	"errors"
	"snaptrans/internal/config"
	"snaptrans/internal/ocr"
	"strings"
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
	ctx, cancel := context.WithTimeout(a.lifetimeContext(), 30*time.Second)
	defer cancel()
	startedAt := time.Now()
	err := worker.Start(ctx)
	// A background failure must not replace the active translation UI. Log
	// stages only: engine errors may contain OCR output or local file paths.
	a.mu.Lock()
	current := a.ocrWorker == worker
	a.mu.Unlock()
	if current && a.log != nil {
		a.log.Infof("OCR warm-up completed startup_ms=%d ready=%t error_class=%s", time.Since(startedAt).Milliseconds(), err == nil, ocrErrorClass(err))
	}
}

func ocrErrorClass(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "timed out") || strings.Contains(message, "timeout") {
		return "deadline"
	}
	if strings.Contains(message, "pipe") || strings.Contains(message, "eof") || strings.Contains(message, "process exited") || strings.Contains(message, "write command") {
		return "transport"
	}
	if strings.Contains(message, "init") || strings.Contains(message, "start") || strings.Contains(message, "not found") || strings.Contains(message, "executable") || strings.Contains(message, "closed") {
		return "init"
	}
	return "error"
}

// Keep only the most recent successful result across captures. The image hash
// and OCR settings must match exactly; never retain screenshot bytes.
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
	startedAt := time.Now()
	mode, fallbackReason := "cache", "none"
	var persistentElapsed, oneShotElapsed time.Duration
	result, err := a.ocrCache.Run(ctx, cfg, data, func() (ocr.Result, error) {
		a.mu.Lock()
		worker := a.ocrWorker
		if !cfg.PersistentOCR || a.ocrWorkerConfig != workerSettings(cfg) {
			worker = nil
		}
		a.mu.Unlock()
		if worker != nil {
			mode = "persistent"
			persistentStarted := time.Now()
			result, err := worker.Run(ctx, data)
			persistentElapsed = time.Since(persistentStarted)
			if err == nil {
				return result, nil
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return ocr.Result{}, err
			}
			fallbackReason = ocrErrorClass(err)
		}
		mode = "oneshot"
		oneShotStarted := time.Now()
		fallback := ocr.NewRapidOCR(cfg.RapidOCRPath, time.Duration(cfg.RapidOCRTimeoutSeconds)*time.Second)
		result, err := fallback.ExtractResult(ctx, data)
		oneShotElapsed = time.Since(oneShotStarted)
		return result, err
	})
	if mode == "cache" && err != nil {
		mode = "none"
	}
	if a.log != nil {
		a.log.Infof("OCR request mode=%s cache_hit=%t persistent_configured=%t fallback=%t persistent_ms=%d oneshot_ms=%d total_ms=%d error_class=%s fallback_reason=%s",
			mode, mode == "cache", cfg.PersistentOCR, fallbackReason != "none", persistentElapsed.Milliseconds(), oneShotElapsed.Milliseconds(), time.Since(startedAt).Milliseconds(), ocrErrorClass(err), fallbackReason)
	}
	return result, err
}
