package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	explosionSparks   = 14   // streak particles spawned when an enemy is destroyed
	explosionDebris   = 6    // dot particles (chunks) added to the same explosion
	sparkSpeedMin     = 2.0  // world units per frame
	sparkSpeedMax     = 6.0  // world units per frame
	sparkLifeMin      = 18   // frames
	sparkLifeMax      = 34   // frames
	sparkDrag         = 0.90 // velocity retained per frame (sparks slow down)
	sparkWidth        = 1.6  // crisp streak width, logical px (scaled by DPI)
	sparkGlowWidth    = 3.0  // wider emissive streak feeding the bloom
	muzzleFlashFrames = 4    // how long the muzzle flash lingers

	// maxParticles caps the live pool: a hard backstop so a pile-up of explosions
	// can never grow the slice without bound. Particles are purely cosmetic, so
	// emits past the cap are dropped silently (the oldest keep aging out).
	maxParticles = 1500

	dotGlowScale = 1.8 // dot radius multiplier in the emissive pass (softer bloom)

	// Thruster exhaust: a sparse plume trailed from the ship's rear each frame
	// it accelerates, so the counts stay small (the pool cap is the backstop).
	thrusterPerFrame = 2
	thrusterSpeedMin = 1.0
	thrusterSpeedMax = 2.6
	thrusterSpread   = 0.7 // sideways fan as a fraction of the backward speed
	thrusterLifeMin  = 8
	thrusterLifeMax  = 16
	thrusterDrag     = 0.86
	thrusterSize     = 1.1 // dot radius, world units
)

var (
	sparkColor       = color.RGBA{0xff, 0xc8, 0x60, 0xff} // warm explosion core
	debrisColor      = color.RGBA{0xff, 0x90, 0x40, 0xff} // hotter ember chunks
	pickupColor      = color.RGBA{0x80, 0xff, 0xb0, 0xff} // green pickup burst
	muzzleColor      = color.RGBA{0xe8, 0xff, 0xff, 0xff} // bright muzzle flash
	wallSparkColor   = color.RGBA{0xc0, 0xff, 0xff, 0xff} // cyan-white bullet-on-wall sparks
	shieldSparkColor = color.RGBA{0x80, 0xff, 0xff, 0xff} // cyan shield deflection
	thrusterColor    = color.RGBA{0x90, 0xe0, 0xff, 0xff} // pale blue engine exhaust
)

// addShake bumps the screen-shake magnitude, taking the max so a fresh jolt is
// not diluted by a smaller one already fading.
func (g *Game) addShake(amount float64) {
	if amount > g.shakeMag {
		g.shakeMag = amount
	}
}

// decayShake fades the shake magnitude each tick.
func (g *Game) decayShake() {
	g.shakeMag *= shakeDecay
	if g.shakeMag < 0.05 {
		g.shakeMag = 0
	}
}

// updateShakeOffset picks this frame's random screen offset (device px). It is
// computed once per Draw so every motion-blur sub-frame shares the same offset.
func (g *Game) updateShakeOffset() {
	if g.shakeMag <= 0 {
		g.shakeX, g.shakeY = 0, 0
		return
	}
	g.shakeX = (randFloat()*2 - 1) * g.shakeMag * g.dpr
	g.shakeY = (randFloat()*2 - 1) * g.shakeMag * g.dpr
}

// partStyle selects how a particle is drawn.
type partStyle uint8

const (
	styleStreak partStyle = iota // fading line from the previous to the current point
	styleDot                     // fading disc of `size` world units (chunks, motes)
)

// particle is a short-lived cosmetic effect element. It carries its own motion
// (velocity + drag) and look (style/size/color) so one pool drives every effect.
type particle struct {
	x, y    float64
	px, py  float64 // previous point, for the streak style
	vx, vy  float64
	drag    float64
	life    int
	maxLife int
	size    float64 // dot radius in world units (styleDot only)
	col     color.RGBA
	style   partStyle
}

