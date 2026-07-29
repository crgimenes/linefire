package game

import (
	"image/color"
	"math"

	"github.com/crgimenes/linefire/effects"
)

const (
	missileSpeed     = 7.0  // world units per frame (slower than bullets)
	missileLife      = 110  // frames before it fizzles out
	missileInterval  = 45   // frames between missiles (heavy weapon, long cooldown)
	missileDamage    = 4    // full damage at the blast center
	missileRadius    = 70.0 // area-of-effect radius, world units
	missileWidth     = 2.4  // crisp core stroke, logical px (scaled by DPI)
	missileGlowWidth = 5.0  // wider emissive stroke feeding the bloom
	missileShake     = 9.0  // screen shake on detonation
)

var (
	missileColor       = color.RGBA{0xff, 0xa0, 0x40, 0xff} // hot orange core
	missileGlowColor   = color.RGBA{0xff, 0x60, 0x20, 0xff} // orange halo
	missileDamageColor = color.RGBA{0xff, 0xa0, 0x40, 0xff} // orange AoE damage numbers
)

// emitMissileTrail leaves a short fiery trail behind a missile in flight.
func (g *Game) emitMissileTrail(x, y float64) {
	life := 10 + randIntN(8)
	g.emit(effects.Particle{
		X: x, Y: y,
		VX:      (randFloat()*2 - 1) * 0.3,
		VY:      (randFloat()*2 - 1) * 0.3,
		Drag:    0.9,
		Life:    life,
		MaxLife: life,
		Size:    1.0,
		Col:     missileColor,
		Style:   effects.StyleDot,
	})
}

// explodeAt detonates an area blast centered at (x, y): every enemy within radius
// takes damage that falls off linearly with distance (full at the center, a floor
// of 1 at the rim), each popping its own damage number, plus a big particle burst,
// an expanding shockwave ring and screen shake. Enemies are walked back-to-front
// because damageEnemy removes the ones it kills.
func (g *Game) explodeAt(x, y float64, dmg int, radius float64, col color.RGBA) {
	g.emitBurst(x, y, missileBlast)
	g.emitBurst(x, y, missileBlastChunks)
	g.spawnShockwave(x, y, radius)
	g.addShake(missileShake)
	g.playEvent(nil, "explosion")
	g.digBlast(x, y, radius) // SPIKE: a blast eats a crater out of the rock

	for i := len(g.entities) - 1; i >= 0; i-- {
		if g.entities[i].kind != kindEnemy {
			continue
		}
		d := math.Hypot(g.entities[i].x-x, g.entities[i].y-y)
		if d > radius {
			continue
		}
		hit := dmg
		if radius > 0 {
			hit = int(math.Ceil(float64(dmg) * (1 - d/radius)))
		}
		if hit < 1 {
			hit = 1
		}
		g.damageEnemy(i, hit, col)
	}
}
