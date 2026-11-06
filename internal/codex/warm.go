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
	idle            *connection
	path            string
	timer           *time.Timer
	epoch           uint64
	warming         bool
	ready           chan struct{}
	sessionStarting chan struct{}
	warmingPath     string
	cancelWarm      context.CancelFunc
	active          int
}

func takeWarm(ctx context.Context, path string, expectedEpoch ...uint64) (*connection, uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	resolved, err := resolveExecutable(path)
	if err != nil {
		return nil, 0, err
	}
	warmProcess.Lock()
	if len(expectedEpoch) > 0 && warmProcess.epoch != expectedEpoch[0] {
		warmProcess.Unlock()
		return nil, expectedEpoch[0], context.Canceled
	}
	c, previous, epoch := warmProcess.idle, warmProcess.path, warmProcess.epoch
	warmProcess.idle = nil
	if warmProcess.timer != nil {
		warmProcess.timer.Stop()
		warmProcess.timer = nil
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
	var timer *time.Timer
	timer = time.AfterFunc(5*time.Minute, func() { expireWarm(c, timer) })
	warmProcess.timer = timer
	warmProcess.Unlock()
}

func expireWarm(c *connection, timer *time.Timer) {
	warmProcess.Lock()
	// Stop cannot recall an AfterFunc callback already queued before checkout.
	// The same process can be returned with a new idle timer before it runs.
	if warmProcess.idle != c || warmProcess.timer != timer {
		warmProcess.Unlock()
		return
	}
	warmProcess.idle = nil
	warmProcess.timer = nil
	warmProcess.Unlock()
	c.close()
}

// CloseIdle also invalidates borrowed connections so shutdown or a model
// settings change cannot put an obsolete process back into the pool.
func CloseIdle() {
	warmProcess.Lock()
	c := warmProcess.idle
	warmProcess.idle = nil
	warmProcess.epoch++
	cancel := warmProcess.cancelWarm
	warmProcess.warming = false
	warmProcess.cancelWarm = nil
	warmProcess.ready = nil
	warmProcess.sessionStarting = nil
	if warmProcess.timer != nil {
		warmProcess.timer.Stop()
		warmProcess.timer = nil
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
	return warm(ctx, path, nil, nil)
}

// WarmSession opportunistically prepares one unused ephemeral conversation.
// An existing unused conversation is preserved until a real request can match
// it. Codex can preconnect without receiving any screen input.
func WarmSession(ctx context.Context, path, model, instructions, effort string) error {
	return warm(ctx, path, &sessionKey{model: model, instructions: instructions, effort: effort}, nil)
}

// WarmSessionRequested prepares a known upcoming task, replacing an unused
// session with a different key. Unlike speculative warming, it can wait for an
// in-flight process warm-up inside the caller's background deadline.
func WarmSessionRequested(ctx context.Context, path, model, instructions, effort string) error {
	warmProcess.Lock()
	epoch := warmProcess.epoch
	warmProcess.Unlock()
	return warmWithPolicy(ctx, path, &sessionKey{model: model, instructions: instructions, effort: effort}, &epoch, true)
}

type sessionRefillContextKey struct{}

type sessionRefill struct {
	path  string
	key   sessionKey
	epoch uint64
}

type sessionRefillScope struct {
	sync.Mutex
	finished  bool
	succeeded bool
	next      *sessionRefill
}

// DeferSessionRefill keeps consecutive model calls in one logical request from
// racing background session preparation. Call finish after the last call, before
// canceling ctx. The first successful session key prepares the next request;
// repair instructions and input never become conversation history.
func DeferSessionRefill(ctx context.Context) (context.Context, func()) {
	if ctx.Value(sessionRefillContextKey{}) != nil {
		return ctx, func() {}
	}
	scope := &sessionRefillScope{}
	warmProcess.Lock()
	warmProcess.active++
	warmProcess.Unlock()
	return context.WithValue(ctx, sessionRefillContextKey{}, scope), func() {
		scope.Lock()
		if scope.finished {
			scope.Unlock()
			return
		}
		scope.finished = true
		next, succeeded := scope.next, scope.succeeded
		scope.Unlock()
		warmProcess.Lock()
		warmProcess.active--
		warmProcess.Unlock()
		if succeeded && next != nil && ctx.Err() == nil {
			refillSession(*next)
		}
	}
}

func (s *sessionRefillScope) record(next sessionRefill) {
	s.Lock()
	defer s.Unlock()
	if s.finished {
		return
	}
	if s.next == nil || s.next.path != next.path || s.next.epoch != next.epoch {
		s.next = &next
	}
	s.succeeded = true
}

func refillSession(next sessionRefill) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := warm(ctx, next.path, &next.key, &next.epoch); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("Codex next session preparation failed", "error", err)
		}
	}()
}

func warm(ctx context.Context, path string, key *sessionKey, expectedEpoch *uint64) error {
	return warmWithPolicy(ctx, path, key, expectedEpoch, false)
}