// burstSpec is a named emitter preset: a radial spray of particles sharing a
// look and a speed/lifetime range. New effects are a new preset, not new code.
type burstSpec struct {
	n                  int
	col                color.RGBA
	style              partStyle
	speedMin, speedMax float64
	lifeMin, lifeMax   int
	drag               float64
	size               float64 // dot radius (styleDot only)
}

var (
	// explosionStreaks + explosionChunks make up an enemy death: warm streaks
	// flung out fast, plus a few slower glowing chunks that linger.
	explosionStreaks = burstSpec{
		n: explosionSparks, col: sparkColor, style: styleStreak,
		speedMin: sparkSpeedMin, speedMax: sparkSpeedMax,
		lifeMin: sparkLifeMin, lifeMax: sparkLifeMax, drag: sparkDrag,
	}
	explosionChunks = burstSpec{
		n: explosionDebris, col: debrisColor, style: styleDot,
		speedMin: 1.0, speedMax: 3.5, lifeMin: 26, lifeMax: 46, drag: 0.93, size: 1.6,
	}
	// pickupMotes is the soft green puff when a power-up is collected.
	pickupMotes = burstSpec{
		n: pickupSparks, col: pickupColor, style: styleDot,
		speedMin: 1.5, speedMax: 4.0, lifeMin: 16, lifeMax: 28, drag: 0.90, size: 1.3,
	}
	// wallSparks is the brief cyan flash where a bullet strikes a wall.
	wallSparks = burstSpec{
		n: 5, col: wallSparkColor, style: styleStreak,
		speedMin: 1.5, speedMax: 4.0, lifeMin: 8, lifeMax: 16, drag: 0.82,
	}
	// shieldSparks fly off the hull when the shield absorbs a hit.
	shieldSparks = burstSpec{
		n: 9, col: shieldSparkColor, style: styleStreak,
		speedMin: 2.0, speedMax: 5.0, lifeMin: 10, lifeMax: 20, drag: 0.85,
	}
	// missileBlast + missileBlastChunks are the big area-of-effect detonation: a
	// fast wide spray of orange streaks plus heavy glowing chunks.
	missileBlast = burstSpec{
		n: 22, col: missileColor, style: styleStreak,
		speedMin: 3.0, speedMax: 8.5, lifeMin: 16, lifeMax: 32, drag: 0.88,
	}
	missileBlastChunks = burstSpec{
		n: 12, col: debrisColor, style: styleDot,
		speedMin: 1.5, speedMax: 5.0, lifeMin: 30, lifeMax: 52, drag: 0.92, size: 2.0,
	}

	// Rock thrown off by a bite out of the map. Digging must read as EXCAVATION, not
	// as sparks off steel: pale chips that tumble and linger, plus dimmer grit that
	// flies. Both are scaled by how big the bite was (see rockChunks/rockGrit).
	rockChunkColor = color.RGBA{0x9a, 0xc8, 0xd6, 0xff}
	rockGritColor  = color.RGBA{0x5e, 0x93, 0xa6, 0xff}
)

// beamCutout is the puff of vapour thrown off the tip when the overheated beam dies:
// it, the falling whistle and the wilting beam are the ONLY notice the player gets.
var beamCutout = burstSpec{
	n: 11, col: laserGlowColor, style: styleStreak,
	speedMin: 0.7, speedMax: 2.6, lifeMin: 12, lifeMax: 28, drag: 0.87,
}

// rockChunks is the heavy debris a bite of radius r knocks loose: slow, lingering.
func rockChunks(r float64) burstSpec {
	return burstSpec{
		n: min(max(int(1+r*0.5), 2), 14), col: rockChunkColor, style: styleDot,
		speedMin: 0.5, speedMax: 1.2 + r*0.06, lifeMin: 16, lifeMax: 34, drag: 0.90, size: 1.3,
	}
}

// rockGrit is the light dust off the same bite: faster, shorter-lived streaks.
func rockGrit(r float64) burstSpec {
	return burstSpec{
		n: min(max(int(2+r*0.8), 3), 18), col: rockGritColor, style: styleStreak,
		speedMin: 1.0, speedMax: 2.2 + r*0.12, lifeMin: 8, lifeMax: 18, drag: 0.86,
	}
}

