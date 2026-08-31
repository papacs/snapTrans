//go:build windows

package selection

import (
	"context"
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/lxn/win"
)

// Slots and GUIDs follow Microsoft's UIAutomationClient.h, not IDispatch.
// No method here changes focus, selection, text, scrolling, or the clipboard.
type comPtr struct{ vtable *[90]uintptr }

func (p *comPtr) call(slot int, args ...uintptr) error {
	if p == nil {
		return ErrUnsupported
	}
	values := append([]uintptr{uintptr(unsafe.Pointer(p))}, args...)
	hr, _, _ := syscall.SyscallN(p.vtable[slot], values...)
	runtime.KeepAlive(p)
	if int32(hr) < 0 {
		return fmt.Errorf("UI Automation HRESULT 0x%08X", uint32(hr))
	}
	return nil
}
func (p *comPtr) release() {
	if p != nil {
		_ = p.call(2)
	}
}
func (p *comPtr) object(slot int, args ...uintptr) (*comPtr, error) {
	var out *comPtr
	err := p.call(slot, append(args, uintptr(unsafe.Pointer(&out)))...)
	runtime.KeepAlive(&out)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, ErrUnsupported
	}
	return out, nil
}
func (p *comPtr) integer(slot int) (int32, error) {
	var value int32
	err := p.call(slot, uintptr(unsafe.Pointer(&value)))
	runtime.KeepAlive(&value)
	return value, err
}

var readerOnce sync.Once
var defaultReader *Reader

