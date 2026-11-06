package translator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"snaptrans/internal/codex"
	"snaptrans/internal/config"
)

type runtimeAPIStage struct {
	Stage     string `json:"stage"`
	Phase     string `json:"phase"`
	Method    string `json:"method,omitempty"`
	ItemType  string `json:"itemType,omitempty"`
	ElapsedMS int64  `json:"elapsedMs"`
	AtMS      int64  `json:"atMs"`
	ProcessID int64  `json:"processId,omitempty"`
}

type runtimeAPIRecorder struct {
	sync.Mutex
	started time.Time
	phase   string
	stages  []runtimeAPIStage
}

func (*runtimeAPIRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *runtimeAPIRecorder) WithAttrs([]slog.Attr) slog.Handler     { return r }
func (r *runtimeAPIRecorder) WithGroup(string) slog.Handler          { return r }
func (r *runtimeAPIRecorder) Handle(_ context.Context, record slog.Record) error {
	switch record.Message {
	case "Codex RPC", "Codex runtime started", "Codex connection setup", "Codex prewarm wait", "Codex notification", "Codex stream event", "Translation cache hit", "Translation repairing missing regions", "Reconnecting Codex model connection", "Codex upstream error":
	default:
		return nil
	}
	r.Lock()
	defer r.Unlock()
	stage := runtimeAPIStage{Stage: record.Message, Phase: r.phase, AtMS: time.Since(r.started).Milliseconds()}
	record.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case "method":
			stage.Method = attr.Value.String()
		case "item_type":
			stage.ItemType = attr.Value.String()
		case "elapsed_ms":
			stage.ElapsedMS = attr.Value.Int64()
		case "process_id":
			stage.ProcessID = attr.Value.Int64()
		}
		return true
	})
	r.stages = append(r.stages, stage)
	return nil
}

func (r *runtimeAPIRecorder) setPhase(phase string) {
	r.Lock()
	r.phase = phase
	r.Unlock()
}

func runtimeAPIErrorClass(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	text := strings.ToLower(err.Error())
	for _, match := range []struct{ text, class string }{
		{"routing discovery", "routing-discovery"}, {"inactive before returning text", "first-output-timeout"},
		{"sign in", "authentication"}, {"unauthorized", "authentication"}, {"401", "authentication"},
		{"429", "rate-limit"}, {"model", "model-or-upstream"}, {"incomplete", "incomplete-output"},
		{"closed unexpectedly", "connection-closed"}, {"eof", "eof"}, {"connect", "connection"},
	} {
		if strings.Contains(text, match.text) {
			return match.class
		}
	}
	return "other"
}

type runtimeAPICaseResult struct {
	Name         string            `json:"name"`
	Direction    Direction         `json:"direction"`
	StartedAt    string            `json:"startedAt"`
	IdleMS       int64             `json:"idleMs,omitempty"`
	PrewarmMS    int64             `json:"prewarmMs,omitempty"`
	PrewarmError string            `json:"prewarmError,omitempty"`
	FirstTokenMS int64             `json:"firstTokenMs"`
	TotalMS      int64             `json:"totalMs"`
	OutputChars  int               `json:"outputChars"`
	TokenChunks  int               `json:"tokenChunks"`
	ErrorClass   string            `json:"errorClass"`
	Stages       []runtimeAPIStage `json:"stages"`
}

type runtimeAPIRun struct {
	Profile         string                 `json:"profile,omitempty"`
	ID              string                 `json:"id"`
	StartedAt       string                 `json:"startedAt"`
	Provider        string                 `json:"provider"`
	Model           string                 `json:"model"`
	ReasoningEffort string                 `json:"reasoningEffort"`
	FastTranslation bool                   `json:"fastTranslation"`
	Platform        string                 `json:"platform"`
	RequestTimeoutS int                    `json:"requestTimeoutSeconds"`
	Cases           []runtimeAPICaseResult `json:"cases"`
}