// emit appends one particle, respecting the hard pool cap (see maxParticles).
func (g *Game) emit(p particle) {
	if len(g.particles) >= maxParticles {
		return
	}
	p.px, p.py = p.x, p.y
	g.particles = append(g.particles, p)
}

// emitBurst sprays one preset radially from (x, y).
func (g *Game) emitBurst(x, y float64, s burstSpec) {
	for range s.n {
		ang := randFloat() * 2 * math.Pi
		speed := s.speedMin + randFloat()*(s.speedMax-s.speedMin)
		life := s.lifeMin + randIntN(s.lifeMax-s.lifeMin+1)
		g.emit(particle{
			x: x, y: y,
			vx:      math.Cos(ang) * speed,
			vy:      math.Sin(ang) * speed,
			drag:    s.drag,
			life:    life,
			maxLife: life,
			size:    s.size,
			col:     s.col,
			style:   s.style,
		})
	}
}

// emitExplosion plays the enemy-death effect: streaks plus glowing chunks.
func (g *Game) emitExplosion(x, y float64) {
	g.emitBurst(x, y, explosionStreaks)
	g.emitBurst(x, y, explosionChunks)
}

// emitThruster trails exhaust from the ship's rear while it accelerates along the
// forward unit vector (fx, fy). Called once per thrust frame, so it stays sparse.
// The plume shoots backward with a small sideways fan; (px, py) is (fx, fy) turned
// 90° for that lateral spread.
func (g *Game) emitThruster(fx, fy float64) {
	rearX := g.x - fx*g.radius
	rearY := g.y - fy*g.radius
	px, py := -fy, fx
	for range thrusterPerFrame {
		speed := thrusterSpeedMin + randFloat()*(thrusterSpeedMax-thrusterSpeedMin)
		fan := (randFloat()*2 - 1) * thrusterSpread * speed
		life := thrusterLifeMin + randIntN(thrusterLifeMax-thrusterLifeMin+1)
		g.emit(particle{
			x: rearX, y: rearY,
			vx:      -fx*speed + px*fan,
			vy:      -fy*speed + py*fan,
			drag:    thrusterDrag,
			life:    life,
			maxLife: life,
			size:    thrusterSize,
			col:     thrusterColor,
			style:   styleDot,
		})
	}
}

// stepParticles advances and ages every particle, dropping the dead ones. Pure
// motion (no rendering), so it is cheap and unit-testable.
func (g *Game) stepParticles() {
	kept := g.particles[:0]
	for i := range g.particles {
		p := g.particles[i]
		p.life--
		if p.life <= 0 {
			continue
		}
		p.px, p.py = p.x, p.y
		p.x += p.vx
		p.y += p.vy
		p.vx *= p.drag
		p.vy *= p.drag
		kept = append(kept, p)
	}
	g.particles = kept
}

// drawParticles renders the pool, fading each particle with its remaining life.
// glow widens streaks and discs so the same pool feeds the bloom buffer too.
func (g *Game) drawParticles(dst *ebiten.Image, cam ebiten.GeoM, glow bool) {
	streakW := sparkWidth
	sizeScale := 1.0
	if glow {
		streakW = sparkGlowWidth
		sizeScale = dotGlowScale
	}
	for i := range g.particles {
		p := &g.particles[i]
		frac := float64(p.life) / float64(p.maxLife)
		c := p.col
		c.A = uint8(float64(p.col.A) * frac)
		if p.style == styleDot {
			cx, cy := cam.Apply(p.x, p.y)
			r := p.size * g.camPixelScale() * sizeScale * frac
			if r < 0.5*g.dpr {
				r = 0.5 * g.dpr // keep a visible speck until it fades out
			}
			fillCircle(dst, cx, cy, r, c)
			continue
		}
		x0, y0 := cam.Apply(p.px, p.py)
		x1, y1 := cam.Apply(p.x, p.y)
		strokeLine(dst, x0, y0, x1, y1, streakW*g.dpr, c)
	}
}
