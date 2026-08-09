package ocr

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Worker keeps a single RapidOCR-json process alive and feeds it base64
// images through stdin. RapidOCR-json v0.2.0 runs in an infinite loop when
// started without an image path: it prints "OCR init completed." and then
// reads one JSON command per line, answering each with one JSON result.
// Keeping the process alive avoids the per-selection cold start.
type Worker struct {
	executable string
	timeout    time.Duration

	mu           sync.Mutex
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       io.Reader
	processGuard io.Closer
	ready        bool
	closed       bool

	// RapidOCR exposes a line-oriented request/response protocol over one
	// stdin/stdout pair. These context-aware gates prevent two callers from
	// consuming each other's response and avoid the goroutine leak caused by
	// waiting on a sync.Mutex after the caller has already cancelled.
	startGate chan struct{}
	runGate   chan struct{}
}

func NewRapidOCRWorker(path string, timeout time.Duration) *Worker {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Worker{
		executable: path,
		timeout:    timeout,
		startGate:  newWorkerGate(),
		runGate:    newWorkerGate(),
	}
}

func newWorkerGate() chan struct{} {
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return gate
}

func acquireWorkerGate(ctx context.Context, gate chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-gate:
		return nil
	}
}

func releaseWorkerGate(gate chan struct{}) {
	gate <- struct{}{}
}

func (w *Worker) acquireStart(ctx context.Context) error {
	return acquireWorkerGate(ctx, w.startGate)
}

func (w *Worker) releaseStart() {
	releaseWorkerGate(w.startGate)
}

func (w *Worker) acquireRun(ctx context.Context) error {
	return acquireWorkerGate(ctx, w.runGate)
}

func (w *Worker) releaseRun() {
	releaseWorkerGate(w.runGate)
}

// SetExecutable updates the configured OCR executable path. The change
// takes effect on the next (re)start.
func (w *Worker) SetExecutable(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.executable = path
}

// Start launches the OCR process and blocks until it has initialized. It
// is safe to call concurrently: concurrent callers wait for the in-flight
// start and return once the worker is ready.
func (w *Worker) Start(ctx context.Context) error {
	if err := w.acquireStart(ctx); err != nil {
		return err
	}
	defer w.releaseStart()

	w.mu.Lock()
	closed := w.closed
	alreadyReady := w.ready
	w.mu.Unlock()
	if closed {
		return errors.New("ocr worker is closed")
	}
	if alreadyReady {
		return nil
	}
	return w.startProcess(ctx)
}

func (w *Worker) startProcess(ctx context.Context) error {
	w.mu.Lock()
	w.closeProcessLocked()
	configuredPath := w.executable
	w.mu.Unlock()

	cwd, _ := os.Getwd()
	executable, _ := os.Executable()
	resolved, err := ResolveExecutablePath(configuredPath, cwd, executable)
	if err != nil {
		return err
	}

	// Startup cancellation is handled explicitly below. The child must not be
	// tied to the short-lived warm-up context after it reports ready.
	cmd := newRapidOCRWorkerCommand(context.Background(), resolved)
	cmd.Dir = filepath.Dir(resolved)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("ocr worker: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ocr worker: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("ocr worker: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ocr worker: start: %w", err)
	}
	guard, err := guardOCRProcess(cmd.Process)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("ocr worker: guard process: %w", err)
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		_ = guard.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return errors.New("ocr worker is closed")
	}
	w.cmd = cmd
	w.stdin = stdin
	w.stdout = stdout
	w.processGuard = guard
	w.mu.Unlock()
	go func() { _, _ = io.Copy(io.Discard, stderr) }()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	initDone := make(chan struct{})
	initErr := make(chan error, 1)
	go func() {
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if strings.Contains(strings.ToLower(line), "init completed") {
				close(initDone)
				return
			}
		}
		if err := scanner.Err(); err != nil {
			initErr <- err
			return
		}
		initErr <- errors.New("ocr worker: process exited during initialization")
	}()

	select {
	case <-ctx.Done():
		w.stopProcess()
		return ctx.Err()
	case err := <-initErr:
		w.stopProcess()
		return err
	case <-initDone:
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.cmd != cmd {
		return errors.New("ocr worker stopped during initialization")
	}
	w.ready = true
	return nil
}

func newRapidOCRWorkerCommand(ctx context.Context, resolvedExecutable string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, resolvedExecutable, "--ensureAscii=1")
	configureOCRCommand(cmd)
	return cmd
}

