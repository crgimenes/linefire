package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	crosshairSize      = 8.0   // half-length of the crosshair arms, logical px
	crosshairWorldDist = 240.0 // how far ahead the locked-aim reticle sits, world units
)

var crosshairColor = color.RGBA{0xff, 0xc8, 0xff, 0xff}

// toggleAimMode switches between screen-relative aim (the cursor's world point)
// and world-locked aim (the direction is frozen in world space, so rotating the
// ship keeps the turret on the same heading). Locking captures the current
// heading so the toggle is seamless.
func (g *Game) toggleAimMode() {
	g.aimLocked = !g.aimLocked
	if g.aimLocked {
		g.aimRefAngle = g.angle
	}
}

// aimAngleForCamera is the heading used to unproject the cursor: the live heading
// in screen mode, or the frozen heading in world-locked mode (so ship rotation
// no longer swings the aim).
func (g *Game) aimAngleForCamera() float64 {
	if g.aimLocked {
		return g.aimRefAngle
	}
	return g.angle
}

// aimDir is the unit world-space direction from the ship to the mouse cursor.
func (g *Game) aimDir() (float64, float64, bool) {
	mx, my := ebiten.CursorPosition()
	return g.aimDirFrom(float64(mx), float64(my))
}

// aimDirFrom converts a screen position to a unit world-space direction from the
// ship by inverting the camera transform. In world-locked mode it inverts using
// the frozen heading, so the direction does not change as the ship turns. Pure
// (no live input) so it is testable.
func (g *Game) aimDirFrom(mx, my float64) (float64, float64, bool) {
	cam := g.cameraGeoMAt(g.x, g.y, g.aimAngleForCamera())
	if !cam.IsInvertible() {
		return 0, 0, false
	}
	cam.Invert()
	wx, wy := cam.Apply(mx, my)
	dx, dy := wx-g.x, wy-g.y
	d := math.Hypot(dx, dy)
	if d == 0 {
		return 0, 0, false
	}
	return dx / d, dy / d, true
}

// crosshairPos is the screen position of the aim reticle: the cursor in screen
// mode, or the projected aim direction (current camera) in world-locked mode.
func (g *Game) crosshairPos() (float32, float32) {
	if g.aimLocked {
		dx, dy, ok := g.aimDir()
		if ok {
			cam := g.cameraGeoM() // current heading, so the reticle rotates with the world
			px, py := cam.Apply(g.x+dx*crosshairWorldDist, g.y+dy*crosshairWorldDist)
			return float32(px), float32(py)
		}
	}
	mxi, myi := ebiten.CursorPosition()
	return float32(mxi), float32(myi)
}

// drawCrosshair draws a small cross at the aim reticle. In screen mode it sits
// at the cursor; in world-locked mode it sits at the projected aim direction, so
// it rotates with the world and shows where the bolts actually go.
func (g *Game) drawCrosshair(screen *ebiten.Image) {
	mx, my := g.crosshairPos()
	s := float32(crosshairSize * g.dpr)
	w := float32(1.5 * g.dpr)
	vector.StrokeLine(screen, mx-s, my, mx-s/3, my, w, crosshairColor, true)
	vector.StrokeLine(screen, mx+s/3, my, mx+s, my, w, crosshairColor, true)
	vector.StrokeLine(screen, mx, my-s, mx, my-s/3, w, crosshairColor, true)
	vector.StrokeLine(screen, mx, my+s/3, mx, my+s, w, crosshairColor, true)
}
