// Package effects is Linefire's particle system: one pool that drives every
// cosmetic spray in the game — explosions, sparks, exhaust, debris — and the
// presets that describe them.
//
// It is deliberately free of any game state. A pool is handed its own randomness
// and is drawn through an explicit camera and scale, so the same explosion plays
// in the game, in the editors' previews, and in Linefire Skirmish, rather than
// each of them growing its own.
//
// Everything here is LIGHT: particles are drawn additively, and a particle fades
// by dropping its additive gain, never by lowering its color's alpha. Additive
// blending ignores what lies under it, so a lower alpha alone would leave a dying
// spark exactly as bright as a fresh one.
package effects

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/crgimenes/linefire/render"
)

const (
	// DefaultMax caps a pool: a hard backstop so a pile-up of explosions can never
	// grow the slice without bound. Particles are purely cosmetic, so emits past
	// the cap are dropped silently and the oldest keep aging out.
	DefaultMax = 1500

	streakWidth     = 1.6 // crisp streak width, logical px (scaled by the caller's DPR)
	streakGlowWidth = 3.0 // wider emissive streak feeding the bloom
	dotGlowScale    = 1.8 // dot radius multiplier in the emissive pass (softer bloom)
	minDotRadius    = 0.5 // logical px: keep a visible speck until it fades out

	// Engine exhaust, trailed once per accelerating frame, so the counts stay small.
	thrusterPerFrame = 3
	thrusterSpeedMin = 2.4
	thrusterSpeedMax = 4.6
	thrusterSpread   = 0.22 // narrow fan
	thrusterLifeMin  = 10
	thrusterLifeMax  = 20
	thrusterDrag     = 0.90
)

// Colors shared by the presets below.
var (
	SparkColor    = color.RGBA{0xff, 0xc8, 0x60, 0xff} // warm explosion core
	DebrisColor   = color.RGBA{0xff, 0x90, 0x40, 0xff} // hotter ember chunks
	ThrusterColor = color.RGBA{0x90, 0xe0, 0xff, 0xff} // pale blue engine exhaust
)

// ExplosionStreaks and ExplosionChunks together are a ship dying: warm streaks
// flung out fast, plus a few slower glowing chunks that linger. Play both with
// Pool.Explosion.
var (
	ExplosionStreaks = Burst{
		N: 14, Col: SparkColor, Style: StyleStreak,
		SpeedMin: 2.0, SpeedMax: 6.0, LifeMin: 18, LifeMax: 34, Drag: 0.90,
	}
	ExplosionChunks = Burst{
		N: 6, Col: DebrisColor, Style: StyleDot,
		SpeedMin: 1.0, SpeedMax: 3.5, LifeMin: 26, LifeMax: 46, Drag: 0.93, Size: 1.6,
	}
)

// Style selects how a particle is drawn.
type Style uint8

const (
	StyleStreak Style = iota // fading line from the previous to the current point
	StyleDot                 // fading disc of Size world units (chunks, motes)
)

// Particle is a short-lived element of an effect. It carries its own motion
// (velocity plus drag) and look (style, size, color) so one pool drives
// everything. Emit fills PX/PY; the rest is the caller's.
type Particle struct {
	X, Y    float64
	PX, PY  float64 // previous point, for the streak style
	VX, VY  float64
	Drag    float64
	Life    int
	MaxLife int
	Size    float64 // dot radius in world units (StyleDot only)
	Col     color.RGBA
	Style   Style
}

// Burst is a named emitter preset: a radial spray of particles sharing a look
// and a speed/lifetime range. A new effect should be a new preset, not new code.
type Burst struct {
	N                  int
	Col                color.RGBA
	Style              Style
	SpeedMin, SpeedMax float64
	LifeMin, LifeMax   int
	Drag               float64
	Size               float64 // dot radius (StyleDot only)
}

// Pool holds the live particles.
type Pool struct {
	parts  []Particle
	max    int
	random func() float64
}

// New returns an empty pool holding at most max particles and drawing its
// randomness from random, which must return values in [0, 1).
//
// The caller supplies the source so the effects inherit whatever determinism it
// has: the game uses its global one, a seeded simulation its own.
func New(max int, random func() float64) *Pool {
	if max <= 0 {
		max = DefaultMax
	}
	return &Pool{max: max, random: random}
}

// Len reports how many particles are alive.
func (p *Pool) Len() int { return len(p.parts) }

// Particles exposes the live pool for inspection. The slice is the pool's own:
// read it, do not keep it past the next Step.
func (p *Pool) Particles() []Particle { return p.parts }

