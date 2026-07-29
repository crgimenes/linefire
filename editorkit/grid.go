package editorkit

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/crgimenes/linefire/render"
)

// minDotPx is the smallest on-screen dot spacing before the grid coarsens.
const minDotPx = 8.0

// visibleStep coarsens a world grid step by powers of two until its on-screen
// spacing is at least minPx, so the dots stay legible (and bounded in count)
// when zoomed out instead of vanishing. Returns 0 when no grid should be drawn
// (non-positive step or scale). The coarsened step is always a multiple of the
// original, so the visible dots still land on real grid lines.
func visibleStep(step, scale, minPx float64) float64 {
	if step <= 0 || scale <= 0 {
		return 0
	}
	s := step
	for s*scale < minPx {
		s *= 2
	}
	return s
}

// DrawDotGrid draws a faint grid of dots across the region at the given world
// spacing, plus brighter axis lines through (originX, originY). When zoomed out
// the spacing coarsens (in powers of two) so the dots remain visible rather than
// disappearing, keeping the cost bounded.
func DrawDotGrid(dst *ebiten.Image, view render.View, region image.Rectangle, step float64, dotColor, axisColor color.Color, originX, originY float64) {
	s := visibleStep(step, view.Scale, minDotPx)
	if s > 0 {
		minX, minY := view.Unproject(float64(region.Min.X), float64(region.Min.Y))
		maxX, maxY := view.Unproject(float64(region.Max.X), float64(region.Max.Y))
		startX := math.Floor(minX/s) * s
		startY := math.Floor(minY/s) * s
		for gx := startX; gx <= maxX; gx += s {
			for gy := startY; gy <= maxY; gy += s {
				sx, sy := view.Project(gx, gy)
				vector.FillRect(dst, sx-0.5, sy-0.5, 1, 1, dotColor, false)
			}
		}
	}

	ox, oy := view.Project(originX, originY)
	vector.StrokeLine(dst, float32(region.Min.X), oy, float32(region.Max.X), oy, 1, axisColor, false)
	vector.StrokeLine(dst, ox, float32(region.Min.Y), ox, float32(region.Max.Y), 1, axisColor, false)
}
