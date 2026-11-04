// Package codex uses the local Codex App Server's JSONL API. Codex owns all
// subscription credentials and token refresh; snapTrans never reads auth.json.
package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Model struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

type Status struct {
	Authenticated bool    `json:"authenticated"`
	Email         string  `json:"email"`
	Plan          string  `json:"plan"`
	Models        []Model `json:"models"`
}

// Let Codex choose its native transport and fallback for subscription access.
// Forcing a custom HTTP-only provider can stall behind local proxies.
const subscriptionProvider = "openai"

var errConnectionClosed = errors.New("Codex App Server closed unexpectedly; check the CLI path and local Codex configuration")

type message struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type sessionKey struct {
	model, instructions, effort string
}

func (key sessionKey) normalized() sessionKey {
	key.model = strings.TrimSpace(key.model)
	key.effort = strings.TrimSpace(key.effort)
	if key.effort == "default" {
		key.effort = ""
	}
	return key
}

type preparedThread struct {
	key             sessionKey
	id              string
	accountRevision uint64
}

// Connections are exclusively borrowed: auth operations use private processes,
// while inference can reuse a warm process without sharing conversations.
type connection struct {
	ctx                   context.Context
	cmd                   *exec.Cmd
	in                    io.WriteCloser
	out                   *bufio.Scanner
	seq                   int
	pending               []message
	pendingBytes          int
	dir                   string
	closeOnce             sync.Once
	processCancel         context.CancelFunc
	subscriptionCheckedAt time.Time
	accountRevision       uint64
	defaultModel          string
	prepared              *preparedThread
	prepareEnabled        bool
}

func start(ctx context.Context, path string) (*connection, error) {
	path, err := resolveExecutable(path)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "snaptrans-codex-")
	if err != nil {
		return nil, err
	}
	// This process only converts screen content to text. Inherited coding
	// plugins and synchronous lifecycle hooks add work before any model output.
	cmd := exec.CommandContext(ctx, path, "app-server", "-c", `model_provider="`+subscriptionProvider+`"`,
		"-c", "features.hooks=false", "-c", "features.plugins=false",
		"-c", "features.plugin_discovery_prefetch=false", "-c", "features.skill_search=false",
		"-c", "features.apps=false")
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	configureCommand(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		os.Remove(dir)
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		os.Remove(dir)
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		in.Close()
		out.Close()
		os.Remove(dir)
		return nil, fmt.Errorf("start Codex: %w", err)
	}
	slog.Info("Codex runtime started", "path", path)
	c := &connection{ctx: ctx, cmd: cmd, in: in, out: bufio.NewScanner(out), dir: dir}
	c.out.Buffer(make([]byte, 4096), 4<<20)
	err = c.call("initialize", map[string]any{"clientInfo": map[string]string{
		"name": "snaptrans", "title": "snapTrans", "version": "0.2.1",
	}}, nil)
	if err == nil {
		err = c.send(map[string]any{"method": "initialized", "params": map[string]any{}})
	}
	if err != nil {
		c.close()
		return nil, fmt.Errorf("initialize Codex App Server (update Codex CLI if unsupported): %w", err)
	}
	return c, nil
}

func (c *connection) close() {
	c.closeOnce.Do(func() {
		if c.processCancel != nil {
			c.processCancel()
		}
		c.in.Close()
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
		_ = os.Remove(c.dir)
	})
}

func (c *connection) send(value any) error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	err := json.NewEncoder(c.in).Encode(value)
	if err != nil {
		if c.ctx.Err() != nil {
			return c.ctx.Err()
		}
		return fmt.Errorf("%w: %w", errConnectionClosed, err)
	}
	return nil
}

