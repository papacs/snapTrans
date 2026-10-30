package codex

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Reuse only the process and its HTTP connections. Every request still gets a
// fresh ephemeral conversation, so messages and instructions cannot carry over.
var warmProcess struct {
	sync.Mutex
	idle        *connection
	path        string
	timer       *time.Timer
	epoch       uint64
	warming     bool
	ready       chan struct{}
	warmingPath string
	cancelWarm  context.CancelFunc
}

func takeWarm(ctx context.Context, path string) (*connection, uint64, error) {
	resolved, err := resolveExecutable(path)
	if err != nil {
		return nil, 0, err
	}
	warmProcess.Lock()
	c, previous, epoch := warmProcess.idle, warmProcess.path, warmProcess.epoch
	warmProcess.idle = nil
	if warmProcess.timer != nil {
		warmProcess.timer.Stop()
	}
	warmProcess.Unlock()
	if c != nil {
		if previous == resolved {
			c.ctx = ctx
			stop := context.AfterFunc(ctx, c.close)
			err := c.requireSubscription()
			stopped := stop()
			if err == nil && stopped && ctx.Err() == nil {
				return c, epoch, nil
			}
		}
		c.close()
	}
	if ctx.Err() != nil {
		return nil, epoch, ctx.Err()
	}
	// The process lifetime outlives a single request; only an actively borrowed
	// request can cancel it. This preserves cancellation without killing reuse.
	lifetime, cancel := context.WithCancel(context.Background())
	stop := context.AfterFunc(ctx, cancel)
	c, err = start(lifetime, resolved)
	stopped := stop()
	if err != nil || !stopped || ctx.Err() != nil {
		cancel()
		if c != nil {
			c.close()
		}
		if ctx.Err() != nil && !errors.Is(err, ctx.Err()) {
			err = ctx.Err()
		}
		return nil, epoch, err
	}
	c.processCancel = cancel
	c.ctx = ctx
	return c, epoch, nil
}

func returnWarm(c *connection, path string, epoch uint64) {
	if c.cmd.ProcessState != nil {
		c.close()
		return
	}
	warmProcess.Lock()
	if warmProcess.idle != nil || warmProcess.epoch != epoch {
		warmProcess.Unlock()
		c.close()
		return
	}
	warmProcess.idle, warmProcess.path = c, path
	warmProcess.timer = time.AfterFunc(5*time.Minute, func() {
		warmProcess.Lock()
		if warmProcess.idle != c {
			warmProcess.Unlock()
			return
		}
		warmProcess.idle = nil
		warmProcess.Unlock()
		c.close()
	})
	warmProcess.Unlock()
}

// CloseIdle also invalidates borrowed connections so shutdown or a model
// settings change cannot put an obsolete process back into the pool.
func CloseIdle() {
	warmProcess.Lock()
	c := warmProcess.idle
	warmProcess.idle = nil
	warmProcess.epoch++
	cancel := warmProcess.cancelWarm
	if warmProcess.timer != nil {
		warmProcess.timer.Stop()
	}
	warmProcess.Unlock()
	if cancel != nil {
		cancel()
	}
	if c != nil {
		c.close()
	}
}

// Warm prepares only the local process. Network setup must not be discarded
// when a foreground request takes over shortly after selection starts.
func Warm(ctx context.Context, path string, _ ...string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	warmProcess.Lock()
	if warmProcess.idle != nil || warmProcess.warming {
		warmProcess.Unlock()
		return nil
	}
	warmProcess.warming = true
	warmProcess.cancelWarm = cancel
	warmProcess.warmingPath = path
	warmProcess.ready = make(chan struct{})
	warmProcess.Unlock()
	defer func() {
		warmProcess.Lock()
		warmProcess.warming = false
		warmProcess.cancelWarm = nil
		close(warmProcess.ready)
		warmProcess.Unlock()
	}()
	c, epoch, err := takeWarm(ctx, path)
	if err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, c.close)
	if !stop() || ctx.Err() != nil || err != nil {
		c.close()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	returnWarm(c, c.cmd.Path, epoch)
	return nil
}