// Opt in only with explicit authorization: real configured-provider requests,
// synthetic text only, no settings writes and no credentials/output in reports.
func TestLocalConfiguredRuntimeAPISoak(t *testing.T) {
	if os.Getenv("SNAPTRANS_RUNTIME_API_SOAK") != "1" {
		t.Skip("optional real configured-provider streaming measurements")
	}
	store, err := config.NewStore("snapTrans")
	if err != nil {
		t.Fatal("configuration store unavailable")
	}
	cfg, err := store.Load()
	if err != nil {
		t.Fatal("configuration load failed")
	}
	options := Options{Provider: cfg.Provider, APIKey: cfg.APIKey, BaseURL: cfg.BaseURL,
		Model: cfg.Model, CodexPath: cfg.CodexPath, CodexReasoningEffort: cfg.CodexReasoningEffort,
		FastTranslation: cfg.FastTranslation, SystemPrompt: cfg.SystemPrompt, Glossary: cfg.Glossary, Source: "ocr"}
	if cfg.Provider == "codex" {
		options.Model = cfg.CodexModel
	}
	client := NewOpenAICompatible(options)
	if cfg.Provider != "codex" && cfg.APIKey == "" {
		t.Fatal("configured API has no key")
	}
	requestTimeout := time.Duration(cfg.TranslationTimeoutSeconds) * time.Second
	if requestTimeout <= 0 || requestTimeout > time.Minute {
		requestTimeout = time.Minute
	}
	started := time.Now()
	run := runtimeAPIRun{ID: started.UTC().Format("20060102T150405.000000000Z"), StartedAt: started.UTC().Format(time.RFC3339Nano),
		Profile:  os.Getenv("SNAPTRANS_RUNTIME_API_PROFILE"),
		Provider: cfg.Provider, Model: client.options.Model, ReasoningEffort: client.forTranslation().options.CodexReasoningEffort,
		FastTranslation: cfg.FastTranslation, Platform: runtime.GOOS + "/" + runtime.GOARCH, RequestTimeoutS: int(requestTimeout.Seconds())}
	reportPath := os.Getenv("SNAPTRANS_RUNTIME_API_REPORT")
	if reportPath == "" {
		reportPath = filepath.Join("..", "..", "docs", "benchmarks", "runtime-api-2026-11-06.json")
	}
	var report struct {
		Date   string          `json:"date"`
		Method string          `json:"method"`
		Runs   []runtimeAPIRun `json:"runs"`
	}
	if raw, err := os.ReadFile(reportPath); err == nil && json.Unmarshal(raw, &report) != nil {
		t.Fatal("existing runtime report is invalid")
	}
	report.Date = "2026-11-06"
	report.Method = "Real configured-provider production text translation; unique synthetic inputs; no screenshot data, credentials, source text or full model outputs retained. Stage attribution is by timestamp and process ID; background refill can overlap the next sample."
	report.Runs = append(report.Runs, run)
	save := func() {
		report.Runs[len(report.Runs)-1] = run
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil || os.MkdirAll(filepath.Dir(reportPath), 0755) != nil || os.WriteFile(reportPath, append(raw, '\n'), 0600) != nil {
			t.Fatal("runtime report write failed")
		}
	}
	save()
	previousLogger := slog.Default()
	recorder := &runtimeAPIRecorder{started: started}
	slog.SetDefault(slog.New(recorder))
	defer slog.SetDefault(previousLogger)
	defer codex.CloseIdle()
	cases := []struct {
		name      string
		direction Direction
		cold      bool
		prewarm   bool
		idle      time.Duration
	}{
		{"cold-zh", DirectionToChinese, true, false, 0},
		{"reused-zh", DirectionToChinese, false, false, 0},
		{"switch-en", DirectionToEnglish, false, false, 0},
		{"reused-en", DirectionToEnglish, false, false, 0},
		{"prewarmed-zh", DirectionToChinese, true, true, 0},
		{"prepared-reused-zh", DirectionToChinese, false, false, 0},
		{"prepared-switch-en", DirectionToEnglish, false, false, 0},
		{"idle-reused-en", DirectionToEnglish, false, false, 30 * time.Second},
		{"return-zh", DirectionToChinese, false, false, 0},
		{"prewarmed-en", DirectionToEnglish, true, true, 0},
	}
	filter := os.Getenv("SNAPTRANS_RUNTIME_API_CASES")
	for i, sample := range cases {
		if filter != "" && !strings.Contains(","+filter+",", ","+sample.name+",") {
			continue
		}
		recorder.Lock()
		recorder.started, recorder.stages, recorder.phase = time.Now(), nil, "prepare"
		recorder.Unlock()
		result := runtimeAPICaseResult{Name: sample.name, Direction: sample.direction, StartedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if sample.cold {
			codex.CloseIdle()
			if transport, ok := http.DefaultTransport.(*http.Transport); ok {
				transport.CloseIdleConnections()
			}
		}
		if sample.idle > 0 {
			recorder.setPhase("idle")
			time.Sleep(sample.idle)
			result.IdleMS = sample.idle.Milliseconds()
		}
		if sample.prewarm {
			recorder.setPhase("prewarm")
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			warmStarted := time.Now()
			warmErr := client.WarmTranslation(ctx, sample.direction)
			result.PrewarmMS, result.PrewarmError = time.Since(warmStarted).Milliseconds(), runtimeAPIErrorClass(warmErr)
			cancel()
		}
		recorder.setPhase("request")
		requestStarted := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		nonce := fmt.Sprintf("%d-%02d", started.UnixNano(), i+1)
		source := "Please confirm that synthetic test item " + nonce + " will finish this afternoon."
		if sample.direction == DirectionToEnglish {
			source = "请确认编号为 " + nonce + " 的测试任务将在今天下午完成。"
		}
		err := client.Translate(ctx, source, sample.direction, func(token string) {
			if result.FirstTokenMS == 0 && strings.TrimSpace(token) != "" {
				result.FirstTokenMS = time.Since(requestStarted).Milliseconds()
			}
			result.TokenChunks++
			result.OutputChars += utf8.RuneCountInString(token)
		})
		result.TotalMS, result.ErrorClass = time.Since(requestStarted).Milliseconds(), runtimeAPIErrorClass(err)
		cancel()
		recorder.Lock()
		result.Stages = append([]runtimeAPIStage(nil), recorder.stages...)
		recorder.Unlock()
		run.Cases = append(run.Cases, result)
		save()
		t.Logf("runtime case=%s direction=%s first_token_ms=%d total_ms=%d chars=%d error_class=%s", sample.name, sample.direction, result.FirstTokenMS, result.TotalMS, result.OutputChars, result.ErrorClass)
		if err != nil || result.OutputChars == 0 {
			t.Errorf("runtime case %s failed: %s", sample.name, result.ErrorClass)
		}
	}
}
