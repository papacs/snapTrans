package capture

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultScrollFrames = 128
	defaultScrollPixels = int64(40_000_000)
	maxCanvasHeight     = 32767
)

type ManualScrollOptions struct {
	MaxFrames int
	MaxPixels int64
}

type ManualScrollStatus struct {
	CaptureID uint64 `json:"captureId,omitempty"`
	Frames    int    `json:"frames"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Error     string `json:"error,omitempty"`
}

type ManualScrollSnapshot struct {
	CurrentImageBytes []byte `json:"-"`
	PreviewImageBytes []byte `json:"-"`
	Frames            int    `json:"frames"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	LimitReached      bool   `json:"limitReached"`
	Appended          bool   `json:"appended"`
}

// ManualScrollCapture observes the selected window while the user scrolls it.
// This type never drives input; it only captures and joins frames with a
// reliable downward visual overlap.
type ManualScrollCapture struct {
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	canceled     atomic.Bool
	rect         image.Rectangle
	stitcher     *verticalStitcher
	captureFrame regionCapture
	maxFrames    int
	started      time.Time
}

func StartManualScrollCapture(
	ctx context.Context,
	rect image.Rectangle,
	options ManualScrollOptions,
) (*ManualScrollCapture, error) {
	return StartManualScrollCaptureWithSource(ctx, rect, options, func(rect image.Rectangle) (image.Image, error) {
		return ScreenRegion(rect)
	})
}

// StartManualScrollCaptureWithSource starts a scrolling session with a
// platform-specific frame source. On Windows this lets the desktop shell
// capture the underlying target window while the overlay remains visible.
func StartManualScrollCaptureWithSource(
	ctx context.Context,
	rect image.Rectangle,
	options ManualScrollOptions,
	captureFrame func(image.Rectangle) (image.Image, error),
) (*ManualScrollCapture, error) {
	return startManualScrollCapture(ctx, rect, options, captureFrame)
}

type regionCapture func(image.Rectangle) (image.Image, error)

func startManualScrollCapture(
	ctx context.Context,
	rect image.Rectangle,
	options ManualScrollOptions,
	captureFrame regionCapture,
) (*ManualScrollCapture, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if rect.Dx() < 2 || rect.Dy() < 2 {
		return nil, errors.New("scrolling capture region is too small")
	}
	if captureFrame == nil {
		return nil, errors.New("scrolling capture is unavailable")
	}
	if options.MaxFrames <= 0 {
		options.MaxFrames = defaultScrollFrames
	}
	if options.MaxPixels <= 0 {
		options.MaxPixels = defaultScrollPixels
	}
	first, err := captureFrame(rect)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stitcher, err := newVerticalStitcher(first, options.MaxPixels)
	if err != nil {
		return nil, err
	}

	sessionContext, cancel := context.WithCancel(ctx)
	return &ManualScrollCapture{
		ctx:          sessionContext,
		cancel:       cancel,
		rect:         rect,
		stitcher:     stitcher,
		captureFrame: captureFrame,
		maxFrames:    options.MaxFrames,
		started:      time.Now(),
	}, nil
}

func (session *ManualScrollCapture) CaptureNext() (ManualScrollSnapshot, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if err := session.cancellationError(); err != nil {
		return ManualScrollSnapshot{}, err
	}

	if session.stitcher.frameCount() >= session.maxFrames || session.stitcher.limitReached {
		return ManualScrollSnapshot{Frames: session.stitcher.frameCount(), Width: session.stitcher.frames[0].Bounds().Dx(), Height: session.stitcher.totalHeight, LimitReached: true}, nil
	}
	frame, err := session.captureFrame(session.rect)
	if err != nil {
		return ManualScrollSnapshot{}, err
	}
	if err := session.cancellationError(); err != nil {
		return ManualScrollSnapshot{}, err
	}
	appended := false
	if session.stitcher.frameCount() < session.maxFrames {
		appended, err = session.stitcher.addManualContext(session.ctx, frame)
		if err != nil {
			return ManualScrollSnapshot{}, err
		}
	}
	if err := session.cancellationError(); err != nil {
		return ManualScrollSnapshot{}, err
	}
	if !appended {
		return ManualScrollSnapshot{
			Frames:       session.stitcher.frameCount(),
			Width:        session.stitcher.frames[0].Bounds().Dx(),
			Height:       session.stitcher.totalHeight,
			Appended:     false,
			LimitReached: session.stitcher.limitReached,
		}, nil
	}

	return session.snapshotLocked(appended)
}