func (c *connection) read() (message, error) {
	var msg message
	if err := c.ctx.Err(); err != nil {
		return msg, err
	}
	if !c.out.Scan() {
		if c.ctx.Err() != nil {
			return msg, c.ctx.Err()
		}
		if err := c.out.Err(); err != nil {
			return msg, err
		}
		return msg, errConnectionClosed
	}
	if err := c.ctx.Err(); err != nil {
		return msg, err
	}
	if err := json.Unmarshal(c.out.Bytes(), &msg); err != nil {
		return msg, fmt.Errorf("invalid Codex response: %w", err)
	}
	// This client implements no tool or approval requests.
	if msg.Method != "" && len(msg.ID) > 0 {
		if err := c.send(map[string]any{"id": msg.ID, "error": map[string]any{"code": -32601, "message": "snapTrans only accepts text output"}}); err != nil {
			return msg, err
		}
	}
	if msg.Method == "account/updated" && len(msg.ID) == 0 {
		c.accountRevision++
		c.subscriptionCheckedAt = time.Time{}
		c.defaultModel = ""
	}
	return msg, nil
}

func streamNotification(method string) bool {
	switch method {
	case "turn/started", "item/started", "item/reasoning/textDelta", "item/reasoning/summaryTextDelta", "item/reasoning/summaryPartAdded", "error", "item/agentMessage/delta", "turn/completed":
		return true
	}
	return false
}

func notificationSize(msg message) int {
	size := len(msg.ID) + len(msg.Method) + len(msg.Params) + len(msg.Result)
	if msg.Error != nil {
		size += len(msg.Error.Message)
	}
	return size
}

func (c *connection) deferNotification(msg message) error {
	// Retain only notifications consumed by stream or login. Metadata updates
	// can arrive throughout the lifetime of a reused process.
	if !streamNotification(msg.Method) && msg.Method != "account/login/completed" {
		return nil
	}
	size := notificationSize(msg)
	if len(c.pending) >= 256 || size > (4<<20)-c.pendingBytes {
		return errors.New("Codex notification buffer limit exceeded")
	}
	c.pending = append(c.pending, msg)
	c.pendingBytes += size
	return nil
}

func (c *connection) call(method string, params any, result any) error {
	started := time.Now()
	defer func() { slog.Info("Codex RPC", "method", method, "elapsed_ms", time.Since(started).Milliseconds()) }()
	c.seq++
	id := c.seq
	if err := c.send(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		msg, err := c.read()
		if err != nil {
			return fmt.Errorf("Codex %s: %w", method, err)
		}
		if string(msg.ID) != fmt.Sprint(id) || msg.Method != "" {
			if len(msg.ID) == 0 {
				if err := c.deferNotification(msg); err != nil {
					return err
				}
			}
			continue
		}
		if msg.Error != nil {
			return fmt.Errorf("Codex %s: %s", method, msg.Error.Message)
		}
		if result != nil {
			return json.Unmarshal(msg.Result, result)
		}
		return nil
	}
}

func (c *connection) next() (message, error) {
	if err := c.ctx.Err(); err != nil {
		return message{}, err
	}
	if len(c.pending) > 0 {
		msg := c.pending[0]
		c.pending[0] = message{}
		c.pending = c.pending[1:]
		c.pendingBytes -= notificationSize(msg)
		if len(c.pending) == 0 {
			c.pending = nil
			c.pendingBytes = 0
		}
		return msg, nil
	}
	return c.read()
}

func (c *connection) account() (Status, error) {
	var response struct {
		Account *struct{ Type, Email, PlanType string }
	}
	err := c.call("account/read", map[string]any{"refreshToken": false}, &response)
	// Routing discovery has its own short server deadline. A transient failure
	// can recover on the same process; keep the retry inside the caller's deadline.
	if err != nil && c.ctx.Err() == nil && (strings.Contains(err.Error(), "workspace routing discovery timed out") || strings.Contains(err.Error(), "workspace routing discovery failed")) {
		slog.Warn("Retrying Codex workspace routing discovery")
		err = c.call("account/read", map[string]any{"refreshToken": false}, &response)
	}
	status := Status{Models: []Model{}}
	if response.Account != nil {
		status.Authenticated = response.Account.Type == "chatgpt"
		if status.Authenticated {
			status.Email, status.Plan = response.Account.Email, response.Account.PlanType
		}
	}
	return status, err
}

