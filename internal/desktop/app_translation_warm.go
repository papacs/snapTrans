package desktop

import (
	"context"
	"strings"
	"time"

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

func (a *App) prewarmTranslationConnection() {
	a.mu.Lock()
	cfg := a.cfg.WithDefaults()
	a.mu.Unlock()
	if strings.TrimSpace(cfg.APIKey) == "" {
		return
	}

	target := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	a.translationWarmMu.Lock()
	started := a.translationWarm.begin(target, time.Now())
	a.translationWarmMu.Unlock()
	if !started {
		return
	}

	go a.runTranslationWarmup(cfg, target)
}

func (a *App) runTranslationWarmup(cfg config.Config, target string) {
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), translationWarmTimeout)
	defer cancel()
	client := translator.NewOpenAICompatible(translator.Options{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model})
	err := client.Warm(ctx)

	a.translationWarmMu.Lock()
	a.translationWarm.finish(target, time.Now(), err == nil)
	a.translationWarmMu.Unlock()
	if err == nil && a.log != nil {
		a.log.Infof("translation connection warmed warm_ms=%d", time.Since(startedAt).Milliseconds())
	}
}
