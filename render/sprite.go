package render

import (
	"image/color"
	"math"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// Sprite primitives for the high-volume draws (particles, bullet streaks, portal
// motes). Each used to be an anti-aliased vector path, tessellated on the CPU on
// EVERY call — up to a few thousand per frame with a busy particle pool, twice
// (crisp + glow passes). Instead a tiny white texture is built once and each draw
// is a tinted, transformed quad: no per-call tessellation, and ebiten batches
// consecutive quads from the same source into one GPU draw.
//
// The textures are white with premultiplied alpha and a baked ~1.5-texel feather,
// so the linear filter gives an anti-aliased edge at any rotation. These sprites
// are for small, fading, fast-moving marks: the feather scales with the quad, so
// the bigger the draw the softer the edge — fine for glow-adjacent effects, wrong
// for big crisp geometry (that stays on the vector path renderer).

const (
	discTexR      = 12.0 // disc radius in texels (image is 2*(R+pad) square)
	lineTexL      = 24.0 // line body length in texels
	lineTexW      = 8.0  // line body width in texels
	spriteFeather = 1.5  // alpha ramp width in texels at the shape's edge
	spritePad     = 2.0  // transparent border so the linear filter never bleeds
)

var (
	spriteOnce sync.Once
	discTex    *ebiten.Image // soft-edged white disc
	lineTex    *ebiten.Image // feathered white bar along +X, butt-capped like StrokeLine
)

// buildSprites rasterizes the primitives on the CPU (coverage -> premultiplied
// white) and uploads them once.
func buildSprites() {
	spriteOnce.Do(func() {
		discTex = buildAlphaTex(int(2*(discTexR+spritePad)), int(2*(discTexR+spritePad)), func(x, y float64) float64 {
			c := discTexR + spritePad
			return math.Hypot(x-c, y-c) - discTexR // signed distance to the disc edge
		})
		lineTex = buildAlphaTex(int(lineTexL+2*spritePad), int(lineTexW+2*spritePad), func(x, y float64) float64 {
			dx := math.Max(spritePad-x, x-(spritePad+lineTexL))
			dy := math.Max(spritePad-y, y-(spritePad+lineTexW))
			return math.Max(dx, dy) // signed distance to the bar's rectangle
		})
	})
}

// buildAlphaTex fills a w×h image where dist (texel center -> signed distance to
// the shape edge, negative inside) is mapped through a feather-wide alpha ramp.
func buildAlphaTex(w, h int, dist func(x, y float64) float64) *ebiten.Image {
	pix := make([]byte, w*h*4)
	for y := range h {
		for x := range w {
			d := dist(float64(x)+0.5, float64(y)+0.5)
			a := 1 - (d/spriteFeather + 0.5) // 1 inside, 0 outside, ramp across the edge
			if a > 1 {
				a = 1
			}
			if a < 0 {
				a = 0
			}
			v := byte(a*255 + 0.5)
			i := (y*w + x) * 4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = v, v, v, v // premultiplied white
		}
	}
	img := ebiten.NewImage(w, h)
	img.WritePixels(pix)
	return img
}

// FillCircle draws a tinted disc of radius r (device px) centered at (cx, cy).
func FillCircle(dst *ebiten.Image, cx, cy, r float64, col color.RGBA) {
	disc(dst, cx, cy, r, col, 1, ebiten.Blend{})
}

// FillCircleAdd is FillCircle with ADDITIVE blending, scaled by gain — the same
// light-adding draw the bolts use. Note that a caller fades an additive mark with
// gain, NOT with the color's alpha: additive blending ignores the destination, so
// a lower alpha alone would leave the mark just as bright.
func FillCircleAdd(dst *ebiten.Image, cx, cy, r float64, col color.RGBA, gain float64) {
	disc(dst, cx, cy, r, col, gain, ebiten.BlendLighter)
}

// disc is the shared body behind both: one tinted, scaled quad of the disc sprite.
func disc(dst *ebiten.Image, cx, cy, r float64, col color.RGBA, gain float64, blend ebiten.Blend) {
	if r <= 0 {
		return
	}
	buildSprites()
	s := r / discTexR
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear, Blend: blend}
	op.GeoM.Translate(-(discTexR + spritePad), -(discTexR + spritePad))
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(cx, cy)
	op.ColorScale.ScaleWithColor(col)
	if gain != 1 {
		f := float32(gain)
		op.ColorScale.Scale(f, f, f, f)
	}
	dst.DrawImage(discTex, op)
}

// StrokeLine draws a tinted straight streak of the given width (device px) from
// (x0, y0) to (x1, y1), butt-capped like the vector StrokeLine it replaces.
func StrokeLine(dst *ebiten.Image, x0, y0, x1, y1, width float64, col color.RGBA) {
	streak(dst, x0, y0, x1, y1, width, col, 1, ebiten.Blend{})
}

// StrokeLineAdd is StrokeLine with ADDITIVE blending, scaled by gain: the streak
// adds light to whatever lies under it instead of covering it. That is how kutta
// composites its glowing smoke, and it is what makes a mark read as hot — the
// same stroke drawn over-and-over blows out to white where it overlaps.
func StrokeLineAdd(dst *ebiten.Image, x0, y0, x1, y1, width float64, col color.RGBA, gain float64) {
	streak(dst, x0, y0, x1, y1, width, col, gain, ebiten.BlendLighter)
}

// streak is the shared body behind both: one tinted, transformed quad of the line
// sprite.
func streak(dst *ebiten.Image, x0, y0, x1, y1, width float64, col color.RGBA, gain float64, blend ebiten.Blend) {
	dx, dy := x1-x0, y1-y0
	length := math.Hypot(dx, dy)
	if length == 0 || width <= 0 {
		return
	}
	buildSprites()
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear, Blend: blend}
	op.GeoM.Translate(-spritePad, -spritePad-lineTexW/2) // origin at the bar's start, on its axis
	op.GeoM.Scale(length/lineTexL, width/lineTexW)
	op.GeoM.Rotate(math.Atan2(dy, dx))
	op.GeoM.Translate(x0, y0)
	op.ColorScale.ScaleWithColor(col)
	if gain != 1 {
		f := float32(gain)
		op.ColorScale.Scale(f, f, f, f)
	}
	dst.DrawImage(lineTex, op)
}