func (c *connection) models() ([]Model, error) {
	models := []Model{}
	var cursor *string
	seen := make(map[string]bool)
	for {
		var page struct {
			Data []struct {
				ID, Model, DisplayName string
				IsDefault, Hidden      bool
			}
			NextCursor *string
		}
		if err := c.call("model/list", map[string]any{"limit": 100, "cursor": cursor}, &page); err != nil {
			return nil, err
		}
		for _, model := range page.Data {
			if model.Hidden {
				continue
			}
			id := model.Model
			if id == "" {
				id = model.ID
			}
			models = append(models, Model{ID: id, Name: model.DisplayName, IsDefault: model.IsDefault})
		}
		if page.NextCursor == nil || *page.NextCursor == "" {
			return models, nil
		}
		if seen[*page.NextCursor] {
			return nil, errors.New("Codex model list repeated its cursor")
		}
		seen[*page.NextCursor] = true
		cursor = page.NextCursor
	}
}

func Inspect(ctx context.Context, path string) (Status, error) {
	c, err := start(ctx, path)
	if err != nil {
		return Status{}, err
	}
	defer c.close()
	status, err := c.account()
	if err == nil && status.Authenticated {
		status.Models, err = c.models()
	}
	return status, err
}

func Login(ctx context.Context, path string, openURL func(string)) error {
	c, err := start(ctx, path)
	if err != nil {
		return err
	}
	defer c.close()
	var response struct{ AuthURL, LoginID string }
	if err := c.call("account/login/start", map[string]any{"type": "chatgpt"}, &response); err != nil {
		return err
	}
	parsed, err := url.Parse(response.AuthURL)
	if err != nil || parsed.Scheme != "https" || (parsed.Hostname() != "auth.openai.com" && parsed.Hostname() != "chatgpt.com") {
		return errors.New("Codex returned an invalid login URL")
	}
	openURL(response.AuthURL)
	for {
		msg, err := c.next()
		if err != nil {
			return err
		}
		if msg.Method != "account/login/completed" {
			continue
		}
		var event struct {
			LoginID string
			Success bool
			Error   *string
		}
		if err := json.Unmarshal(msg.Params, &event); err != nil {
			return err
		}
		if event.LoginID != response.LoginID {
			continue
		}
		if event.Success {
			return nil
		}
		if event.Error != nil {
			return fmt.Errorf("Codex login: %s", *event.Error)
		}
		return errors.New("Codex login was not completed")
	}
}

func Stream(ctx context.Context, path, model, instructions, input string, onToken func(string)) (string, error) {
	c, err := start(ctx, path)
	if err != nil {
		return "", err
	}
	defer c.close()
	if err := c.requireSubscription(); err != nil {
		return "", err
	}
	return c.stream(model, instructions, input, "", onToken)
}

func (c *connection) requireSubscription() error {
	if !c.subscriptionCheckedAt.IsZero() && time.Since(c.subscriptionCheckedAt) < time.Minute {
		return nil
	}
	revision := c.accountRevision
	status, err := c.account()
	if err != nil {
		return err
	}
	if !status.Authenticated {
		return errors.New("sign in to Codex with ChatGPT in model settings first; API-key login cannot use subscription access")
	}
	if revision == c.accountRevision {
		c.subscriptionCheckedAt = time.Now()
	}
	return nil
}

func (c *connection) resolveModel(model string) (string, error) {
	if model = strings.TrimSpace(model); model != "" {
		return model, nil
	}
	if c.defaultModel == "" {
		revision := c.accountRevision
		models, err := c.models()
		if err != nil {
			return "", err
		}
		for _, candidate := range models {
			if candidate.IsDefault {
				model = strings.TrimSpace(candidate.ID)
				break
			}
		}
		if model == "" {
			return "", errors.New("Codex did not return a default model; choose a model in settings")
		}
		if revision == c.accountRevision {
			c.defaultModel = model
		}
		return model, nil
	}
	return c.defaultModel, nil
}

func (c *connection) hasPreparedSession(key sessionKey) bool {
	key = key.normalized()
	if key.model == "" {
		key.model = c.defaultModel
	}
	return c.prepared != nil && c.prepared.key == key && c.prepared.accountRevision == c.accountRevision
}

