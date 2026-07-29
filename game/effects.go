package game

import (
	"image/color"

	"github.com/crgimenes/linefire/effects"
	"github.com/hajimehoshi/ebiten/v2"
)

// The particle pool itself lives in the effects package, along with the pool cap,
// the streak widths and the explosion preset. What stays here is this game's own
// presets — thruster, rock, shield, pickup, missile — which are data on top of it.
const (
	muzzleFlashFrames = 4 // how long the muzzle flash lingers

	// Thruster exhaust: a sparse plume trailed from the ship's rear each frame it
	// accelerates, so the counts stay small (the pool cap is the backstop). It is cut
	// from the afterburner's pattern (emitBoostFlame): a NARROW fan of STREAKS thrown
	// fast out the tail, just shorter and cooler than the boost so Shift still reads
	// as a step up. Fat discs were the wrong shape entirely — they smear a blob over
	// the hull instead of a jet, and idle blue ends up burying the boost's orange.
	thrusterPerFrame = 3
	thrusterSpeedMin = 2.4
	thrusterSpeedMax = 4.6
	thrusterSpread   = 0.22 // narrow fan, like the afterburner's
	thrusterLifeMin  = 10
	thrusterLifeMax  = 20
	thrusterDrag     = 0.90
)

var (
	debrisColor      = effects.DebrisColor                // ember chunks, shared with the explosion
	pickupColor      = color.RGBA{0x80, 0xff, 0xb0, 0xff} // green pickup burst
	muzzleColor      = color.RGBA{0xe8, 0xff, 0xff, 0xff} // bright muzzle flash
	wallSparkColor   = color.RGBA{0xc0, 0xff, 0xff, 0xff} // wallSparks default, for a caller with no shot color to pass
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

var (
	// pickupMotes is the soft green puff when a power-up is collected.
	pickupMotes = effects.Burst{
		N: pickupSparks, Col: pickupColor, Style: effects.StyleDot,
		SpeedMin: 1.5, SpeedMax: 4.0, LifeMin: 16, LifeMax: 28, Drag: 0.90, Size: 1.3,
	}
	// wallSparks is the brief cyan flash where a bullet strikes a wall.
	wallSparks = effects.Burst{
		N: 5, Col: wallSparkColor, Style: effects.StyleStreak,
		SpeedMin: 1.5, SpeedMax: 4.0, LifeMin: 8, LifeMax: 16, Drag: 0.82,
	}
	// shieldSparks fly off the hull when the shield absorbs a hit.
	shieldSparks = effects.Burst{
		N: 9, Col: shieldSparkColor, Style: effects.StyleStreak,
		SpeedMin: 2.0, SpeedMax: 5.0, LifeMin: 10, LifeMax: 20, Drag: 0.85,
	}
	// missileBlast + missileBlastChunks are the big area-of-effect detonation: a
	// fast wide spray of orange streaks plus heavy glowing chunks.
	missileBlast = effects.Burst{
		N: 22, Col: missileColor, Style: effects.StyleStreak,
		SpeedMin: 3.0, SpeedMax: 8.5, LifeMin: 16, LifeMax: 32, Drag: 0.88,
	}
	missileBlastChunks = effects.Burst{
		N: 12, Col: debrisColor, Style: effects.StyleDot,
		SpeedMin: 1.5, SpeedMax: 5.0, LifeMin: 30, LifeMax: 52, Drag: 0.92, Size: 2.0,
	}

	// Rock thrown off by a bite out of the map. Digging must read as EXCAVATION, not
	// as sparks off steel: pale chips that tumble and linger, plus dimmer grit that
	// flies. Both are scaled by how big the bite was (see rockChunks/rockGrit).
	rockChunkColor = color.RGBA{0x9a, 0xc8, 0xd6, 0xff}
	rockGritColor  = color.RGBA{0x5e, 0x93, 0xa6, 0xff}
)

// beamCutout is the puff of vapour thrown off the tip when the overheated beam dies:
// it, the falling whistle and the wilting beam are the ONLY notice the player gets.
var beamCutout = effects.Burst{
	N: 11, Col: laserGlowColor, Style: effects.StyleStreak,
	SpeedMin: 0.7, SpeedMax: 2.6, LifeMin: 12, LifeMax: 28, Drag: 0.87,
}

// rockChunks is the heavy debris a bite of radius r knocks loose: slow, lingering.
func rockChunks(r float64) effects.Burst {
	return effects.Burst{
		N: min(max(int(1+r*0.5), 2), 14), Col: rockChunkColor, Style: effects.StyleDot,
		SpeedMin: 0.5, SpeedMax: 1.2 + r*0.06, LifeMin: 16, LifeMax: 34, Drag: 0.90, Size: 1.3,
	}
}

// rockGrit is the light dust off the same bite: faster, shorter-lived streaks.
func rockGrit(r float64) effects.Burst {
	return effects.Burst{
		N: min(max(int(2+r*0.8), 3), 18), Col: rockGritColor, Style: effects.StyleStreak,
		SpeedMin: 1.0, SpeedMax: 2.2 + r*0.12, LifeMin: 8, LifeMax: 18, Drag: 0.86,
	}
}

// fxPool returns the particle pool, building it on first use: a zero Game — which
// the tests construct directly — has to be usable without a constructor.
func (g *Game) fxPool() *effects.Pool {
	if g.fx == nil {
		g.fx = effects.New(effects.DefaultMax, randFloat)
	}
	return g.fx
}

// emit appends one particle to the pool.
func (g *Game) emit(p effects.Particle) {
	g.fxPool().Emit(p)
}

// emitBurst sprays one preset radially from (x, y).
func (g *Game) emitBurst(x, y float64, s effects.Burst) {
	g.fxPool().Burst(x, y, s)
}

// emitExplosion plays the death effect: streaks plus glowing chunks. It is the
// package's, so a ship blows up the same way here and in Linefire Skirmish.
func (g *Game) emitExplosion(x, y float64) {
	g.fxPool().Explosion(x, y)
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
		g.emit(effects.Particle{
			X: rearX, Y: rearY,
			VX:      -fx*speed + px*fan,
			VY:      -fy*speed + py*fan,
			Drag:    thrusterDrag,
			Life:    life,
			MaxLife: life,
			Col:     thrusterColor,
			Style:   effects.StyleStreak,
		})
	}
}

// stepParticles advances and ages every particle, dropping the dead ones.
func (g *Game) stepParticles() {
	g.fxPool().Step()
}

// drawParticles renders the pool through the camera; glow selects the wider
// strokes that feed the bloom buffer.
func (g *Game) drawParticles(dst *ebiten.Image, cam ebiten.GeoM, glow bool) {
	g.fxPool().Draw(dst, effects.View{
		Cam:        cam,
		DPR:        g.dpr,
		PixelScale: g.camPixelScale(),
		Glow:       glow,
	})
}
