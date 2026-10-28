package desktop

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"sort"
	"strings"

	"snaptrans/internal/capture"
	"snaptrans/internal/config"
	"snaptrans/internal/hotkeys"
)

func (a *App) TriggerReply() error { return a.triggerCaptureMode("reply") }

func validRegion(region TranslateRegionRequest) bool {
	return region.Width >= 8 && region.Height >= 8 && region.Width <= 16384 && region.Height <= 16384 &&
		region.X >= -1_000_000 && region.X <= 1_000_000 && region.Y >= -1_000_000 && region.Y <= 1_000_000
}

func validateShortcuts(cfg config.Config) error {
	seen := map[string]bool{}
	for _, shortcut := range []string{cfg.ShortcutKey, cfg.ScreenshotShortcutKey, cfg.ReplyShortcutKey} {
		mods, key, err := hotkeys.ParseShortcut(shortcut)
		if err != nil {
			return fmt.Errorf("invalid shortcut %q: %w", shortcut, err)
		}
		sort.Slice(mods, func(i, j int) bool { return mods[i] < mods[j] })
		identity := fmt.Sprintf("%v:%v", mods, key)
		if seen[identity] {
			return errors.New("translation, screenshot and reply shortcuts must be different")
		}
		seen[identity] = true
	}
	return nil
}

func (a *App) registerReplyShortcut(shortcut string) error {
	registration, err := hotkeys.Register(shortcut, func() { _ = a.TriggerReply() })
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.replyShortcut != nil {
		_ = a.replyShortcut.Unregister()
	}
	a.replyShortcut = registration
	return nil
}

func (a *App) replySource(ctx context.Context, cfg config.Config, request TextActionRequest, frame image.Image) (string, error) {
	if source := strings.TrimSpace(request.Text); source != "" {
		return source, nil
	}
	if request.Region == nil || frame == nil {
		return "", errors.New("no captured reply context is available; capture again")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cropped, _, err := cropTranslationRegion(frame, *request.Region)
	if err != nil {
		return "", err
	}
	encoded, err := capture.EncodePNGBytes(cropped)
	if err != nil {
		return "", err
	}
	result, err := a.runOCR(ctx, cfg, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(encoded))
	if err != nil {
		return "", err
	}
	source := strings.TrimSpace(result.Text)
	if source == "" {
		return "", errors.New("OCR returned no text; select the message text and try again")
	}
	return source, nil
}
