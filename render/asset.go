// Package render draws Linefire assets with the ebiten vector package. The same
// code is used by the editor canvas (zoomed) and the real-size preview, and is
// intended to be reusable by the future game.
package render

import (
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"linefire/asset"
)

// View describes how asset-space coordinates map to screen pixels: a uniform
// scale plus a translation. ScreenX = OffsetX + assetX*Scale. It is used by the
// editors (small assets, CPU transform). The game instead caches world geometry
// once and applies the camera to visible batches before drawing (see mesh.go).
type View struct {
	OffsetX float64
	OffsetY float64
	Scale   float64
}

// Project converts an asset-space coordinate to screen pixels.
func (v View) Project(x, y float64) (float32, float32) {
	return float32(v.OffsetX + x*v.Scale), float32(v.OffsetY + y*v.Scale)
}

// Unproject converts a screen pixel back to an asset-space coordinate.
func (v View) Unproject(sx, sy float64) (float64, float64) {
	return (sx - v.OffsetX) / v.Scale, (sy - v.OffsetY) / v.Scale
}

// DrawAsset renders every layer of the asset into dst using the given view.
// Filled shapes are drawn first, then strokes on top. It draws the crisp
// geometry only; the additive bloom that uses the per-layer glow field is
// applied separately by Glow (see glow.go).
func DrawAsset(dst *ebiten.Image, a *asset.Asset, v View, antialias bool) {
	DrawLayers(dst, a.Layers, v, antialias)
}

// DrawLayers renders a slice of styled layers (used for both assets and map
// walls, which share the layer format).
func DrawLayers(dst *ebiten.Image, layers []asset.Layer, v View, antialias bool) {
	for i := range layers {
		layer := &layers[i]
		if layer.Hidden {
			continue
		}
		fillCol, fillVisible := ParseColor(layer.Fill)
		strokeCol, strokeVisible := ParseColor(layer.Stroke)

		for _, p := range layer.Paths {
			path := buildPath(p, v)

			// Only closed paths are filled; open paths render as plain lines.
			if fillVisible && p.Closed() {
				var cs ebiten.ColorScale
				cs.ScaleWithColor(fillCol)
				vector.FillPath(dst, path, &vector.FillOptions{}, &vector.DrawPathOptions{
					AntiAlias:  antialias,
					ColorScale: cs,
				})
			}

			if strokeVisible && layer.StrokeWidth > 0 {
				var cs ebiten.ColorScale
				cs.ScaleWithColor(strokeCol)
				vector.StrokePath(dst, path, &vector.StrokeOptions{
					Width:    float32(layer.StrokeWidth * v.Scale),
					LineCap:  vector.LineCapRound,
					LineJoin: vector.LineJoinRound,
				}, &vector.DrawPathOptions{
					AntiAlias:  antialias,
					ColorScale: cs,
				})
			}
		}
	}
}

// buildPath converts an asset path into an ebiten vector path in screen space.
func buildPath(p asset.Path, v View) *vector.Path {
	p = p.Flatten() // expand any Bézier curve to line segments
	path := &vector.Path{}
	started := false
	for _, c := range p.Commands {
		switch c.Op {
		case asset.OpMoveTo:
			sx, sy := v.Project(c.X, c.Y)
			path.MoveTo(sx, sy)
			started = true
		case asset.OpLineTo:
			sx, sy := v.Project(c.X, c.Y)
			if !started {
				path.MoveTo(sx, sy)
				started = true
				continue
			}
			path.LineTo(sx, sy)
		case asset.OpClose:
			path.Close()
		}
	}
	return path
}

// ParseColor parses a color string. It accepts "#rgb", "#rrggbb" and
// "#rrggbbaa". An empty string or "transparent" reports visible == false so the
// caller can skip drawing that stroke or fill.
func ParseColor(s string) (col color.RGBA, visible bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "transparent" || s == "none" {
		return color.RGBA{}, false
	}
	if !strings.HasPrefix(s, "#") {
		return color.RGBA{}, false
	}
	hex := s[1:]

	switch len(hex) {
	case 3: // #rgb -> #rrggbb
		r, okR := hexVal(hex[0])
		g, okG := hexVal(hex[1])
		b, okB := hexVal(hex[2])
		if !okR || !okG || !okB {
			return color.RGBA{}, false
		}
		return color.RGBA{r*16 + r, g*16 + g, b*16 + b, 0xff}, true
	case 6, 8:
		r, okR := hexByte(hex[0:2])
		g, okG := hexByte(hex[2:4])
		b, okB := hexByte(hex[4:6])
		if !okR || !okG || !okB {
			return color.RGBA{}, false
		}
		a := uint8(0xff)
		if len(hex) == 8 {
			var okA bool
			a, okA = hexByte(hex[6:8])
			if !okA {
				return color.RGBA{}, false
			}
		}
		return color.RGBA{r, g, b, a}, true
	default:
		return color.RGBA{}, false
	}
}

// hexVal returns the value of a single hex digit (0-15) and whether it was a
// valid digit.
func hexVal(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	default:
		return 0, false
	}
}

// hexByte returns the value of a two-digit hex string (0-255) and whether both
// digits were valid.
func hexByte(s string) (uint8, bool) {
	hi, okHi := hexVal(s[0])
	lo, okLo := hexVal(s[1])
	if !okHi || !okLo {
		return 0, false
	}
	return hi*16 + lo, true
}
