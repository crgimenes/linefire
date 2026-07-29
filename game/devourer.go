package game

import (
	"image/color"
	"math"

	"github.com/crgimenes/linefire/effects"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// The DEVOURER is the game's one-of-a-kind superweapon: a deployable black hole. You drop it
// (aimDrop, like a mine), it ARMS for a couple of seconds — a growing vortex with a countdown,
// your window to flee — then it goes ACTIVE and drags EVERYTHING toward it, the player included:
// to hold position you must thrust away at full burn. Anything that reaches the core is swallowed
// and destroyed; debris streams in from all around for drama. When the timer runs out it COLLAPSES
// in a violent implosion. Charges are scarce (a picked-up weapon, 3 uses), so it stays a panic
// button, not a spam tool.

const (
	devourerCharges = 3 // uses granted per pickup

	devourerArm      = 180 // frames arming before the pull starts (~3s: the flee window)
	devourerActive   = 300 // frames the pull lasts (~5s)
	devourerCooldown = 90  // frames between deploys (spam guard on top of charges + one-at-a-time)

	devourerReach      = 1200.0 // world radius the pull reaches — the whole screen and beyond
	devourerPullAccel  = 0.16   // player pull accel at the rim (< thrust so a full-burn flee can hold)
	devourerPullMaxMul = 3.0    // closer = stronger; this caps how much the pull ramps near the core
	devourerEntityPull = 7.0    // base world units/frame an object is dragged in (ramps like the player pull)
	devourerSwallow    = 34.0   // objects (and the ship) within this of the core are destroyed

	devourerDanger    = 150.0 // within this of the core the singularity shreds the SHIP's hull
	devourerDangerDPS = 4     // base hull damage per tick in the danger zone (scales up toward the core)
	devourerHurtEvery = 6     // frames between danger-zone hull ticks, so the hit SFX does not machine-gun

	devourerCollapseDmg    = 300   // implosion center damage: massive — levels nearly everything in range
	devourerCollapseRadius = 460.0 // implosion radius

	devourerVisRadius = 60.0 // drawn vortex radius, world units
	devourerMotes     = 96   // converging particles at full intensity
	devourerSpeed     = 0.7  // particle rim -> centre travel (cycles per second)
	devourerDebris    = 3    // inbound debris streaks spawned per active frame
)

var (
	devourerColor       = color.RGBA{0xc0, 0x40, 0xff, 0xff} // accretion violet
	devourerDebrisColor = color.RGBA{0x90, 0x40, 0xe0, 0xff}
	// devourerSwallowBurst is the flash when an object is crushed at the core.
	devourerSwallowBurst = effects.Burst{
		N: 10, Col: devourerColor, Style: effects.StyleStreak,
		SpeedMin: 1.0, SpeedMax: 4.0, LifeMin: 8, LifeMax: 16, Drag: 0.85,
	}
	// devourerCollapseBurst is the violet flare over the implosion.
	devourerCollapseBurst = effects.Burst{
		N: 34, Col: devourerColor, Style: effects.StyleStreak,
		SpeedMin: 4.0, SpeedMax: 11.0, LifeMin: 18, LifeMax: 40, Drag: 0.9,
	}
)

// devourer is the live black hole: only one exists at a time. It arms, then pulls, then collapses.
type devourer struct {
	x, y   float64
	arm    int // frames of arming left (countdown shown; no pull yet)
	active int // frames of active pull left (once arm hits 0)
}

// deployDevourerWeapon drops the black hole at the ship, spending a charge. Refused with no
// charge or while one is already live, so a held button cannot stack them.
func (g *Game) deployDevourerWeapon(w *weapon) bool {
	if g.devourerAmmo <= 0 {
		g.logf("DEVOURER  no charge")
		return false
	}
	if g.devourer != nil {
		return false
	}
	g.devourerAmmo--
	g.devourer = &devourer{x: g.x, y: g.y, arm: devourerArm, active: devourerActive}
	g.sfx.play(w.fire)
	g.logf("DEPLOY  ► DEVOURER // %d left", g.devourerAmmo)
	return true
}

// stepDevourer advances the live black hole one frame: count down the arming, then run the pull
// (dragging objects in and eating any that reach the core), then collapse.
func (g *Game) stepDevourer() {
	d := g.devourer
	if d == nil {
		return
	}
	if d.arm > 0 {
		d.arm--
		if d.arm == 0 {
			g.addShake(4)
			g.logf(">>> it hungers <<<")
		}
		return
	}
	if d.active > 0 {
		d.active--
		g.devourerPull(d)
		g.devourerPullAllies(d)
		g.devourerCrushShip(d)
		g.spawnDevourerDebris(d)
		return
	}
	g.collapseDevourer(d)
	g.devourer = nil
}

// devourerCrushShip hurts the SHIP by how deep into the well it is: instant death at the core,
// and rising hull damage anywhere inside the danger zone — so being dragged in is lethal, not
// just the final collapse, and you do not have to hit the exact centre to die. A bubble/god
// shield still saves you, as it should. Damage ticks on a cadence so the hit SFX does not buzz.
func (g *Game) devourerCrushShip(d *devourer) {
	dist := math.Hypot(g.x-d.x, g.y-d.y)
	if dist <= devourerSwallow {
		g.hurtPlayer(maxHealth + maxShield) // the core: swallowed whole
		return
	}
	if dist < devourerDanger && d.active%devourerHurtEvery == 0 {
		frac := 1 - dist/devourerDanger // 0 at the edge of the zone, ~1 at the core
		g.hurtPlayer(devourerDangerDPS + int(float64(3*devourerDangerDPS)*frac))
	}
}

// devourerRamp is the pull multiplier at distance dist: 1 at the rim, growing toward the core so
// the well has a point of no return, capped so it never explodes to infinity.
func devourerRamp(dist float64) float64 {
	return math.Min(devourerReach/dist, devourerPullMaxMul)
}

// pullToward drags a point of the given collision radius toward (tx,ty) by step, but STOPS it at
// walls (axis-separated, so it slides along a face) — a dragged object piles up against rock
// instead of clipping through it. Returns the new position.
func (g *Game) pullToward(x, y, tx, ty, step, radius float64) (float64, float64) {
	dx, dy := tx-x, ty-y
	dist := math.Hypot(dx, dy)
	if dist < 1e-6 {
		return x, y
	}
	nx, ny := x+dx/dist*step, y+dy/dist*step
	if !g.collidesAt(nx, ny, radius) {
		return nx, ny
	}
	rx, ry := x, y
	if !g.collidesAt(nx, y, radius) {
		rx = nx
	}
	if !g.collidesAt(rx, ny, radius) {
		ry = ny
	}
	return rx, ry
}

// applyDevourerPull drags the SHIP toward the black hole (call before clampSpeed, so friction and
// the ship's own thrust fight it). Only during the active phase.
func (g *Game) applyDevourerPull() {
	d := g.devourer
	if d == nil || d.arm > 0 || d.active <= 0 {
		return
	}
	dx, dy := d.x-g.x, d.y-g.y
	dist := math.Hypot(dx, dy)
	if dist < 1 || dist > devourerReach {
		return
	}
	a := devourerPullAccel * devourerRamp(dist)
	g.vx += dx / dist * a
	g.vy += dy / dist * a
}

// devourerPull drags every eligible object toward the core and swallows anything that arrives.
// Objects are moved directly (not steered), so an enemy cannot fly against the pull.
func (g *Game) devourerPull(d *devourer) {
	kept := g.entities[:0]
	for i := range g.entities {
		e := g.entities[i]
		if e.kind == kindPortal { // exits stay put; the hole does not eat the way out
			kept = append(kept, e)
			continue
		}
		dx, dy := d.x-e.x, d.y-e.y
		dist := math.Hypot(dx, dy)
		if dist <= devourerSwallow {
			g.swallowEntity(&e)
			continue // gone
		}
		if dist < devourerReach {
			// Cap the step at dist (not dist-swallow), or it asymptotes to the swallow ring and
			// the object hangs there forever instead of being pulled into the core. Walls block
			// it, so an object dragged into rock piles up against the face instead of clipping.
			step := math.Min(devourerEntityPull*devourerRamp(dist), dist)
			e.x, e.y = g.pullToward(e.x, e.y, d.x, d.y, step, math.Max(e.radius, 6))
		}
		kept = append(kept, e)
	}
	g.entities = kept
}

// devourerPullAllies drags the player's escorts and drones in too — the black hole plays no
// favourites — and destroys any that reach the core.
func (g *Game) devourerPullAllies(d *devourer) {
	kept := g.allies[:0]
	for i := range g.allies {
		a := g.allies[i]
		dx, dy := d.x-a.x, d.y-a.y
		dist := math.Hypot(dx, dy)
		if dist <= devourerSwallow {
			g.emitBurst(a.x, a.y, devourerSwallowBurst)
			g.emitExplosion(a.x, a.y)
			continue
		}
		if dist < devourerReach {
			step := math.Min(devourerEntityPull*devourerRamp(dist), dist)
			a.x, a.y = g.pullToward(a.x, a.y, d.x, d.y, step, 10)
		}
		kept = append(kept, a)
	}
	g.allies = kept
}

// swallowEntity destroys an object crushed at the core: a violet flash, score for a kill, and it
// stays gone if the player revisits the map.
func (g *Game) swallowEntity(e *entity) {
	g.emitBurst(e.x, e.y, devourerSwallowBurst)
	g.markConsumed(e.spawn)
	if e.kind == kindEnemy {
		g.score++
		g.emitExplosion(e.x, e.y) // it detonates the instant it hits the singularity
		g.addShake(deathShake * 0.4)
		g.playEvent(e.a, "destroy")
	}
}

// spawnDevourerDebris streams cosmetic wreckage inward from around the hole, to sell the suck.
func (g *Game) spawnDevourerDebris(d *devourer) {
	for range devourerDebris {
		ang := randFloat() * 2 * math.Pi
		r := devourerReach * (0.35 + 0.35*randFloat())
		sp := 5 + randFloat()*4
		px, py := d.x+r*math.Cos(ang), d.y+r*math.Sin(ang)
		g.emit(effects.Particle{
			X: px, Y: py, PX: px, PY: py,
			VX: -math.Cos(ang) * sp, VY: -math.Sin(ang) * sp,
			Drag: 1.0, Life: 34, MaxLife: 34, Size: 1.3, Col: devourerDebrisColor, Style: effects.StyleStreak,
		})
	}
}

// collapseDevourer ends the black hole in a MASSIVE implosion: it wipes out the enemies in range
// (via explodeAt), levels the structures inside the blast (a full-radius rock carve), takes out
// any escorts caught in it, and hits a too-close player — plus a shockwave, a violet flare and a
// hard shake.
func (g *Game) collapseDevourer(d *devourer) {
	g.explodeAt(d.x, d.y, devourerCollapseDmg, devourerCollapseRadius, devourerColor)
	g.digAt(d.x, d.y, devourerCollapseRadius) // level the structures in the blast (nil-safe on procedural maps)
	g.destroyAlliesInRadius(d.x, d.y, devourerCollapseRadius)
	g.emitBurst(d.x, d.y, devourerCollapseBurst)
	g.addShake(deathShake * 1.8)
	if dist := math.Hypot(g.x-d.x, g.y-d.y); dist < devourerCollapseRadius {
		g.hurtPlayer(int(math.Ceil(float64(devourerCollapseDmg) * (1 - dist/devourerCollapseRadius))))
	}
	g.logf("*** DEVOURER COLLAPSE ***")
}

// destroyAlliesInRadius removes every escort within r of (x,y) — the collapse spares nothing.
func (g *Game) destroyAlliesInRadius(x, y, r float64) {
	kept := g.allies[:0]
	for i := range g.allies {
		a := g.allies[i]
		if math.Hypot(a.x-x, a.y-y) <= r {
			g.emitBurst(a.x, a.y, devourerSwallowBurst)
			continue
		}
		kept = append(kept, a)
	}
	g.allies = kept
}

// drawDevourer renders the black hole: a dark core, a pulsing accretion ring, and particles
// spiralling to the centre (reusing the portal look, denser and menacing). While arming it grows
// and shows a seconds countdown; the particle count climbs as it nears going active.
func (g *Game) drawDevourer(dst *ebiten.Image, cam ebiten.GeoM) {
	d := g.devourer
	if d == nil {
		return
	}
	mx, my := cam.Apply(d.x, d.y)
	cx, cy := float32(mx), float32(my)
	sc := g.camPixelScale()
	t := float64(ebiten.Tick()) / float64(ebiten.TPS())

	grow, motes := 1.0, devourerMotes
	if d.arm > 0 {
		grow = 1 - float64(d.arm)/float64(devourerArm) // 0 -> 1 as it arms
		motes = int(float64(devourerMotes) * (0.15 + 0.85*grow))
	}
	rMax := devourerVisRadius * (0.3 + 0.7*grow) * sc

	vector.FillCircle(dst, cx, cy, float32(rMax*0.5), color.RGBA{0x00, 0x00, 0x00, 0xff}, true) // the void core
	ring := devourerColor
	ring.A = uint8(0x80 + 0x60*(0.5+0.5*math.Sin(t*5)))
	vector.StrokeCircle(dst, cx, cy, float32(rMax), float32(1.7*g.dpr), ring, true)

	for i := range motes {
		p := t*devourerSpeed + float64(i)/float64(devourerMotes)
		u := 1 - (p - math.Floor(p)) // 1 at rim, 0 at core
		ang := effects.SpokeAngle(i, int(math.Floor(p)))
		rr := float64(rMax) * u
		px := cx + float32(rr*math.Cos(ang))
		py := cy + float32(rr*math.Sin(ang))
		mc := devourerColor
		mc.A = uint8(235 * math.Sin(math.Pi*u))
		vector.FillCircle(dst, px, py, float32(1.7*sc), mc, true)
	}

	if d.arm > 0 {
		secs := (d.arm-1)/60 + 1
		img := g.numberImage(itoa(secs))
		w, h := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
		g.drawGlyph(dst, img, w, h, float64(cx), float64(cy)-float64(rMax)-10*g.dpr, 1.6*g.dpr, devourerColor)
	}
}

// itoa renders a small non-negative int without fmt, for the countdown glyph key.
func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 && i > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
