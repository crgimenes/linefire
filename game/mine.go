package game

import (
	"image/color"

	"github.com/crgimenes/linefire/weapon"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	mineBlinkPeriod = 8   // frames per blink phase while arming
	mineCoreRadius  = 4.0 // drawn body radius, world units
)

var (
	mineColor       = color.RGBA{0xff, 0x70, 0x30, 0xff} // armed body: hot orange
	mineArmingColor = color.RGBA{0xff, 0xd0, 0x40, 0xff} // arming blink: yellow
)

// mine is a stationary trap: it arms after a delay, then detonates with an AoE
// blast (reusing explodeAt) when an enemy wanders within trigger range. Its
// payload (damage/radius/trigger/color) comes from the weapon that laid it.
type mine struct {
	x, y    float64
	arm     int        // frames until armed; >0 = arming (blinking), 0 = live
	dmg     int        // blast center damage
	radius  float64    // AoE radius
	trigger float64    // proximity distance that sets it off
	col     color.RGBA // AoE damage-number color
}

// stepMines arms laid mines and detonates any armed mine an enemy has wandered
// into, removing the ones that go off.
func (g *Game) stepMines() {
	kept := g.mines[:0]
	for i := range g.mines {
		m := g.mines[i]
		if m.arm > 0 {
			m.arm--
			kept = append(kept, m)
			continue
		}
		if g.enemyWithin(m.x, m.y, m.trigger) {
			g.explodeAt(m.x, m.y, m.dmg, m.radius, m.col)
			continue // detonated: drop it
		}
		kept = append(kept, m)
	}
	g.mines = kept
}

// enemyWithin reports whether any live enemy is within r of (x, y).
func (g *Game) enemyWithin(x, y, r float64) bool {
	r2 := r * r
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy {
			continue
		}
		dx, dy := e.x-x, e.y-y
		if dx*dx+dy*dy <= r2 {
			return true
		}
	}
	return false
}

// drawMines renders each mine: a blinking body while arming, a steady body with a
// faint trigger ring once armed. glow draws only the bright core (bigger) for the
// bloom; the crisp pass adds the trigger ring.
func (g *Game) drawMines(dst *ebiten.Image, cam ebiten.GeoM, glow bool) {
	scale := g.camPixelScale()
	for i := range g.mines {
		m := &g.mines[i]
		px, py := cam.Apply(m.x, m.y)
		cx, cy := float32(px), float32(py)
		r := float32(mineCoreRadius * scale)

		if m.arm > 0 {
			if (m.arm/mineBlinkPeriod)%2 == 0 {
				vector.FillCircle(dst, cx, cy, r, mineArmingColor, true)
			}
			continue
		}
		if !glow {
			ring := mineColor
			ring.A = 0x50
			vector.StrokeCircle(dst, cx, cy, float32(weapon.Catalog[weapon.CatMine].TriggerRadius*scale), float32(1*g.dpr), ring, true)
		}
		if glow {
			r *= 1.6 // a fatter, softer core feeds the bloom
		}
		vector.FillCircle(dst, cx, cy, r, mineColor, true)
	}
}
