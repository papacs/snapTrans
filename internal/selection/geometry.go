package selection

import (
	"math"
	"sort"
)

// Inline styles may produce several adjacent boxes on one visual line.
// Disjoint columns and different baselines must not become one paint rectangle.
func mergeLineRects(rects []Line) (Line, error) {
	if len(rects) == 0 {
		return Line{}, ErrUnsupported
	}
	sort.Slice(rects, func(i, j int) bool { return rects[i].X < rects[j].X })
	merged := rects[0]
	for _, r := range rects[1:] {
		if math.Abs(r.Y-merged.Y) > math.Min(r.Height, merged.Height)*0.45 || r.X-(merged.X+merged.Width) > math.Max(r.Height, merged.Height)*2 {
			return Line{}, ErrUnsupported
		}
		right := math.Max(merged.X+merged.Width, r.X+r.Width)
		bottom := math.Max(merged.Y+merged.Height, r.Y+r.Height)
		merged.Y = math.Min(merged.Y, r.Y)
		merged.Width = right - merged.X
		merged.Height = bottom - merged.Y
	}
	return merged, nil
}
