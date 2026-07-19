package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// The touch affordances stay invisible until the first finger lands (a desktop never
// sees them) and even then only the LIVE stick draws — floating sticks have no home
// position, so an idle screen shows nothing but the game.

var (
	stickRingColor   = color.RGBA{0x70, 0xe0, 0xff, 0x38} // faint CRT cyan base ring
	stickKnobColor   = color.RGBA{0x70, 0xe0, 0xff, 0x70} // brighter knob
	stickTurretColor = color.RGBA{0xd0, 0x80, 0xff, 0x80} // knob once the turret is engaged (its violet)
	pauseSpotColor   = color.RGBA{0x70, 0xe0, 0xff, 0x46} // the two pause bars in the corner
)

// drawTouchSticks draws the live thumb sticks and the pause hotspot. Called on the
// final screen (layout space), over the HUD.
func (g *Game) drawTouchSticks(screen *ebiten.Image) {
	tc := &touchPad
	if !tc.seen || g.paused || g.creditsMode || g.over || g.gameWon {
		return
	}
	g.drawStick(screen, &tc.left, stickKnobColor)
	knob := stickKnobColor
	if tc.turret {
		knob = stickTurretColor
	}
	g.drawStick(screen, &tc.right, knob)
	g.drawPauseSpot(screen)
}

// drawStick draws one thumb: the base ring where the finger landed and the knob at
// the (clamped) deflection.
func (g *Game) drawStick(screen *ebiten.Image, t *thumb, knob color.RGBA) {
	if !t.active {
		return
	}
	ring := stickRingPx * g.dpr
	vector.StrokeCircle(screen, float32(t.ox), float32(t.oy), float32(ring), 1.5, stickRingColor, true)

	dx, dy := t.deflection()
	d := math.Hypot(dx, dy)
	if d > ring {
		dx, dy = dx/d*ring, dy/d*ring // the knob rides the rim when the finger overshoots
	}
	vector.FillCircle(screen, float32(t.ox+dx), float32(t.oy+dy), float32(stickKnobPx*g.dpr), knob, true)
}

// drawPauseSpot marks the pause hotspot with two small bars at the top center,
// clear of the HUD panel (top-left) and the minimap (top-right).
func (g *Game) drawPauseSpot(screen *ebiten.Image) {
	s := pauseSpotPx * g.dpr
	cx := float64(g.sw) / 2
	bw, bh := s*0.10, s*0.34
	y := (s - bh) / 2
	vector.FillRect(screen, float32(cx-bw*1.5), float32(y), float32(bw), float32(bh), pauseSpotColor, false)
	vector.FillRect(screen, float32(cx+bw*0.5), float32(y), float32(bw), float32(bh), pauseSpotColor, false)
}