func DefaultReader() *Reader {
	readerOnce.Do(func() {
		defaultReader = &Reader{requests: make(chan request), done: make(chan struct{})}
		go defaultReader.run()
	})
	return defaultReader
}
func Foreground() uintptr {
	hwnd := win.GetForegroundWindow()
	var pid uint32
	win.GetWindowThreadProcessId(hwnd, &pid)
	if hwnd == 0 || pid == uint32(os.Getpid()) {
		return 0
	}
	return uintptr(hwnd)
}
func StillForeground(hwnd uintptr) bool {
	return hwnd != 0 && uintptr(win.GetForegroundWindow()) == hwnd
}
func (r *Reader) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(r.done)
	automation, err := newAutomation()
	if err != nil {
		return
	}
	defer ole.CoUninitialize()
	defer automation.release()
	for req := range r.requests {
		if err := req.ctx.Err(); err != nil {
			req.result <- response{err: err}
			continue
		}
		result, err := readForeground(automation, req.ctx, req.window)
		req.result <- response{result: result, err: err}
	}
}
func newAutomation() (*comPtr, error) {
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		if value, ok := err.(*ole.OleError); !ok || value.Code() != 1 {
			return nil, err
		}
	}
	unknown, err := ole.CreateInstance(ole.NewGUID("{E22AD333-B25F-460C-83D0-0581107395C9}"), ole.NewGUID("{34723AFF-0C9D-49D0-9896-7AB52DF8CD8A}"))
	if err != nil {
		ole.CoUninitialize()
		return nil, err
	}
	automation := (*comPtr)(unsafe.Pointer(unknown))
	// IUIAutomation2: provider connection and transaction timeouts (ms).
	if err = automation.call(61, 150); err == nil {
		err = automation.call(63, 150)
	}
	if err != nil {
		automation.release()
		ole.CoUninitialize()
		return nil, err
	}
	_ = automation.call(59, 0) // Reading must never activate an application.
	return automation, nil
}
func readForeground(automation *comPtr, ctx context.Context, window uintptr) (Result, error) {
	if window == 0 {
		return Result{}, ErrBlocked
	}
	if !StillForeground(window) {
		return Result{}, ErrChanged
	}
	var pid uint32
	win.GetWindowThreadProcessId(win.HWND(window), &pid)
	if pid == uint32(os.Getpid()) {
		return Result{}, ErrBlocked
	}
	element, err := automation.object(8) // GetFocusedElement
	if err != nil {
		return Result{}, err
	}
	walker, err := automation.object(16) // RawViewWalker
	if err != nil {
		element.release()
		return Result{}, err
	}
	defer walker.release()
	for depth := 0; depth < 8; depth++ {
		if err = ctx.Err(); err != nil {
			element.release()
			return Result{}, err
		}
		// Stay inside the foreground process. Never walk desktop-wide trees.
		currentPID, pidErr := element.integer(20)
		password, passwordErr := element.integer(35)
		if pidErr != nil || passwordErr != nil || uint32(currentPID) != pid || password != 0 {
			element.release()
			return Result{}, ErrBlocked
		}
		pattern, patternErr := element.object(16, 10014) // GetCurrentPattern(TextPattern)
		if patternErr == nil {
			result, readErr := readPattern(ctx, pattern)
			pattern.release()
			element.release()
			if !StillForeground(window) {
				return Result{}, ErrChanged
			}
			result.Window = window
			return result, readErr
		}
		parent, parentErr := walker.object(3, uintptr(unsafe.Pointer(element)))
		element.release()
		if parentErr != nil {
			return Result{}, ErrUnsupported
		}
		element = parent
	}
	element.release()
	return Result{}, ErrUnsupported
}
func readPattern(ctx context.Context, pattern *comPtr) (Result, error) {
	ranges, err := pattern.object(5) // GetSelection
	if err != nil {
		return Result{}, ErrNoSelection
	}
	defer ranges.release()
	count, err := ranges.integer(3)
	if err != nil || count > 16 {
		return Result{}, ErrUnsupported
	}
	if count == 0 {
		return Result{}, ErrNoSelection
	}
	result := Result{}
	chars := 0
	for i := int32(0); i < count; i++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		span, err := ranges.object(4, uintptr(i))
		if err != nil {
			return Result{}, err
		}
		lines, readErr := readRangeLines(ctx, span)
		span.release()
		if readErr != nil {
			return Result{}, readErr
		}
		for _, line := range lines {
			chars += len([]rune(line.Text))
		}
		result.Lines = append(result.Lines, lines...)
		if chars > MaxCharacters || len(result.Lines) > MaxLines {
			return Result{}, ErrUnsupported
		}
	}
	if len(result.Lines) == 0 {
		return Result{}, ErrNoSelection
	}
	return result, nil
}
func rangeText(span *comPtr) (string, error) {
	var value *uint16
	err := span.call(12, MaxCharacters+1, uintptr(unsafe.Pointer(&value)))
	runtime.KeepAlive(&value)
	if err != nil {
		return "", err
	}
	if value == nil {
		return "", nil
	}
	defer ole.SysFreeString((*int16)(unsafe.Pointer(value)))
	text := ole.BstrToString(value)
	if len([]rune(text)) > MaxCharacters {
		return "", ErrUnsupported
	}
	return text, nil
}
func compare(a *comPtr, aEnd uintptr, b *comPtr, bEnd uintptr) (int32, error) {
	var result int32
	err := a.call(5, aEnd, uintptr(unsafe.Pointer(b)), bEnd, uintptr(unsafe.Pointer(&result)))
	runtime.KeepAlive(b)
	runtime.KeepAlive(&result)
	return result, err
}
func moveTo(a *comPtr, aEnd uintptr, b *comPtr, bEnd uintptr) error {
	err := a.call(15, aEnd, uintptr(unsafe.Pointer(b)), bEnd)
	runtime.KeepAlive(b)
	return err
}
func readRangeLines(ctx context.Context, span *comPtr) ([]Line, error) {
	text, err := rangeText(span)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	cursor, err := span.object(3)
	if err != nil {
		return nil, err
	}
	defer cursor.release()
	if err = moveTo(cursor, 1, span, 0); err != nil {
		return nil, err
	}
	lines := []Line{}
	var consumed strings.Builder
	for n := 0; n < MaxLines*2; n++ {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		cmp, err := compare(cursor, 0, span, 1)
		if err != nil {
			return nil, err
		}
		if cmp >= 0 {
			if compactText(consumed.String()) != compactText(text) {
				return nil, fmt.Errorf("%w: line text coverage mismatch", ErrUnsupported)
			}
			return lines, nil
		}
		line, err := cursor.object(3)
		if err != nil {
			return nil, err
		}
		got, segment, err := readLine(line, span)
		if err != nil {
			line.release()
			return nil, err
		}
		progress, err := compare(line, 1, cursor, 0)
		if err == nil && progress == 0 {
			// Chromium may expand a collapsed boundary to the preceding line.
			// Step into the next character on our cloned range, then expand;
			// the final coverage check ensures no selected character is lost.
			line.release()
			var moved int32
			err = cursor.call(14, 0, 0, 1, uintptr(unsafe.Pointer(&moved)))
			runtime.KeepAlive(&moved)
			if err != nil || moved != 1 {
				return nil, ErrUnsupported
			}
			continue
		}
		if err != nil || progress < 0 {
			line.release()
			return nil, fmt.Errorf("%w: line endpoint did not advance (%d)", ErrUnsupported, progress)
		}
		err = moveTo(cursor, 0, line, 1)
		line.release()
		if err != nil {
			return nil, err
		}
		consumed.WriteString(segment)
		if got != nil {
			lines = append(lines, *got)
			if len(lines) > MaxLines {
				return nil, ErrUnsupported
			}
		}
	}
	return nil, ErrUnsupported
}
func compactText(text string) string { return strings.Join(strings.Fields(text), "") }
func readLine(line, span *comPtr) (*Line, string, error) {
	if err := line.call(6, 3); err != nil {
		return nil, "", err
	} // ExpandToEnclosingUnit(Line)
	cmp, err := compare(line, 0, span, 0)
	if err != nil {
		return nil, "", err
	}
	if cmp < 0 {
		if err = moveTo(line, 0, span, 0); err != nil {
			return nil, "", err
		}
	}
	cmp, err = compare(line, 1, span, 1)
	if err != nil {
		return nil, "", err
	}
	if cmp > 0 {
		if err = moveTo(line, 1, span, 1); err != nil {
			return nil, "", err
		}
	}
	text, err := rangeText(line)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(text) == "" {
		return nil, text, nil
	}
	var array *ole.SafeArray
	err = line.call(10, uintptr(unsafe.Pointer(&array)))
	runtime.KeepAlive(&array)
	if err != nil || array == nil {
		return nil, "", fmt.Errorf("%w: missing line rectangles (%v)", ErrUnsupported, err)
	}
	values := &ole.SafeArrayConversion{Array: array}
	defer values.Release()
	count, err := values.TotalElements(0)
	if err != nil || count == 0 || count%4 != 0 || count > 256 {
		return nil, "", fmt.Errorf("%w: line rectangle array count %d (%v)", ErrUnsupported, count, err)
	}
	kind, err := values.GetType()
	if err != nil || kind != uint16(ole.VT_R8) {
		return nil, "", fmt.Errorf("%w: rectangle array type %d (%v)", ErrUnsupported, kind, err)
	}
	raw := values.ToValueArray()
	rects := make([]Line, 0, len(raw)/4)
	for i := 0; i < len(raw); i += 4 {
		nums := [4]float64{}
		for j := 0; j < 4; j++ {
			v, ok := raw[i+j].(float64)
			if !ok || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e7 {
				return nil, "", ErrUnsupported
			}
			nums[j] = v
		}
		if nums[2] > 0 && nums[3] > 0 {
			rects = append(rects, Line{X: nums[0], Y: nums[1], Width: nums[2], Height: nums[3]})
		}
	}
	rect, err := mergeLineRects(rects)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %d line rectangles are not contiguous", err, len(rects))
	}
	rect.Text = strings.TrimSpace(text)
	rect.Background = rangeColor(line, 40001)
	rect.Foreground = rangeColor(line, 40008)
	return &rect, text, nil
}

func rangeColor(span *comPtr, attribute uintptr) string {
	var value ole.VARIANT
	if err := span.call(9, attribute, uintptr(unsafe.Pointer(&value))); err != nil {
		return ""
	}
	defer value.Clear()
	if value.VT != ole.VT_I4 {
		return ""
	}
	color := uint32(value.Val)
	if color > 0xffffff {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", color&255, (color>>8)&255, (color>>16)&255)
}
