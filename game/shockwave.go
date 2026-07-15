package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	maxShocks  = 12  // hard cap on live shockwave rings
	shockLife  = 24  // frames an expanding ring lives (~0.4s at 60 TPS)
	shockWidth = 2.0 // ring stroke width, logical px (scaled by DPI)
)

var shockColor = color.RGBA{0xff, 0xc0, 0x60, 0xff} // warm expanding blast ring

// shockwave is an expanding ring drawn at an explosion, growing from 0 to
// maxRadius (in world units) over its life while fading out.
type shockwave struct {
	x, y      float64
	maxRadius float64
	life      int
	maxLife   int
}

// radius grows from 0 to maxRadius over the ring's life, easing out (fast then
// slow) so the blast snaps open and settles.
func (s shockwave) radius() float64 {
	t := 1 - float64(s.life)/float64(s.maxLife) // 0 -> 1 over life
	return s.maxRadius * (1 - (1-t)*(1-t))      // ease-out quadratic
}

// spawnShockwave rings out a blast at (x, y) reaching maxRadius, respecting the cap.
func (g *Game) spawnShockwave(x, y, maxRadius float64) {
	if len(g.shocks) >= maxShocks {
		return
	}
	g.shocks = append(g.shocks, shockwave{x: x, y: y, maxRadius: maxRadius, life: shockLife, maxLife: shockLife})
}

// stepShockwaves ages the rings, dropping the finished ones. Pure, so it is cheap
// and unit-testable without a graphics context.
func (g *Game) stepShockwaves() {
	kept := g.shocks[:0]
	for i := range g.shocks {
		s := g.shocks[i]
		s.life--
		if s.life <= 0 {
			continue
		}
		kept = append(kept, s)
	}
	g.shocks = kept
}

// drawShockwaves strokes each ring at its current radius, fading with its life.
func (g *Game) drawShockwaves(dst *ebiten.Image, cam ebiten.GeoM) {
	for i := range g.shocks {
		s := &g.shocks[i]
		frac := float64(s.life) / float64(s.maxLife)
		c := shockColor
		c.A = uint8(float64(shockColor.A) * frac)
		cx, cy := cam.Apply(s.x, s.y)
		r := s.radius() * g.camPixelScale()
		vector.StrokeCircle(dst, float32(cx), float32(cy), float32(r), float32(shockWidth*g.dpr), c, true)
	}
}