// Preparing a fresh ephemeral thread starts Codex's native transport warm-up
// without sending screen content or starting a model turn.
func (c *connection) prepareSession(key sessionKey) error {
	if c.hasPreparedSession(key) {
		return nil
	}
	revision := c.accountRevision
	key = key.normalized()
	model, err := c.resolveModel(key.model)
	if err != nil {
		return err
	}
	key.model = model
	id, err := c.startThread(key.model, key.instructions, key.effort)
	if err != nil {
		return err
	}
	c.prepared = &preparedThread{key: key, id: id, accountRevision: revision}
	c.prepareEnabled = true
	return nil
}

func (c *connection) startThread(model, instructions, effort string) (string, error) {
	key := (sessionKey{model: model, instructions: instructions, effort: effort}).normalized()
	resolved, err := c.resolveModel(key.model)
	if err != nil {
		return "", err
	}
	key.model = resolved
	if prepared := c.prepared; prepared != nil {
		c.prepared = nil
		if prepared.key == key && prepared.accountRevision == c.accountRevision {
			return prepared.id, nil
		}
		if err := c.unsubscribeThread(prepared.id); err != nil {
			return "", fmt.Errorf("discard unused Codex session: %w", err)
		}
	}
	model, effort = key.model, key.effort
	// Override inherited agent capabilities. Screen content is untrusted data;
	// it must not trigger local commands, plugins, or external tools.
	var inherited struct {
		Config struct {
			MCPServers map[string]json.RawMessage `json:"mcp_servers"`
		}
	}
	if err := c.call("config/read", map[string]any{"includeLayers": false}, &inherited); err != nil {
		return "", err
	}
	servers := map[string]any{}
	for name := range inherited.Config.MCPServers {
		servers[name] = map[string]any{"enabled": false}
	}
	options := map[string]any{
		"mcp_servers": servers, "web_search": "disabled", "project_doc_max_bytes": 0,
		"features": map[string]any{"shell_tool": false, "unified_exec": false, "apps": false, "view_image": false,
			"hooks": false, "plugins": false, "plugin_discovery_prefetch": false, "skill_search": false,
			"apply_patch_freeform": false, "collab": false, "multi_agent": false, "code_mode": false},
	}
	if effort != "" && effort != "default" {
		options["model_reasoning_effort"] = effort
	}
	var thread struct{ Thread struct{ ID string } }
	if err := c.call("thread/start", map[string]any{
		"model": model, "modelProvider": subscriptionProvider, "ephemeral": true, "cwd": c.dir,
		"approvalPolicy": "never", "sandbox": "read-only", "personality": "none",
		"baseInstructions": instructions, "developerInstructions": "Only produce the requested text. Do not use tools, read files, or execute commands.", "config": options,
	}, &thread); err != nil {
		return "", err
	}
	if thread.Thread.ID == "" {
		return "", errors.New("Codex returned no thread ID")
	}
	return thread.Thread.ID, nil
}

func (c *connection) stream(model, instructions, input, effort string, onToken func(string), images ...string) (string, error) {
	return c.streamWithProgress(model, instructions, input, effort, onToken, nil, images...)
}

