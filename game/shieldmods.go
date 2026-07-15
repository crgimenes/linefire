package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Shield mods are timed defensive combat mods, distinct from the flat g.shield HP
// buffer. A BUBBLE grants full damage immunity for a while (a panic button to push
// through fire); a REFLECTOR turns incoming enemy shots into player shots for a
// while (offensive defense). Both are fly-over pickups that ADD time up to a cap,
// carried across maps and reset on restart (until checkpoints — see T24). They are
// timers, so they run down on their own — no menu, no slot.

const (
	bubbleAddFrames  = 360  // ~6s of immunity granted per bubble pickup
	maxBubbleTime    = 1080 // cap (~18s stored)
	reflectAddFrames = 360  // ~6s of reflection granted per reflector pickup
	maxReflectTime   = 1080 // cap

	reflectSpeed    = 9.0   // world units/frame for a reflected shot
	reflectDamage   = 2     // hull damage a reflected shot deals
	reflectAimRange = 520.0 // a reflected shot seeks the nearest enemy within this range
)

var (
	reflectColor     = color.RGBA{0xff, 0xf0, 0xff, 0xff} // reflected shot core: bright near-white
	reflectGlowColor = color.RGBA{0xc0, 0x80, 0xff, 0xff} // reflected shot halo: violet
	bubbleRingColor  = color.RGBA{0x80, 0xf0, 0xff, 0xff} // bubble bubble: cyan-white
	reflectRingColor = color.RGBA{0xc0, 0x80, 0xff, 0xff} // reflector ring: violet
)

// addBubble grants (more) immunity time up to the cap, narrating it. Reports whether
// it actually rose.
func (g *Game) addBubble() bool {
	if g.bubbleTime >= maxBubbleTime {
		g.logf("SHIELD  maxed")
		return false
	}
	g.bubbleTime = min(g.bubbleTime+bubbleAddFrames, maxBubbleTime)
	g.logf("SHIELD  bubble %ds", g.bubbleTime/60)
	return true
}

// addReflect grants (more) reflection time up to the cap, narrating it.
func (g *Game) addReflect() bool {
	if g.reflectTime >= maxReflectTime {
		g.logf("REFLECTOR  maxed")
		return false
	}
	g.reflectTime = min(g.reflectTime+reflectAddFrames, maxReflectTime)
	g.logf("REFLECTOR  %ds", g.reflectTime/60)
	return true
}

// stepShieldMods runs the timed shields down one frame each.
func (g *Game) stepShieldMods() {
	if g.bubbleTime > 0 {
		g.bubbleTime--
	}
	if g.reflectTime > 0 {
		g.reflectTime--
	}
}

// reflectShot converts an incoming enemy shot into a player shot: it seeks the
// nearest enemy in range, else flies straight back the way it came. Sparks and a
// small kick sell the deflection.
func (g *Game) reflectShot(p *projectile) {
	vx, vy := -p.vx, -p.vy // default: straight back
	if tx, ty, ok := g.nearestEnemyFrom(p.x, p.y, reflectAimRange); ok {
		dx, dy := tx-p.x, ty-p.y
		if d := math.Hypot(dx, dy); d > 0 {
			vx, vy = dx/d*reflectSpeed, dy/d*reflectSpeed
		}
	}
	g.projectiles = append(g.projectiles, projectile{
		x: p.x, y: p.y, px: p.x, py: p.y,
		vx: vx, vy: vy,
		life: bulletLife, dmg: reflectDamage, col: damageColor,
		rcol: reflectColor, rglow: reflectGlowColor, width: bulletWidth, glowW: bulletGlowWidth,
	})
	g.emitBurst(p.x, p.y, shieldSparks)
	g.addShake(0.6)
}

// drawTimedShields draws the bubble and reflector rings at the ship center (the ship
// is fixed there) while each is active, pulsing so they read as energized.
func (g *Game) drawTimedShields(screen *ebiten.Image) {
	if g.bubbleTime <= 0 && g.reflectTime <= 0 {
		return
	}
	w, h := g.screenSize()
	cx, cy := w/2+g.shakeX, h/2+g.shakeY
	pulse := 0.6 + 0.4*math.Sin(float64(ebiten.Tick())/4)
	if g.bubbleTime > 0 {
		r := (g.radius + 12) * g.camPixelScale()
		c := bubbleRingColor
		c.A = uint8(0x60 + 0x80*pulse*fadeNear(g.bubbleTime))
		vector.StrokeCircle(screen, float32(cx), float32(cy), float32(r), float32(3*g.dpr), c, true)
	}
	if g.reflectTime > 0 {
		r := (g.radius + 16) * g.camPixelScale()
		c := reflectRingColor
		c.A = uint8(0x60 + 0x80*pulse*fadeNear(g.reflectTime))
		vector.StrokeCircle(screen, float32(cx), float32(cy), float32(r), float32(2*g.dpr), c, true)
	}
}

// fadeNear returns a 0..1 factor that dips as a timer nears expiry, so a shield
// visibly flickers out in its last second instead of vanishing abruptly.
func fadeNear(frames int) float64 {
	if frames >= 60 {
		return 1
	}
	return float64(frames) / 60
}
