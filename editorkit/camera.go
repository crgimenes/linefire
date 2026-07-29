// Package editorkit holds editor primitives shared by the asset editor and the
// map editor: a pan/zoom camera, a dot-grid renderer and a generic undo/redo
// history. It is asset-agnostic; it only depends on render for the View type.
package editorkit

import (
	"image"
	"math"

	"github.com/crgimenes/linefire/render"
)

// Zoom limits and framing margin (in screen pixels of scale).
const (
	MinScale  = 0.5
	MaxScale  = 64.0
	FitMargin = 24.0
)

// Camera maps asset/world space to screen via a render.View, with zoom, pan and
// fit-to-box helpers. MinScale/MaxScale override the package defaults when set
// (non-zero), letting an unbounded map zoom out much further than an asset.
type Camera struct {
	View     render.View
	MinScale float64
	MaxScale float64
}

// clamp keeps a scale within this camera's range (package defaults when unset).
func (c *Camera) clamp(s float64) float64 {
	mn, mx := c.MinScale, c.MaxScale
	if mn <= 0 {
		mn = MinScale
	}
	if mx <= 0 {
		mx = MaxScale
	}
	if s < mn {
		return mn
	}
	if s > mx {
		return mx
	}
	return s
}

// ZoomAt multiplies the scale by factor while keeping the world point under the
// screen position (mx, my) fixed.
func (c *Camera) ZoomAt(mx, my, factor float64) {
	ax, ay := c.View.Unproject(mx, my)
	scale := c.clamp(c.View.Scale * factor)
	c.View.Scale = scale
	c.View.OffsetX = mx - ax*scale
	c.View.OffsetY = my - ay*scale
}

// Pan shifts the view by a screen-space delta.
func (c *Camera) Pan(dx, dy float64) {
	c.View.OffsetX += dx
	c.View.OffsetY += dy
}

// FitBox frames the world-space box [minX,minY]-[maxX,maxY] centered within the
// screen region, leaving a margin.
func (c *Camera) FitBox(region image.Rectangle, minX, minY, maxX, maxY float64) {
	bw := maxX - minX
	bh := maxY - minY
	if bw <= 0 {
		bw = 1
	}
	if bh <= 0 {
		bh = 1
	}
	sx := (float64(region.Dx()) - 2*FitMargin) / bw
	sy := (float64(region.Dy()) - 2*FitMargin) / bh
	scale := c.clamp(math.Min(sx, sy))

	centerXWorld := (minX + maxX) / 2
	centerYWorld := (minY + maxY) / 2
	centerXScreen := float64(region.Min.X) + float64(region.Dx())/2
	centerYScreen := float64(region.Min.Y) + float64(region.Dy())/2

	c.View = render.View{
		OffsetX: centerXScreen - centerXWorld*scale,
		OffsetY: centerYScreen - centerYWorld*scale,
		Scale:   scale,
	}
}

// ClampScale keeps a scale within the allowed zoom range.
func ClampScale(s float64) float64 {
	if s < MinScale {
		return MinScale
	}
	if s > MaxScale {
		return MaxScale
	}
	return s
}