func (session *ManualScrollCapture) Status() ManualScrollStatus {
	session.mu.Lock()
	defer session.mu.Unlock()
	status := ManualScrollStatus{
		Frames: session.stitcher.frameCount(),
		Width:  session.stitcher.frames[0].Bounds().Dx(),
		Height: session.stitcher.totalHeight,
	}
	return status
}

func (session *ManualScrollCapture) snapshotLocked(appended bool) (ManualScrollSnapshot, error) {
	preview := session.stitcher.thumbnail(240, 4096)
	previewBytes, err := encodePNGBytesContext(session.ctx, preview, png.BestSpeed)
	if err != nil {
		return ManualScrollSnapshot{}, err
	}
	if err := session.cancellationError(); err != nil {
		return ManualScrollSnapshot{}, err
	}
	return ManualScrollSnapshot{
		PreviewImageBytes: previewBytes,
		Frames:            session.stitcher.frameCount(),
		Width:             session.stitcher.frames[0].Bounds().Dx(),
		Height:            session.stitcher.totalHeight,
		Appended:          appended,
		LimitReached:      session.stitcher.frameCount() >= session.maxFrames || session.stitcher.limitReached,
	}, nil
}

func (session *ManualScrollCapture) Finish() (Result, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if err := session.cancellationError(); err != nil {
		return Result{}, err
	}

	frame := session.stitcher.image()
	if err := session.cancellationError(); err != nil {
		return Result{}, err
	}
	encodeStartedAt := time.Now()
	encoded, err := encodePNGBytesContext(session.ctx, frame, png.BestSpeed)
	if err != nil {
		return Result{}, err
	}
	if err := session.cancellationError(); err != nil {
		return Result{}, err
	}
	return Result{
		ImageBytes:      encoded,
		Width:           frame.Bounds().Dx(),
		Height:          frame.Bounds().Dy(),
		OriginX:         session.rect.Min.X,
		OriginY:         session.rect.Min.Y,
		Source:          "wails",
		Mode:            "screenshot",
		ScrollFrames:    session.stitcher.frameCount(),
		CaptureDuration: time.Since(session.started),
		EncodeDuration:  time.Since(encodeStartedAt),
		EncodedBytes:    len(encoded),
		CompressionMode: pngCompressionMode(png.BestSpeed),
	}, nil
}

// Cancellation must not wait behind a native frame capture holding mu.
func (session *ManualScrollCapture) Cancel() {
	session.canceled.Store(true)
	if session.cancel != nil {
		session.cancel()
	}
}

func (session *ManualScrollCapture) cancellationError() error {
	if session.canceled.Load() {
		return context.Canceled
	}
	return session.ctx.Err()
}

type verticalStitcher struct {
	frames        []*image.RGBA
	totalHeight   int
	observedFrame *image.RGBA
	observedTop   int
	maxPixels     int64
	limitReached  bool
}

func newVerticalStitcher(first image.Image, maxPixels int64) (*verticalStitcher, error) {
	normalized := rgbaImage(first)
	if normalized.Bounds().Empty() {
		return nil, errors.New("scrolling capture returned an empty frame")
	}
	if int64(normalized.Bounds().Dx())*int64(normalized.Bounds().Dy()) > maxPixels {
		return nil, errors.New("scrolling capture exceeds the image size limit")
	}
	return &verticalStitcher{
		frames:        []*image.RGBA{normalized},
		totalHeight:   normalized.Bounds().Dy(),
		observedFrame: normalized,
		observedTop:   0,
		maxPixels:     maxPixels,
	}, nil
}

func (stitcher *verticalStitcher) frameCount() int {
	return len(stitcher.frames)
}

func (stitcher *verticalStitcher) addManual(next image.Image) bool {
	appended, _ := stitcher.addManualContext(context.Background(), next)
	return appended
}

