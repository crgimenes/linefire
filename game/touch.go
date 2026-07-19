package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Touch controls: the whole game on two thumbs, for phones and tablets (the web
// build). Each half of the screen is one FLOATING stick — the base is wherever the
// finger lands, so there is no fixed gadget to look for:
//
//   - LEFT thumb: steering. The deflection is a SCREEN direction; the ship always
//     points screen-up, so pushing where you want to go turns the ship there (the
//     same turnToward/turnSpeed as the keys) and thrusts while deflected. Because
//     the world rotates around the ship, flying toward the goal settles the stick
//     onto "up" — screen-relative steering self-stabilizes.
//   - RIGHT thumb: fire. Held near its origin it fires the forward NOSE gun (slot
//     0); dragged past turretThresholdPx it becomes the TURRET stick for the rest
//     of the touch — the drag direction, unprojected through the camera, aims slot
//     1, which fires while the finger is down. A tap is a shot; a sweep is aimed
//     turret fire.
//
// Keyboard and mouse stay fully live — the touch layer is additive, a no-op until
// a finger actually lands. Touch state is a PROCESS-level singleton (like minigui's
// pointer): a portal warp rebuilds the Game struct mid-drag, and the thumbs must
// survive that reset without the player lifting a finger.

const (
	touchDeadzonePx   = 14.0 // logical px of deflection before a stick registers
	turretThresholdPx = 40.0 // deflection that flips the right thumb from nose fire to turret aim
	touchAimReachPx   = 220. // how far from the ship the synthetic turret "cursor" sits
	stickRingPx       = 46.0 // stick base ring radius (drawn), logical px
	stickKnobPx       = 14.0 // stick knob radius (drawn), logical px
	pauseSpotPx       = 52.0 // side of the square pause hotspot in the top-left corner, logical px
)

// thumb is one finger riding one half of the screen.
type thumb struct {
	active bool
	just   bool // claimed this frame (justPressed semantics for discrete weapons)
	id     ebiten.TouchID
	ox, oy float64 // where the finger landed (the stick base), layout px
	x, y   float64 // where the finger is now
}

// dx, dy is the thumb's deflection from its base.
func (t *thumb) deflection() (float64, float64) { return t.x - t.ox, t.y - t.oy }

// touchCtl is the full touch-control state. Package-level (not on Game) so thumbs
// survive the *g = *New(...) world resets of portals, restarts and the attract regen.
type touchCtl struct {
	seen       bool  // a touch has ever happened: draw the touch affordances
	left       thumb // steering
	right      thumb // nose fire / turret aim
	turret     bool  // the right thumb crossed the threshold: turret mode until release
	turretJust bool  // turret mode began this frame (justPressed semantics downstream)
}

// touchPad is the live singleton behind the exported hooks.
var touchPad touchCtl

// steerDelta converts a stick deflection (screen space, +y down) into the heading
// change it commands, in degrees relative to the ship's current heading: screen-up
// (the ship's nose) is zero, right is +90. Pure, so the mapping is testable.
func steerDelta(dx, dy float64) float64 {
	return normDeg(math.Atan2(dy, dx)*180/math.Pi + 90)
}

// claimThumb assigns a fresh touch to the half its ORIGIN is in (crossing the middle
// mid-drag never re-assigns), refusing halves that already hold a finger. Returns the
// claimed thumb or nil.
func (tc *touchCtl) claimThumb(id ebiten.TouchID, x, y, midX float64) *thumb {
	side := &tc.left
	if x >= midX {
		side = &tc.right
	}
	if side.active {
		return nil
	}
	*side = thumb{active: true, just: true, id: id, ox: x, oy: y, x: x, y: y}
	if side == &tc.right {
		tc.turret = false // each right touch starts as nose fire until it sweeps
	}
	return side
}

// updateTouch claims new fingers, tracks the live ones, and releases the lifted
// ones. Runs every Update frame, before input is read.
func (g *Game) updateTouch() {
	tc := &touchPad
	tc.turretJust = false
	tc.left.just, tc.right.just = false, false

	for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
		tc.seen = true
		xi, yi := ebiten.TouchPosition(id)
		x, y := float64(xi), float64(yi)
		if g.touchPauseSpot(x, y) {
			continue // the pause hotspot ate this touch; it steers nothing
		}
		if tc.claimThumb(id, x, y, float64(g.sw)/2) == nil && tc.left.active && tc.right.active {
			g.debugHUD = !g.debugHUD // a THIRD finger with both thumbs down: the touch F3 (FPS/scale readout)
		}
	}

	refresh := func(t *thumb) {
		if !t.active {
			return
		}
		if inpututil.IsTouchJustReleased(t.id) {
			t.active = false
			return
		}
		xi, yi := ebiten.TouchPosition(t.id)
		t.x, t.y = float64(xi), float64(yi)
	}
	refresh(&tc.left)
	refresh(&tc.right)

	if tc.right.active && !tc.turret {
		dx, dy := tc.right.deflection()
		if math.Hypot(dx, dy) > turretThresholdPx*g.dpr {
			tc.turret, tc.turretJust = true, true
		}
	}
	if !tc.right.active {
		tc.turret = false
	}
}