// Emit appends one particle, respecting the pool cap.
func (p *Pool) Emit(pt Particle) {
	if len(p.parts) >= p.max {
		return
	}
	pt.PX, pt.PY = pt.X, pt.Y
	p.parts = append(p.parts, pt)
}

// Burst sprays one preset radially from (x, y).
func (p *Pool) Burst(x, y float64, b Burst) {
	for range b.N {
		angle := p.random() * 2 * math.Pi
		speed := b.SpeedMin + p.random()*(b.SpeedMax-b.SpeedMin)
		life := b.LifeMin + p.intn(b.LifeMax-b.LifeMin+1)
		p.Emit(Particle{
			X: x, Y: y,
			VX:      math.Cos(angle) * speed,
			VY:      math.Sin(angle) * speed,
			Drag:    b.Drag,
			Life:    life,
			MaxLife: life,
			Size:    b.Size,
			Col:     b.Col,
			Style:   b.Style,
		})
	}
}

// Explosion plays a death: streaks plus glowing chunks.
func (p *Pool) Explosion(x, y float64) {
	p.Burst(x, y, ExplosionStreaks)
	p.Burst(x, y, ExplosionChunks)
}

// Thruster trails exhaust from (x, y) — the ship's rear — for one frame of
// acceleration along the forward unit vector (fx, fy). Call it once per thrusting
// frame; the counts are small because of that, and the pool cap is the backstop.
//
// The plume is a NARROW fan of streaks thrown fast out the tail. Fat discs were
// the wrong shape entirely: they smear a blob over the hull instead of a jet.
func (p *Pool) Thruster(x, y, fx, fy float64) {
	sx, sy := -fy, fx // the forward vector turned 90°, for the lateral spread
	for range thrusterPerFrame {
		speed := thrusterSpeedMin + p.random()*(thrusterSpeedMax-thrusterSpeedMin)
		fan := (p.random()*2 - 1) * thrusterSpread * speed
		life := thrusterLifeMin + p.intn(thrusterLifeMax-thrusterLifeMin+1)
		p.Emit(Particle{
			X: x, Y: y,
			VX:      -fx*speed + sx*fan,
			VY:      -fy*speed + sy*fan,
			Drag:    thrusterDrag,
			Life:    life,
			MaxLife: life,
			Col:     ThrusterColor,
			Style:   StyleStreak,
		})
	}
}

// Step advances and ages every particle, dropping the dead ones. Pure motion, no
// rendering, so it is cheap and testable on its own.
func (p *Pool) Step() {
	kept := p.parts[:0]
	for i := range p.parts {
		pt := p.parts[i]
		pt.Life--
		if pt.Life <= 0 {
			continue
		}
		pt.PX, pt.PY = pt.X, pt.Y
		pt.X += pt.VX
		pt.Y += pt.VY
		pt.VX *= pt.Drag
		pt.VY *= pt.Drag
		kept = append(kept, pt)
	}
	p.parts = kept
}

// View is how a pool is placed on screen: the camera transform plus the two
// scales the drawing needs.
type View struct {
	Cam ebiten.GeoM // world -> screen

	// DPR is device pixels per logical pixel, applied to stroke widths so a
	// streak is the same apparent thickness on any display.
	DPR float64

	// PixelScale is device pixels per world unit, applied to dot radii so a chunk
	// is the same size in the world however the camera is zoomed.
	PixelScale float64

	// Glow widens strokes and discs for the emissive pass that feeds the bloom.
	Glow bool
}

// Draw renders the pool, fading each particle with its remaining life.
func (p *Pool) Draw(dst *ebiten.Image, v View) {
	width := streakWidth
	sizeScale := 1.0
	if v.Glow {
		width = streakGlowWidth
		sizeScale = dotGlowScale
	}
	for i := range p.parts {
		pt := &p.parts[i]
		frac := float64(pt.Life) / float64(pt.MaxLife)
		if pt.Style == StyleDot {
			cx, cy := v.Cam.Apply(pt.X, pt.Y)
			r := max(pt.Size*v.PixelScale*sizeScale*frac, minDotRadius*v.DPR)
			render.FillCircleAdd(dst, cx, cy, r, pt.Col, frac)
			continue
		}
		x0, y0 := v.Cam.Apply(pt.PX, pt.PY)
		x1, y1 := v.Cam.Apply(pt.X, pt.Y)
		render.StrokeLineAdd(dst, x0, y0, x1, y1, width*v.DPR, pt.Col, frac)
	}
}

// intn returns a value in [0, n) from the pool's source, or 0 for n <= 0.
func (p *Pool) intn(n int) int {
	if n <= 0 {
		return 0
	}
	return min(int(p.random()*float64(n)), n-1)
}