// Run submits the image data URL and returns the parsed result. It waits
// for the worker to be ready first, so the first request may also perform
// the initial process warm-up.
func (w *Worker) Run(ctx context.Context, imageDataURL string) (Result, error) {
	runCtx, cancel := w.runContext(ctx)
	defer cancel()
	if err := w.acquireRun(runCtx); err != nil {
		return Result{}, err
	}
	restartAfterRun := false
	defer func() {
		w.releaseRun()
		if restartAfterRun {
			go w.warmAfterFailure()
		}
	}()

	if err := w.ensureReady(runCtx); err != nil {
		return Result{}, err
	}

	w.mu.Lock()
	if !w.ready || w.stdin == nil || w.stdout == nil {
		w.mu.Unlock()
		return Result{}, errors.New("ocr worker is not ready")
	}
	stdin := w.stdin
	stdout := w.stdout
	w.mu.Unlock()

	imageBytes, err := DecodeImageDataURL(imageDataURL)
	if err != nil {
		return Result{}, err
	}
	imageWidth, imageHeight := imageDimensions(imageBytes)

	command, err := json.Marshal(map[string]string{"image_base64": base64.StdEncoding.EncodeToString(imageBytes)})
	if err != nil {
		return Result{}, fmt.Errorf("ocr worker: encode command: %w", err)
	}

	type output struct {
		data []byte
		err  error
	}
	resultCh := make(chan output, 1)
	go func() {
		if _, err := fmt.Fprintln(stdin, string(command)); err != nil {
			resultCh <- output{err: fmt.Errorf("ocr worker: write command: %w", err)}
			return
		}
		raw, err := readOneResult(stdout)
		resultCh <- output{data: raw, err: err}
	}()

	select {
	case <-runCtx.Done():
		// A request already written to the stream cannot be abandoned safely:
		// its late response would otherwise be consumed by the next capture.
		// Stop it before releasing the run gate, then warm a clean worker.
		w.stopProcess()
		restartAfterRun = true
		if errors.Is(runCtx.Err(), context.Canceled) {
			return Result{}, runCtx.Err()
		}
		return Result{}, fmt.Errorf("RapidOCR timed out after %s", w.timeout)
	case out := <-resultCh:
		if out.err != nil {
			w.stopProcess()
			restartAfterRun = true
			return Result{}, out.err
		}
		return ExtractResultFromJSON(out.data, imageWidth, imageHeight)
	}
}

// readOneResult reads from the given reader until a complete JSON result is
// available. RapidOCR-json may print banner lines before the result and the
// result itself may be formatted across multiple lines, so lines are
// accumulated until the JSON braces balance.
func readOneResult(reader io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var buffer []byte
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.Contains(strings.ToLower(line), "init completed") {
			continue
		}
		if len(buffer) == 0 && !strings.HasPrefix(line, "{") && !strings.HasPrefix(line, "[") {
			continue
		}
		buffer = append(buffer, line...)
		if jsonBalanced(string(buffer)) {
			return buffer, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("ocr worker: process exited before returning a result")
}

// jsonBalanced reports whether the braces and brackets in the given bytes
// are balanced, ignoring braces inside string literals.
func jsonBalanced(raw string) bool {
	depth := 0
	inString := false
	escaped := false
	for _, char := range raw {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch char {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
	}
	return !inString && depth == 0
}

func (w *Worker) runContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := w.timeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func (w *Worker) ensureReady(ctx context.Context) error {
	w.mu.Lock()
	ready := w.ready
	w.mu.Unlock()
	if ready {
		return nil
	}
	return w.Start(ctx)
}

func (w *Worker) warmAfterFailure() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = w.Start(ctx)
}

// Restart terminates the current process (if any) and starts a fresh one.
func (w *Worker) Restart(ctx context.Context) error {
	if err := w.acquireRun(ctx); err != nil {
		return err
	}
	defer w.releaseRun()
	w.stopProcess()
	return w.Start(ctx)
}

// Close terminates the worker process and releases resources.
func (w *Worker) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	w.closeProcessLocked()
}

func (w *Worker) stopProcess() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closeProcessLocked()
}

func (w *Worker) closeProcessLocked() {
	if w.stdin != nil {
		_ = w.stdin.Close()
		w.stdin = nil
	}
	if w.processGuard != nil {
		_ = w.processGuard.Close()
		w.processGuard = nil
	}
	if w.cmd != nil {
		_ = w.cmd.Process.Kill()
		_ = w.cmd.Wait()
		w.cmd = nil
	}
	w.stdout = nil
	w.ready = false
}