// touchPauseSpot reports whether a fresh touch landed on the pause hotspot (top
// CENTER — the HUD owns the top-left and the minimap the top-right) and opens the
// menu if so. Only live during actual play — the menu, the attract screens and the
// automap handle their own input.
func (g *Game) touchPauseSpot(x, y float64) bool {
	if g.paused || g.creditsMode || g.over || g.gameWon || g.mapOpen {
		return false
	}
	side := pauseSpotPx * g.dpr
	cx := float64(g.sw) / 2
	if y > side || x < cx-side/2 || x > cx+side/2 {
		return false
	}
	g.paused = true
	return true
}

// applyTouchControls drives steering, thrust and the two fire modes from the live
// thumbs. Returns whether the ship is thrusting. Called from readManualInput, so
// the keyboard path and this one stack rather than compete.
func (g *Game) applyTouchControls() bool {
	tc := &touchPad
	thrusting := false

	if tc.left.active {
		dx, dy := tc.left.deflection()
		if math.Hypot(dx, dy) > touchDeadzonePx*g.dpr {
			g.angle = turnToward(g.angle, g.angle+steerDelta(dx, dy), turnSpeed)
			fx, fy, _, _ := g.headingDirs()
			g.vx += fx * thrust
			g.vy += fy * thrust
			g.emitThruster(fx, fy)
			thrusting = true
		}
	}

	if tc.right.active {
		if tc.turret {
			if !g.fireBlocked {
				g.fireSlotInput(1, tc.turretJust) // the aimed turret; aim comes from pointerPos
			}
		} else {
			g.fireSlotInput(0, tc.right.just) // nose gun, straight ahead
		}
	}

	return thrusting
}

// touchAimPoint is the synthetic "cursor" the turret stick aims through: a point at
// touchAimReachPx from the ship (screen center) along the stick's deflection. ok is
// false when the right thumb is not in turret mode, so callers fall back to the mouse.
func (g *Game) touchAimPoint() (float64, float64, bool) {
	tc := &touchPad
	if !tc.right.active || !tc.turret {
		return 0, 0, false
	}
	dx, dy := tc.right.deflection()
	d := math.Hypot(dx, dy)
	if d == 0 {
		return 0, 0, false
	}
	cx, cy := float64(g.sw)/2, float64(g.sh)/2
	r := touchAimReachPx * g.dpr
	return cx + dx/d*r, cy + dy/d*r, true
}

// pointerPos is where the player is pointing: the turret stick's synthetic cursor
// while it is engaged, else the OS mouse cursor. The aim unproject and the crosshair
// both read this, so the reticle always shows where the turret will actually shoot.
func (g *Game) pointerPos() (float64, float64) {
	if x, y, ok := g.touchAimPoint(); ok {
		return x, y
	}
	mxi, myi := ebiten.CursorPosition()
	return float64(mxi), float64(myi)
}

// touchJustTapped reports a fresh touch this frame — the touch equivalent of "any
// key": start on the title, restart on game over, continue on victory.
func touchJustTapped() bool {
	return len(inpututil.AppendJustPressedTouchIDs(nil)) > 0
}