func StreamFast(ctx context.Context, path, model, instructions, input, effort string, onToken func(string), images ...string) (string, error) {
	return streamFast(ctx, path, model, instructions, input, effort, 20*time.Second, onToken, images...)
}

var errFirstOutputTimeout = errors.New("Codex model connection was inactive before returning text")

func streamFast(ctx context.Context, path, model, instructions, input, effort string, firstOutputTimeout time.Duration, onToken func(string), images ...string) (string, error) {
	for attempt := 0; ; attempt++ {
		attemptCtx, cancel := context.WithCancelCause(ctx)
		output, err := streamFastAttempt(attemptCtx, cancel, path, model, instructions, input, effort, firstOutputTimeout, onToken, images...)
		cause := context.Cause(attemptCtx)
		cancel(nil)
		if errors.Is(cause, errFirstOutputTimeout) {
			err = errFirstOutputTimeout
			// Reconnect only before any text was emitted. Retrying partial output
			// would duplicate text; user cancellation and the overall deadline win.
			if attempt == 0 && output == "" && ctx.Err() == nil {
				slog.Warn("Reconnecting inactive Codex model connection", "idle_timeout_ms", firstOutputTimeout.Milliseconds())
				continue
			}
		}
		if ctx.Err() != nil && !errors.Is(err, ctx.Err()) {
			err = ctx.Err()
		}
		return output, err
	}
}

func streamFastAttempt(ctx context.Context, cancel context.CancelCauseFunc, path, model, instructions, input, effort string, firstOutputTimeout time.Duration, onToken func(string), images ...string) (string, error) {
	warmProcess.Lock()
	ready := warmProcess.ready
	cancelWarm := warmProcess.cancelWarm
	waiting := warmProcess.warming && warmProcess.idle == nil && warmProcess.warmingPath == path
	warmProcess.Unlock()
	if waiting {
		started := time.Now()
		// Prewarming is optional. Never spend the translation's timeout waiting
		// for a background handshake that may itself be stuck on the network.
		timer := time.NewTimer(150 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ready:
		case <-timer.C:
			if cancelWarm != nil {
				cancelWarm()
			}
		case <-ctx.Done():
			return "", ctx.Err()
		}
		slog.Info("Codex prewarm wait", "elapsed_ms", time.Since(started).Milliseconds())
	}
	connectionStarted := time.Now()
	c, epoch, err := takeWarm(ctx, path)
	slog.Info("Codex connection setup", "elapsed_ms", time.Since(connectionStarted).Milliseconds(), "success", err == nil)
	if err != nil {
		return "", err
	}
	stop := context.AfterFunc(ctx, c.close)
	if err = c.requireSubscription(); err != nil {
		stop()
		c.close()
		return "", err
	}
	// Begin the idle deadline only after the model turn is accepted. Session
	// setup and active reasoning must never consume a first-output deadline.
	var firstOutputTimer *time.Timer
	receivedText := false
	output, err := c.streamWithProgress(model, instructions, input, effort, func(token string) {
		if firstOutputTimer != nil && token != "" {
			firstOutputTimer.Stop()
		}
		if token != "" {
			receivedText = true
		}
		onToken(token)
	}, func(string) {
		if receivedText || (effort != "low" && effort != "none") {
			return
		}
		if firstOutputTimer == nil {
			firstOutputTimer = time.AfterFunc(firstOutputTimeout, func() { cancel(errFirstOutputTimeout) })
		} else {
			firstOutputTimer.Reset(firstOutputTimeout)
		}
	}, images...)
	if firstOutputTimer != nil {
		firstOutputTimer.Stop()
	}
	stopped := stop()
	if err != nil || !stopped || ctx.Err() != nil {
		c.close()
		if ctx.Err() != nil && !errors.Is(err, ctx.Err()) {
			err = ctx.Err()
		}
		return output, err
	}
	returnWarm(c, c.cmd.Path, epoch)
	return output, nil
}
