package game

import (
	"image/color"
	"math"

	"github.com/crgimenes/linefire/render"
	"github.com/crgimenes/linefire/weapon"
	"github.com/hajimehoshi/ebiten/v2"
)

const (
	// Heat: firing the beam builds heat each frame; at the cap it shuts off until
	// fully cooled (hysteresis), so parking the laser on auto-fire has a rhythm —
	// roughly 4s of burn, 2s of forced cooldown.
	laserHeatMax  = 240
	laserCoolRate = 2
	// The beam MELTS rock instead of biting it: a small nibble on every damage tick
	// (15/s) rather than one big crater. Slow to breach a wall, but it never stops —
	// hold it long enough and it bores a clean bore-hole.
	laserDigRadius = 3.5
	laserWidth     = 2.4 // crisp core stroke, logical px (scaled by DPI)
	laserGlowWidth = 7.0 // wide glow stroke feeding the bloom

	// The beam SHOWS its own heat instead of being explained: as the emitter loads up
	// it thins, its halo collapses and it dims, so by the time it cuts out the player
	// has been watching it fail for a second. These are the values at full heat, as a
	// fraction of a cold beam.
	laserThinMin = 0.35
	laserGlowMin = 0.25
	laserDimMin  = 0.45
)

var (
	laserColor     = color.RGBA{0xe0, 0xff, 0xe0, 0xff} // bright green-white core
	laserGlowColor = color.RGBA{0x40, 0xff, 0x60, 0xff} // green halo
)

// stepLaserHeat runs the beam's thermal model once per frame: firing heats, rest
// cools, and hitting the cap latches the laser off until it is stone cold.
func (g *Game) stepLaserHeat() {
	if g.laserOn {
		g.laserHeat++
		if g.laserHeat >= laserHeatMax && !g.laserHot {
			g.laserHot = true
			// No log line: the beam has been visibly dying for a second, and it goes
			// out with a falling whistle and a puff of vapour at the tip. Saying
			// "overheated" would only explain what the player just watched happen.
			g.emitBurst(g.laserX1, g.laserY1, beamCutout)
			g.sfx.play(soundReq{"laser_cutout", 909, 0.5})
		}
		return
	}
	g.laserHeat -= laserCoolRate
	if g.laserHeat > 0 {
		return
	}
	g.laserHeat = 0
	g.laserHot = false
}

// aimLaser computes the beam from the ship, clipped at the first wall (or the max
// range), and flags it for drawing this frame. aim picks the direction: forward
// along the heading (a key-bound mount) or toward the cursor/target (a mouse mount).
// Called every frame the laser is held, so the beam is continuous.
func (g *Game) aimLaser(w *weapon.Weapon, aim weapon.Aim) {
	var dx, dy float64
	if aim == weapon.AimCursor {
		adx, ady, ok := g.weaponAimDir()
		if !ok {
			return
		}
		dx, dy = adx, ady
	} else {
		rad := g.angle * math.Pi / 180
		dx, dy = math.Cos(rad), math.Sin(rad)
	}
	g.laserX1, g.laserY1, g.laserHitRock = g.laserEndpoint(g.x, g.y, dx, dy, w.Reach)
	g.laserOn = true
}

// laserEndpoint returns where a unit ray from (x,y) along (dx,dy) ends, and whether it
// terminated ON ROCK (rather than fading out at max range). With a region field the
// rock is the truth, so the beam shines straight down a tunnel the player bored; a
// procedural map has no field and clips on the wall segments.
func (g *Game) laserEndpoint(x, y, dx, dy, reach float64) (float64, float64, bool) {
	f := g.flood
	if f == nil {
		best := reach
		hit := false
		for _, s := range g.segs {
			t, ok := rayHitsSegment(x, y, dx, dy, s)
			if ok && t < best {
				best, hit = t, true
			}
		}
		return x + dx*best, y + dy*best, hit
	}
	step := f.cell * 0.5
	for t := 0.0; t <= reach; t += step {
		px, py := x+dx*t, y+dy*t
		if f.rockAt(px, py) {
			return px, py, true
		}
	}
	return x + dx*reach, y + dy*reach, false
}

// fireLaserTick damages every enemy whose hull the beam crosses — it pierces them
// all (the whole point of a laser). Called by useWeapon on the slot's cooldown, so
// damage lands in ticks. Enemies are walked back-to-front because damageEnemy
// removes the ones it kills.
func (g *Game) fireLaserTick(w *weapon.Weapon) bool {
	if !g.laserOn {
		return false
	}
	if g.laserHitRock {
		g.digAt(g.laserX1, g.laserY1, laserDigRadius) // the beam melts the rock face
	}
	for i := len(g.entities) - 1; i >= 0; i-- {
		if g.entities[i].kind != kindEnemy {
			continue
		}
		e := &g.entities[i]
		if distPointSegmentSq(e.x, e.y, g.x, g.y, g.laserX1, g.laserY1) <= e.radius*e.radius {
			g.damageEnemy(i, damageForLevel(w.Damage, g.damageLevel), w.Col, 0)
		}
	}
	return true
}

// drawLaser strokes the beam from the ship to its endpoint with a bright core (or
// the wide glow), plus a small flash where it lands.
func (g *Game) drawLaser(dst *ebiten.Image, cam ebiten.GeoM, glow bool) {
	if !g.laserOn {
		return
	}
	x0, y0 := cam.Apply(g.x, g.y)
	x1, y1 := cam.Apply(g.laserX1, g.laserY1)
	col, wpx, thin := laserColor, laserWidth, laserThinMin
	if glow {
		col, wpx, thin = laserGlowColor, laserGlowWidth, laserGlowMin
	}
	// The beam wilts as the emitter loads up: thinner, dimmer, its halo collapsing.
	// That IS the overheat warning — nothing is written on screen.
	h := g.laserHeatFrac()
	wpx *= lerp(1, thin, h)
	col.A = uint8(float64(col.A) * lerp(1, laserDimMin, h))

	render.StrokeLine(dst, x0, y0, x1, y1, wpx*g.dpr, col)
	render.FillCircle(dst, x1, y1, wpx*0.9*g.dpr, col)
}

// laserHeatFrac is how loaded the emitter is, 0 (cold) to 1 (about to cut out).
func (g *Game) laserHeatFrac() float64 {
	h := float64(g.laserHeat) / laserHeatMax
	if h < 0 {
		return 0
	}
	if h > 1 {
		return 1
	}
	return h
}