func (c *connection) streamWithProgress(model, instructions, input, effort string, onToken, onProgress func(string), images ...string) (string, error) {
	effort = (sessionKey{effort: effort}).normalized().effort
	model, err := c.resolveModel(model)
	if err != nil {
		return "", err
	}
	threadID, err := c.startThread(model, instructions, effort)
	if err != nil {
		return "", err
	}
	inputs := []map[string]any{{"type": "text", "text": input}}
	for _, image := range images {
		inputs = append(inputs, map[string]any{"type": "image", "url": image})
	}
	turn := map[string]any{"threadId": threadID, "model": model, "input": inputs}
	if effort != "" && effort != "default" {
		turn["effort"] = effort
	}
	// Notifications can arrive before the turn/start acknowledgement. Consume
	// both on the same loop so a delayed RPC response never hides streamed text.
	c.seq++
	requestID := c.seq
	turnStarted := time.Now()
	if err := c.send(map[string]any{"id": requestID, "method": "turn/start", "params": turn}); err != nil {
		return "", err
	}
	var output strings.Builder
	var lastUpstreamError string
	var acknowledged, completed bool
	var acknowledgementTimer *time.Timer
	defer func() {
		if acknowledgementTimer != nil {
			acknowledgementTimer.Stop()
		}
	}()
streamLoop:
	for {
		msg, err := c.next()
		if err != nil {
			if completed && !acknowledged {
				return output.String(), fmt.Errorf("Codex turn/start acknowledgement missing after completed output: %w", err)
			}
			if lastUpstreamError != "" {
				return output.String(), fmt.Errorf("Codex stream (last upstream error: %s): %w", lastUpstreamError, err)
			}
			return output.String(), err
		}
		if string(msg.ID) == fmt.Sprint(requestID) && msg.Method == "" {
			slog.Info("Codex RPC", "method", "turn/start", "elapsed_ms", time.Since(turnStarted).Milliseconds())
			if acknowledgementTimer != nil && !acknowledgementTimer.Stop() {
				return output.String(), errors.New("Codex turn/start acknowledgement timed out after completed output")
			}
			if msg.Error != nil {
				return output.String(), fmt.Errorf("Codex turn/start: %s", msg.Error.Message)
			}
			if onProgress != nil {
				onProgress("turn/start")
			}
			acknowledged = true
			if completed {
				break streamLoop
			}
			continue
		}
		if completed || len(msg.ID) > 0 || !streamNotification(msg.Method) {
			continue
		}
		var event struct {
			ThreadID, Delta string
			WillRetry       bool
			Error           *struct{ Message string }
			Item            struct{ Type string }
			Turn            struct {
				Status string
				Error  *struct{ Message string }
			}
		}
		if err := json.Unmarshal(msg.Params, &event); err != nil {
			return output.String(), err
		}
		if event.ThreadID != threadID {
			continue
		}
		// Log protocol stages only, never screen text or reasoning contents.
		if msg.Method != "item/agentMessage/delta" {
			slog.Debug("Codex stream event", "method", msg.Method, "item_type", event.Item.Type)
		}
		switch msg.Method {
		case "turn/started", "item/started", "item/reasoning/textDelta", "item/reasoning/summaryTextDelta", "item/reasoning/summaryPartAdded":
			if msg.Method == "item/started" && event.Item.Type != "reasoning" && event.Item.Type != "agentMessage" {
				continue
			}
			if onProgress != nil {
				onProgress(msg.Method)
			}
		case "error":
			if event.Error != nil && event.Error.Message != "" {
				lastUpstreamError = event.Error.Message
				slog.Warn("Codex upstream error", "message", lastUpstreamError, "will_retry", event.WillRetry)
				if !event.WillRetry {
					return output.String(), fmt.Errorf("Codex stream: %s", lastUpstreamError)
				}
			}
		case "item/agentMessage/delta":
			output.WriteString(event.Delta)
			onToken(event.Delta)
		case "turn/completed":
			if event.Turn.Error != nil {
				return output.String(), errors.New(event.Turn.Error.Message)
			}
			if event.Turn.Status != "completed" {
				return output.String(), fmt.Errorf("Codex turn %s", event.Turn.Status)
			}
			if strings.TrimSpace(output.String()) == "" {
				return "", errors.New("Codex returned no text")
			}
			// Even a completed notification can precede the RPC response. A
			// rejected turn must never become a successful cached translation.
			completed = true
			if acknowledged {
				break streamLoop
			}
			// Native RPC replies normally precede completion. Bound this unusual
			// confirmation wait so a missing reply cannot hold already shown text.
			acknowledgementTimer = time.AfterFunc(250*time.Millisecond, c.close)
		}
	}
	// Release the ephemeral conversation before returning this process to
	// the warm pool. A failed cleanup discards it without losing output.
	_ = c.unsubscribeThread(threadID)
	return output.String(), nil
}

func (c *connection) unsubscribeThread(threadID string) error {
	timer := time.AfterFunc(250*time.Millisecond, c.close)
	err := c.call("thread/unsubscribe", map[string]any{"threadId": threadID}, nil)
	if !timer.Stop() || err != nil {
		c.close()
		if err == nil {
			err = errConnectionClosed
		} else {
			err = fmt.Errorf("%w: %w", errConnectionClosed, err)
		}
	}
	return err
}
