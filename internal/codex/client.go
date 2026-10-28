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

// Use Codex-managed subscription auth with HTTPS streaming. The built-in
// provider retries WebSockets before falling back, which can add tens of
// seconds on networks that do not carry those connections reliably.
const subscriptionProvider = "snaptrans_https"

type message struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Connections are exclusively borrowed: auth operations use private processes,
// while inference can reuse a warm process without sharing conversations.
type connection struct {
	ctx           context.Context
	cmd           *exec.Cmd
	in            io.WriteCloser
	out           *bufio.Scanner
	seq           int
	pending       []message
	dir           string
	closeOnce     sync.Once
	processCancel context.CancelFunc
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
	cmd := exec.CommandContext(ctx, path, "app-server", "-c", `model_provider="`+subscriptionProvider+`"`, "-c",
		`model_providers.`+subscriptionProvider+`={name="OpenAI",wire_api="responses",requires_openai_auth=true,supports_websockets=false}`)
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

func (c *connection) send(value any) error { return json.NewEncoder(c.in).Encode(value) }

func (c *connection) read() (message, error) {
	var msg message
	if !c.out.Scan() {
		if c.ctx.Err() != nil {
			return msg, c.ctx.Err()
		}
		if err := c.out.Err(); err != nil {
			return msg, err
		}
		return msg, errors.New("Codex App Server closed unexpectedly; check the CLI path and local Codex configuration")
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
	return msg, nil
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
			return err
		}
		if string(msg.ID) != fmt.Sprint(id) || msg.Method != "" {
			if len(msg.ID) == 0 {
				c.pending = append(c.pending, msg)
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
	if len(c.pending) > 0 {
		msg := c.pending[0]
		c.pending = c.pending[1:]
		return msg, nil
	}
	return c.read()
}

func (c *connection) account() (Status, error) {
	var response struct {
		Account *struct{ Type, Email, PlanType string }
	}
	err := c.call("account/read", map[string]any{"refreshToken": false}, &response)
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
		if cursor != nil && *cursor == *page.NextCursor {
			return nil, errors.New("Codex model list repeated its cursor")
		}
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
	status, err := c.account()
	if err != nil {
		return err
	}
	if !status.Authenticated {
		return errors.New("sign in to Codex with ChatGPT in model settings first; API-key login cannot use subscription access")
	}
	return nil
}

func (c *connection) startThread(model, instructions string) (string, error) {
	if model == "" {
		models, err := c.models()
		if err != nil {
			return "", err
		}
		for _, candidate := range models {
			if candidate.IsDefault {
				model = candidate.ID
				break
			}
		}
		if model == "" {
			return "", errors.New("Codex did not return a default model; choose a model in settings")
		}
	}
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
		"features": map[string]any{"shell_tool": false, "unified_exec": false, "apps": false,
			"apply_patch_freeform": false, "include_apply_patch_tool": false, "collab": false, "multi_agent": false, "code_mode": false},
		"tools": map[string]any{"view_image": false},
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
	threadID, err := c.startThread(model, instructions)
	if err != nil {
		return "", err
	}
	inputs := []map[string]any{{"type": "text", "text": input}}
	for _, image := range images {
		inputs = append(inputs, map[string]any{"type": "image", "url": image})
	}
	turn := map[string]any{"threadId": threadID, "input": inputs}
	if effort != "" && effort != "default" {
		turn["effort"] = effort
	}
	if err := c.call("turn/start", turn, nil); err != nil {
		return "", err
	}
	var output strings.Builder
	for {
		msg, err := c.next()
		if err != nil {
			return output.String(), err
		}
		if msg.Method == "error" || msg.Method == "item/started" || msg.Method == "turn/started" {
			slog.Debug("Codex stream event", "method", msg.Method)
		}
		var event struct {
			ThreadID, Delta string
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
		switch msg.Method {
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
			// Release the ephemeral conversation before returning this process to
			// the warm pool. A failed cleanup discards it without losing output.
			if err := c.call("thread/unsubscribe", map[string]any{"threadId": threadID}, nil); err != nil {
				c.close()
			}
			return output.String(), nil
		}
	}
}