func warmWithPolicy(ctx context.Context, path string, key *sessionKey, expectedEpoch *uint64, requested bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		warmProcess.Lock()
		if warmProcess.active > 0 || (expectedEpoch != nil && (warmProcess.epoch != *expectedEpoch || (!requested && warmProcess.idle == nil))) {
			warmProcess.Unlock()
			return nil
		}
		if !warmProcess.warming {
			break
		}
		ready := warmProcess.ready
		wait := requested && warmProcess.warmingPath == path
		warmProcess.Unlock()
		if !wait {
			return nil
		}
		select {
		case <-ready:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if warmProcess.idle != nil && (key == nil || (!requested && warmProcess.idle.prepared != nil) || (requested && warmProcess.idle.hasPreparedSession(*key))) {
		warmProcess.Unlock()
		return nil
	}
	warmProcess.warming = true
	warmProcess.cancelWarm = cancel
	warmProcess.warmingPath = path
	admittedEpoch := warmProcess.epoch
	ready := make(chan struct{})
	warmProcess.ready = ready
	sessionStarting := make(chan struct{})
	warmProcess.sessionStarting = sessionStarting
	warmProcess.Unlock()
	defer func() {
		warmProcess.Lock()
		if warmProcess.ready == ready {
			warmProcess.warming = false
			warmProcess.cancelWarm = nil
		}
		close(ready)
		warmProcess.Unlock()
	}()
	c, epoch, err := takeWarm(ctx, path, admittedEpoch)
	if err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, c.close)
	if key != nil {
		if err = c.requireSubscription(); err == nil {
			// Foreground work can briefly wait for this last setup stage rather
			// than discarding the process and repeating subscription discovery.
			close(sessionStarting)
			err = c.prepareSession(*key)
		}
	}
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
	if scope, ok := ctx.Value(sessionRefillContextKey{}).(*sessionRefillScope); ok {
		// A later failed or canceled pass must not refill from an earlier success.
		scope.Lock()
		scope.succeeded = false
		scope.Unlock()
	}
	for attempt := 0; ; attempt++ {
		attemptCtx, cancel := context.WithCancelCause(ctx)
		output, err := streamFastAttempt(attemptCtx, cancel, path, model, instructions, input, effort, firstOutputTimeout, onToken, images...)
		cause := context.Cause(attemptCtx)
		cancel(nil)
		if errors.Is(cause, errFirstOutputTimeout) {
			err = errFirstOutputTimeout
		}
		// An idle child may have exited since its last successful request. Only
		// transport failures before any text are safe to resend; auth/config
		// errors, partial output, cancellation and the overall deadline win.
		if attempt == 0 && output == "" && ctx.Err() == nil && (errors.Is(err, errFirstOutputTimeout) || errors.Is(err, errConnectionClosed)) {
			slog.Warn("Reconnecting Codex model connection", "idle_timeout_ms", firstOutputTimeout.Milliseconds(), "reason", err)
			continue
		}
		if ctx.Err() != nil && !errors.Is(err, ctx.Err()) {
			err = ctx.Err()
		}
		return output, err
	}
}

func streamFastAttempt(ctx context.Context, cancel context.CancelCauseFunc, path, model, instructions, input, effort string, firstOutputTimeout time.Duration, onToken func(string), images ...string) (string, error) {
	warmProcess.Lock()
	warmProcess.active++
	ready := warmProcess.ready
	sessionStarting := warmProcess.sessionStarting
	cancelWarm := warmProcess.cancelWarm
	waiting := warmProcess.warming && warmProcess.warmingPath == path
	warmProcess.Unlock()
	var refillEpoch uint64
	refill := false
	defer func() {
		warmProcess.Lock()
		warmProcess.active--
		warmProcess.Unlock()
		if refill {
			next := sessionRefill{path: path, key: sessionKey{model: model, instructions: instructions, effort: effort}, epoch: refillEpoch}
			if scope, ok := ctx.Value(sessionRefillContextKey{}).(*sessionRefillScope); ok {
				scope.record(next)
			} else {
				refillSession(next)
			}
		}
	}()
	if waiting {
		started := time.Now()
		// Prewarming is optional. Never spend the translation's timeout waiting
		// for a background handshake that may itself be stuck on the network.
		timer := time.NewTimer(150 * time.Millisecond)
		defer timer.Stop()
	waitForWarm:
		for {
			select {
			case <-ready:
				break waitForWarm
			case <-sessionStarting:
				sessionStarting = nil
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				// The authenticated runtime is already usable. Bound only the
				// remaining unused-thread setup, including a mismatched prompt.
				timer.Reset(time.Second)
			case <-timer.C:
				if cancelWarm != nil {
					cancelWarm()
				}
				break waitForWarm
			case <-ctx.Done():
				return "", ctx.Err()
			}
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
	// Turn acceptance precedes local input/skill preparation. The response
	// idle clock starts only when input is ready (or assistant activity begins);
	// the overall caller deadline still bounds setup and the entire request.
	var firstOutputTimer *time.Timer
	receivedText := false
	inputReady := false
	output, err := c.streamWithProgress(model, instructions, input, effort, func(token string) {
		if firstOutputTimer != nil && token != "" {
			firstOutputTimer.Stop()
		}
		if token != "" {
			receivedText = true
		}
		onToken(token)
	}, func(stage string) {
		if receivedText || (effort != "low" && effort != "none") {
			return
		}
		if stage == "turn/start" || stage == "turn/started" || (stage == progressInputReady && inputReady) {
			return
		}
		inputReady = true
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
	// A successful cold request must also recover preparation for the next
	// capture, using its actual prompt rather than another speculative guess.
	refill, refillEpoch = true, epoch
	returnWarm(c, c.cmd.Path, epoch)
	return output, nil
}
