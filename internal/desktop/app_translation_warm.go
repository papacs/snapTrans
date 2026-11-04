package desktop

import (
	"context"
	"errors"
	"strings"
	"time"

	"snaptrans/internal/codex"
	"snaptrans/internal/config"
	"snaptrans/internal/translator"
)

const (
	translationWarmFreshFor = 60 * time.Second
	translationWarmTimeout  = 3 * time.Second
)

type translationWarmState struct {
	target   string
	active   bool
	warmedAt time.Time
}

func (state *translationWarmState) begin(target string, now time.Time) bool {
	if target == "" {
		return false
	}
	if state.target == target {
		if state.active || (!state.warmedAt.IsZero() && now.Sub(state.warmedAt) < translationWarmFreshFor) {
			return false
		}
	}
	state.target = target
	state.active = true
	return true
}

func (state *translationWarmState) finish(target string, now time.Time, success bool) {
	if state.target != target {
		return
	}
	state.active = false
	if success {
		state.warmedAt = now
	} else {
		state.warmedAt = time.Time{}
	}
}

// Snapshot the configuration and its cancellation parent before scheduling a
// goroutine. Shutdown or settings changes can then cancel even unstarted work.
func (a *App) translationWarmSnapshot() (config.Config, context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.shuttingDown {
		return config.Config{}, nil
	}
	if a.translationWarmCtx == nil {
		a.translationWarmCtx, a.translationWarmStop = context.WithCancel(a.lifetimeContextLocked())
	}
	return a.cfg.WithDefaults(), a.translationWarmCtx
}

func (a *App) cancelTranslationWarmups() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cancelTranslationWarmupsLocked()
}

func (a *App) cancelTranslationWarmupsLocked() {
	if a.translationWarmStop != nil {
		a.translationWarmStop()
		a.translationWarmCtx, a.translationWarmStop = nil, nil
	}
	a.translationWarmMu.Lock()
	a.translationWarm = translationWarmState{}
	a.translationWarmMu.Unlock()
}

func (a *App) prewarmTranslationConnection() {
	cfg, parent := a.translationWarmSnapshot()
	if parent == nil {
		return
	}
	if cfg.Provider == "codex" {
		ctx, cancel := context.WithTimeout(parent, 20*time.Second)
		go func() {
			defer cancel()
			var err error
			if cfg.DirectImageInput {
				err = codex.Warm(ctx, cfg.CodexPath, cfg.CodexModel)
			} else {
				// Guess the usual target without reading the screen. A different
				// direction or prompt cannot reuse this unused conversation.
				client := translator.NewOpenAICompatible(translationTaskOptions(cfg))
				err = client.WarmTranslation(ctx, translator.DirectionToChinese)
			}
			if err != nil && parent.Err() == nil && !errors.Is(err, context.Canceled) && a.log != nil {
				a.log.Errorf("Codex warm-up failed: %v", err)
			}
		}()
		return
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return
	}

	target := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	a.translationWarmMu.Lock()
	started := parent.Err() == nil && a.translationWarm.begin(target, time.Now())
	a.translationWarmMu.Unlock()
	if !started {
		return
	}

	go a.runTranslationWarmup(parent, cfg, target)
}

func (a *App) runTranslationWarmup(parent context.Context, cfg config.Config, target string) {
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(parent, translationWarmTimeout)
	defer cancel()
	client := translator.NewOpenAICompatible(translator.Options{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model})
	err := client.Warm(ctx)

	a.translationWarmMu.Lock()
	if parent.Err() == nil {
		a.translationWarm.finish(target, time.Now(), err == nil)
	}
	a.translationWarmMu.Unlock()
	if err == nil && parent.Err() == nil && a.log != nil {
		a.log.Infof("translation connection warmed warm_ms=%d", time.Since(startedAt).Milliseconds())
	}
}