func (stitcher *verticalStitcher) addManualContext(ctx context.Context, next image.Image) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	observed := stitcher.observedFrame
	normalized := rgbaImage(next)
	if observed.Bounds().Dx() != normalized.Bounds().Dx() || observed.Bounds().Dy() != normalized.Bounds().Dy() {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if bytes.Equal(observed.Pix, normalized.Pix) {
		return false, nil
	}
	coherentAdvance, coherent, err := coherentVerticalAdvance(ctx, observed, normalized)
	if err != nil {
		return false, err
	}
	if coherent && coherentAdvance == 0 {
		return false, nil
	}
	forwardAdvance, reverseAdvance := maxInt(0, coherentAdvance), maxInt(0, -coherentAdvance)
	movingDown := coherentAdvance > 0
	if !coherent {
		// A stationary screen, including one with a small fixed controller overlay,
		// is not a new frame. The row-trimmed comparison ignores that overlay.
		if sampledDifference(observed, normalized, 0) <= 2.2 ||
			stationaryFeatureMatchRatio(observed, normalized) >= 0.42 {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			stitcher.observedFrame = normalized
			return false, nil
		}

		var forwardScore, reverseScore float64
		var forwardFound, reverseFound bool
		forwardAdvance, forwardScore, forwardFound, err = verticalAdvanceContext(ctx, observed, normalized)
		if err != nil {
			return false, err
		}
		reverseAdvance, reverseScore, reverseFound, err = verticalAdvanceContext(ctx, normalized, observed)
		if err != nil {
			return false, err
		}
		forwardOK := forwardFound && forwardScore <= 24
		reverseOK := reverseFound && reverseScore <= 24
		movingDown = forwardOK
		if !forwardOK && !reverseOK {
			return false, nil
		}
		// Compare direction with all sampled rows. The trimmed overlap score above
		// tolerates sticky or animated regions, but it can make repeated table rows
		// look equally plausible in both directions. Full-row scoring retains the
		// timestamps and text that distinguish a real upward move.
		if forwardOK && reverseOK {
			forwardDirectionScore := directionalDifference(observed, normalized, forwardAdvance)
			reverseDirectionScore := directionalDifference(normalized, observed, reverseAdvance)
			movingDown = reverseDirectionScore+2 >= forwardDirectionScore
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if !movingDown {
		stitcher.observedTop = maxInt(0, stitcher.observedTop-reverseAdvance)
		stitcher.observedFrame = normalized
		return false, nil
	}

	nextTop := stitcher.observedTop + forwardAdvance
	nextBottom := nextTop + normalized.Bounds().Dy()
	stitcher.observedTop = nextTop
	stitcher.observedFrame = normalized
	if nextBottom <= stitcher.totalHeight {
		return false, nil
	}

	extension := nextBottom - stitcher.totalHeight
	width := observed.Bounds().Dx()
	if int64(width)*int64(stitcher.totalHeight+extension) > stitcher.maxPixels ||
		stitcher.totalHeight+extension > maxCanvasHeight {
		stitcher.limitReached = true
		return false, nil
	}
	// Keep only new rows. Small touchpad movements otherwise retain an entire
	// viewport per poll and exhaust both memory and the frame limit quickly.
	strip := normalized.SubImage(image.Rect(0, normalized.Bounds().Dy()-extension, width, normalized.Bounds().Dy()))
	stitcher.frames = append(stitcher.frames, rgbaImage(strip))
	stitcher.totalHeight += extension
	return true, nil
}

func (stitcher *verticalStitcher) image() *image.RGBA {
	width := stitcher.frames[0].Bounds().Dx()
	result := image.NewRGBA(image.Rect(0, 0, width, stitcher.totalHeight))
	y := 0
	for _, frame := range stitcher.frames {
		target := image.Rect(0, y, width, y+frame.Bounds().Dy())
		draw.Draw(result, target, frame, image.Point{}, draw.Src)
		y += frame.Bounds().Dy()
	}
	return result
}

func (stitcher *verticalStitcher) thumbnail(maxWidth int, maxHeight int) *image.RGBA {
	sourceWidth := stitcher.frames[0].Bounds().Dx()
	sourceHeight := stitcher.totalHeight
	targetWidth := sourceWidth
	targetHeight := sourceHeight
	if targetWidth > maxWidth {
		targetHeight = maxInt(1, targetHeight*maxWidth/targetWidth)
		targetWidth = maxWidth
	}
	if targetHeight > maxHeight {
		targetWidth = maxInt(1, targetWidth*maxHeight/targetHeight)
		targetHeight = maxHeight
	}
	thumbnail := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := 0; y < targetHeight; y++ {
		sourceY := minInt(sourceHeight-1, y*sourceHeight/targetHeight)
		frameY := sourceY
		frame := stitcher.frames[0]
		for _, candidate := range stitcher.frames {
			frame = candidate
			if frameY < frame.Bounds().Dy() {
				break
			}
			frameY -= frame.Bounds().Dy()
		}
		for x := 0; x < targetWidth; x++ {
			sourceX := minInt(sourceWidth-1, x*sourceWidth/targetWidth)
			thumbnail.SetRGBA(x, y, frame.RGBAAt(sourceX, frameY))
		}
	}
	return thumbnail
}

func verticalAdvance(previous image.Image, next image.Image) (int, float64, bool) {
	advance, score, found, _ := verticalAdvanceContext(context.Background(), previous, next)
	return advance, score, found
}

func verticalAdvanceContext(ctx context.Context, previous image.Image, next image.Image) (int, float64, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, false, err
	}
	width := previous.Bounds().Dx()
	height := previous.Bounds().Dy()
	if width != next.Bounds().Dx() || height != next.Bounds().Dy() || width < 2 || height < 8 {
		return 0, 0, false, nil
	}

	minimumOverlap := maxInt(12, height/10)
	maxAdvance := height - minimumOverlap
	if maxAdvance < 1 {
		return 0, 0, false, nil
	}

	bestAdvance := 0
	bestScore := 1e9
	for advance := 1; advance <= maxAdvance; advance++ {
		if advance%16 == 1 {
			if err := ctx.Err(); err != nil {
				return 0, 0, false, err
			}
		}
		score := sampledDifference(previous, next, advance)
		if score < bestScore {
			bestScore = score
			bestAdvance = advance
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, 0, false, err
	}
	return bestAdvance, bestScore, bestAdvance > 0, nil
}

// stationaryFeatureMatchRatio measures how much non-flat visual content stays
// at the same screen coordinates. It rejects false advances caused by animated
// regions over repeated list rows while ignoring blank page backgrounds.
func stationaryFeatureMatchRatio(previous image.Image, next image.Image) float64 {
	width := previous.Bounds().Dx()
	height := previous.Bounds().Dy()
	if width != next.Bounds().Dx() || height != next.Bounds().Dy() || width < 2 || height < 2 {
		return 0
	}

	left := width / 20
	right := width - left
	if right <= left {
		return 0
	}
	stepX := maxInt(1, (right-left)/40)
	stepY := maxInt(1, height/48)
	featureRows := 0
	matchingRows := 0
	for y := 0; y < height; y += stepY {
		var difference uint64
		var activity uint64
		var differenceSamples uint64
		var activitySamples uint64
		var previousPixel color.RGBA
		var nextPixel color.RGBA
		hasPreviousPixel := false
		for x := left; x < right; x += stepX {
			previousColor := scrollRGBAAt(previous, previous.Bounds().Min.X+x, previous.Bounds().Min.Y+y)
			nextColor := scrollRGBAAt(next, next.Bounds().Min.X+x, next.Bounds().Min.Y+y)
			difference += uint64(absByteDifference(previousColor.R, nextColor.R))
			difference += uint64(absByteDifference(previousColor.G, nextColor.G))
			difference += uint64(absByteDifference(previousColor.B, nextColor.B))
			differenceSamples += 3
			if hasPreviousPixel {
				activity += uint64(absByteDifference(previousPixel.R, previousColor.R))
				activity += uint64(absByteDifference(previousPixel.G, previousColor.G))
				activity += uint64(absByteDifference(previousPixel.B, previousColor.B))
				activity += uint64(absByteDifference(nextPixel.R, nextColor.R))
				activity += uint64(absByteDifference(nextPixel.G, nextColor.G))
				activity += uint64(absByteDifference(nextPixel.B, nextColor.B))
				activitySamples += 6
			}
			previousPixel = previousColor
			nextPixel = nextColor
			hasPreviousPixel = true
		}
		if activitySamples == 0 || differenceSamples == 0 ||
			float64(activity)/float64(activitySamples) < 6 {
			continue
		}
		featureRows++
		if float64(difference)/float64(differenceSamples) <= 3 {
			matchingRows++
		}
	}
	if featureRows == 0 {
		return 0
	}
	return float64(matchingRows) / float64(featureRows)
}

// sampledDifference compares previous[y+advance] with next[y]. It averages
// the best-matching 70% of sampled rows so sticky headers, video, carets, and
// the manual-capture controller do not distort an otherwise exact overlap.
func sampledDifference(previous image.Image, next image.Image, advance int) float64 {
	return sampledDifferenceKeepingRows(previous, next, advance, 7)
}

func directionalDifference(previous image.Image, next image.Image, advance int) float64 {
	return sampledDifferenceKeepingRows(previous, next, advance, 10)
}

func sampledDifferenceKeepingRows(previous image.Image, next image.Image, advance int, keptTenths int) float64 {
	if previousRGBA, ok := previous.(*image.RGBA); ok {
		if nextRGBA, ok := next.(*image.RGBA); ok {
			return sampledRGBADifferenceKeepingRows(previousRGBA, nextRGBA, advance, keptTenths)
		}
	}
	return sampledDifferenceKeepingRowsGeneric(previous, next, advance, keptTenths)
}

func sampledDifferenceKeepingRowsGeneric(previous image.Image, next image.Image, advance int, keptTenths int) float64 {
	width := previous.Bounds().Dx()
	height := previous.Bounds().Dy()
	if width != next.Bounds().Dx() || height != next.Bounds().Dy() || advance < 0 || advance >= height {
		return 1e9
	}

	left := width / 20
	right := width - left
	bottom := height - advance
	if right <= left || bottom <= 0 {
		return 1e9
	}
	stepX := maxInt(1, (right-left)/28)
	stepY := maxInt(1, bottom/28)
	rowScores := make([]float64, 0, 29)
	for y := 0; y < bottom; y += stepY {
		var difference uint64
		var samples uint64
		var firstLeft, firstRight color.RGBA
		var hasFeature bool
		for x := left; x < right; x += stepX {
			leftColor := scrollRGBAAt(previous, previous.Bounds().Min.X+x, previous.Bounds().Min.Y+y+advance)
			rightColor := scrollRGBAAt(next, next.Bounds().Min.X+x, next.Bounds().Min.Y+y)
			if samples == 0 {
				firstLeft, firstRight = leftColor, rightColor
			} else if leftColor != firstLeft || rightColor != firstRight {
				hasFeature = true
			}
			difference += uint64(absByteDifference(leftColor.R, rightColor.R))
			difference += uint64(absByteDifference(leftColor.G, rightColor.G))
			difference += uint64(absByteDifference(leftColor.B, rightColor.B))
			samples += 3
		}
		if samples > 0 && !hasFeature {
			// Horizontal bands and gradients still contain vertical features.
			previousY := minInt(height-1, y+advance+1)
			nextY := minInt(height-1, y+1)
			hasFeature = scrollRGBAAt(previous, previous.Bounds().Min.X+left, previous.Bounds().Min.Y+previousY) != firstLeft ||
				scrollRGBAAt(next, next.Bounds().Min.X+left, next.Bounds().Min.Y+nextY) != firstRight
		}
		// Blank page margins are not evidence of overlap. Keeping them in the
		// best 70% makes sparse text appear stationary at every scroll offset.
		if samples > 0 && (difference > 0 || hasFeature) {
			rowScores = append(rowScores, float64(difference)/float64(samples))
		}
	}
	return trimmedScrollRowScore(rowScores, advance, keptTenths)
}

func trimmedScrollRowScore(rowScores []float64, advance, keptTenths int) float64 {
	if len(rowScores) == 0 {
		if advance == 0 {
			return 0
		}
		return 1e9
	}
	sort.Float64s(rowScores)
	keptRows := maxInt(1, (len(rowScores)*keptTenths+9)/10)
	var total float64
	for _, score := range rowScores[:keptRows] {
		total += score
	}
	return total / float64(keptRows)
}

func scrollRGBAAt(source image.Image, x, y int) color.RGBA {
	if rgba, ok := source.(*image.RGBA); ok {
		return rgba.RGBAAt(x, y)
	}
	return color.RGBAModel.Convert(source.At(x, y)).(color.RGBA)
}

func rgbaImage(source image.Image) *image.RGBA {
	bounds := source.Bounds()
	result := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(result, result.Bounds(), source, bounds.Min, draw.Src)
	return result
}

func absByteDifference(left uint8, right uint8) int {
	if left >= right {
		return int(left - right)
	}
	return int(right - left)
}
