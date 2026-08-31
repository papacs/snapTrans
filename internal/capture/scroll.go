package capture

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sort"
	"sync"
	"time"

	"github.com/kbinani/screenshot"
)

const (
	defaultScrollFrames = 16
	defaultScrollPixels = int64(40_000_000)
	maxCanvasHeight     = 32767
)

type ManualScrollOptions struct {
	MaxFrames int
	MaxPixels int64
}

type ManualScrollStatus struct {
	Frames int    `json:"frames"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Error  string `json:"error,omitempty"`
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
		return screenshot.CaptureRect(rect)
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
	stitcher, err := newVerticalStitcher(first, options.MaxPixels)
	if err != nil {
		return nil, err
	}

	return &ManualScrollCapture{
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

	if session.stitcher.frameCount() >= session.maxFrames || session.stitcher.limitReached {
		return ManualScrollSnapshot{Frames: session.stitcher.frameCount(), Width: session.stitcher.frames[0].Bounds().Dx(), Height: session.stitcher.totalHeight, LimitReached: true}, nil
	}
	frame, err := session.captureFrame(session.rect)
	if err != nil {
		return ManualScrollSnapshot{}, err
	}
	appended := false
	if session.stitcher.frameCount() < session.maxFrames {
		appended = session.stitcher.addManual(frame)
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

	return session.snapshotLocked(frame, appended)
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

func (session *ManualScrollCapture) snapshotLocked(current image.Image, appended bool) (ManualScrollSnapshot, error) {
	currentBytes, err := encodePNGBytesWithLevel(current, png.BestSpeed)
	if err != nil {
		return ManualScrollSnapshot{}, err
	}
	preview := session.stitcher.thumbnail(240, 4096)
	previewBytes, err := encodePNGBytesWithLevel(preview, png.BestSpeed)
	if err != nil {
		return ManualScrollSnapshot{}, err
	}
	return ManualScrollSnapshot{
		CurrentImageBytes: currentBytes,
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

	frame := session.stitcher.image()
	encodeStartedAt := time.Now()
	encoded, err := encodePNGBytesWithLevel(frame, png.BestSpeed)
	if err != nil {
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

func (session *ManualScrollCapture) Cancel() {}

type verticalStitcher struct {
	frames        []*image.RGBA
	advances      []int
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
	observed := stitcher.observedFrame
	normalized := rgbaImage(next)
	if observed.Bounds().Dx() != normalized.Bounds().Dx() || observed.Bounds().Dy() != normalized.Bounds().Dy() {
		return false
	}
	// A stationary screen, including one with a small fixed controller overlay,
	// is not a new frame. The row-trimmed comparison ignores that overlay.
	if sampledDifference(observed, normalized, 0) <= 2.2 ||
		stationaryFeatureMatchRatio(observed, normalized) >= 0.42 {
		stitcher.observedFrame = normalized
		return false
	}

	forwardAdvance, forwardScore, forwardFound := verticalAdvance(observed, normalized)
	reverseAdvance, reverseScore, reverseFound := verticalAdvance(normalized, observed)
	forwardOK := forwardFound && forwardScore <= 24
	reverseOK := reverseFound && reverseScore <= 24
	movingDown := forwardOK
	if !forwardOK && !reverseOK {
		return false
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

	if !movingDown {
		stitcher.observedTop = maxInt(0, stitcher.observedTop-reverseAdvance)
		stitcher.observedFrame = normalized
		return false
	}

	nextTop := stitcher.observedTop + forwardAdvance
	nextBottom := nextTop + normalized.Bounds().Dy()
	stitcher.observedTop = nextTop
	stitcher.observedFrame = normalized
	if nextBottom <= stitcher.totalHeight {
		return false
	}

	extension := nextBottom - stitcher.totalHeight
	width := observed.Bounds().Dx()
	if int64(width)*int64(stitcher.totalHeight+extension) > stitcher.maxPixels ||
		stitcher.totalHeight+extension > maxCanvasHeight {
		stitcher.limitReached = true
		return false
	}
	stitcher.frames = append(stitcher.frames, normalized)
	stitcher.advances = append(stitcher.advances, extension)
	stitcher.totalHeight += extension
	return true
}

func (stitcher *verticalStitcher) image() *image.RGBA {
	width := stitcher.frames[0].Bounds().Dx()
	result := image.NewRGBA(image.Rect(0, 0, width, stitcher.totalHeight))
	draw.Draw(result, image.Rect(0, 0, width, stitcher.frames[0].Bounds().Dy()), stitcher.frames[0], image.Point{}, draw.Src)
	y := stitcher.frames[0].Bounds().Dy()
	for index, advance := range stitcher.advances {
		frame := stitcher.frames[index+1]
		sourceY := frame.Bounds().Dy() - advance
		target := image.Rect(0, y, width, y+advance)
		draw.Draw(result, target, frame, image.Point{X: 0, Y: sourceY}, draw.Src)
		y += advance
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
		for x := 0; x < targetWidth; x++ {
			sourceX := minInt(sourceWidth-1, x*sourceWidth/targetWidth)
			thumbnail.Set(x, y, stitcher.at(sourceX, sourceY))
		}
	}
	return thumbnail
}

func (stitcher *verticalStitcher) at(x int, y int) color.Color {
	frameHeight := stitcher.frames[0].Bounds().Dy()
	if y < frameHeight {
		return stitcher.frames[0].At(x, y)
	}
	offset := frameHeight
	for index, advance := range stitcher.advances {
		if y < offset+advance {
			frame := stitcher.frames[index+1]
			return frame.At(x, frameHeight-advance+y-offset)
		}
		offset += advance
	}
	return stitcher.frames[len(stitcher.frames)-1].At(x, frameHeight-1)
}

func verticalAdvance(previous image.Image, next image.Image) (int, float64, bool) {
	width := previous.Bounds().Dx()
	height := previous.Bounds().Dy()
	if width != next.Bounds().Dx() || height != next.Bounds().Dy() || width < 2 || height < 8 {
		return 0, 0, false
	}

	minimumOverlap := maxInt(12, height/10)
	maxAdvance := height - minimumOverlap
	if maxAdvance < 1 {
		return 0, 0, false
	}

	bestAdvance := 0
	bestScore := 1e9
	for advance := 1; advance <= maxAdvance; advance++ {
		score := sampledDifference(previous, next, advance)
		if score < bestScore {
			bestScore = score
			bestAdvance = advance
		}
	}
	return bestAdvance, bestScore, bestAdvance > 0
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
			previousColor := color.RGBAModel.Convert(previous.At(previous.Bounds().Min.X+x, previous.Bounds().Min.Y+y)).(color.RGBA)
			nextColor := color.RGBAModel.Convert(next.At(next.Bounds().Min.X+x, next.Bounds().Min.Y+y)).(color.RGBA)
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
		for x := left; x < right; x += stepX {
			leftColor := color.RGBAModel.Convert(previous.At(previous.Bounds().Min.X+x, previous.Bounds().Min.Y+y+advance)).(color.RGBA)
			rightColor := color.RGBAModel.Convert(next.At(next.Bounds().Min.X+x, next.Bounds().Min.Y+y)).(color.RGBA)
			difference += uint64(absByteDifference(leftColor.R, rightColor.R))
			difference += uint64(absByteDifference(leftColor.G, rightColor.G))
			difference += uint64(absByteDifference(leftColor.B, rightColor.B))
			samples += 3
		}
		if samples > 0 {
			rowScores = append(rowScores, float64(difference)/float64(samples))
		}
	}
	if len(rowScores) == 0 {
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
