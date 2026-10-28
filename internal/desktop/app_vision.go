package desktop

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"snaptrans/internal/capture"
	"snaptrans/internal/config"
	"snaptrans/internal/translator"
)

func (a *App) startVisionRegion(cfg config.Config, frame image.Image, region TranslateRegionRequest, direction translator.Direction, generation int) {
	a.mu.Lock()
	if a.processing != nil {
		a.processing()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.processing = cancel
	a.mu.Unlock()
	go func() {
		defer cancel()
		started := time.Now()
		dataURL, err := capture.EncodeVisionCrop(frame, image.Rect(region.X, region.Y, region.X+region.Width, region.Y+region.Height))
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			a.emitError("translation", err, generation)
			return
		}
		if a.log != nil {
			a.log.Infof("generation=%d input_mode=image image_prepare_ms=%d image_bytes=%d", generation, time.Since(started).Milliseconds(), len(dataURL)*3/4)
		}
		a.processVisionImage(ctx, cfg, dataURL, direction, generation, started)
	}()
}

func (a *App) processVisionImage(ctx context.Context, cfg config.Config, dataURL string, direction translator.Direction, generation int, started time.Time) {
	if ctx.Err() != nil {
		return
	}
	runtime.EventsEmit(a.ctx, "text-regions", textRegionsPayload{Generation: generation, Source: "image"})
	runtime.EventsEmit(a.ctx, "translation-start", sentinelPayload{Generation: generation})
	translationCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TranslationTimeoutSeconds)*time.Second)
	defer cancel()
	client := translator.NewOpenAICompatible(translationTaskOptions(cfg))
	var output strings.Builder
	first := true
	err := client.TranslateImage(translationCtx, dataURL, direction, func(resolved translator.Direction) {
		direction = resolved
		if ctx.Err() == nil {
			runtime.EventsEmit(a.ctx, "translation-direction", translationDirectionPayload{Generation: generation, Direction: string(resolved)})
		}
	}, func(token string) {
		if ctx.Err() != nil {
			return
		}
		if first {
			first = false
			if a.log != nil {
				a.log.Infof("generation=%d input_mode=image first_token_ms=%d", generation, time.Since(started).Milliseconds())
			}
		}
		output.WriteString(token)
		runtime.EventsEmit(a.ctx, "translation-token", translationTokenPayload{Generation: generation, Token: token})
	})
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("vision translation timed out after %d seconds", cfg.TranslationTimeoutSeconds)
		}
		a.emitError("translation", err, generation)
		return
	}
	runtime.EventsEmit(a.ctx, "translation-done", sentinelPayload{Generation: generation})
	if a.log != nil {
		a.log.Infof("generation=%d input_mode=image total_ms=%d", generation, time.Since(started).Milliseconds())
	}
	a.saveHistory("截图直读 / Screenshot", output.String(), string(direction))
}

func replyInputMode(cfg config.Config, request TextActionRequest) string {
	if request.InputMode != "" {
		return request.InputMode
	}
	if cfg.DirectImageInput && request.Region != nil {
		return "image"
	}
	return "ocr"
}
