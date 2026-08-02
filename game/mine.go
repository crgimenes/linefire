package game

import (
	"image/color"

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
// blast when something it answers to wanders within trigger range. Its payload
// (damage/radius/trigger/color) comes from the weapon that laid it.
//
// A skirmish mine also knows WHOSE it is, and that is the whole of how it
// plays: the trigger is aimed (a fleet walks over its own mines and nothing
// happens), the blast is blind (once it goes off it hurts whoever is there,
// the layer's own wingman included). Same rule the missile and the laser
// already split along — see shiparms.go.
type mine struct {
	x, y    float64
	arm     int        // frames until armed; >0 = arming (blinking), 0 = live
	dmg     int        // blast center damage, in player-health units (campaign)
	hullDmg int        // blast center damage, in hull hits (skirmish)
	radius  float64    // AoE radius
	trigger float64    // proximity distance that sets it off
	col     color.RGBA // AoE damage-number color
	faction int        // the fleet that laid it; 0 = the player's own
	sid     int        // the hull that laid it, for the trace
}

// stepMines arms laid mines and detonates any armed mine something has
// wandered into, removing the ones that go off.
func (g *Game) stepMines() {
	kept := g.mines[:0]
	for i := range g.mines {
		m := g.mines[i]
		if m.arm > 0 {
			m.arm--
			kept = append(kept, m)
			continue
		}
		if g.mineTriggered(&m) {
			g.detonateMine(&m)
			continue // detonated: drop it
		}
		kept = append(kept, m)
	}
	g.mines = kept
}

// mineTriggered reports whether anything this mine answers to is close enough
// to set it off. A fleet knows where it laid its own, so its hulls cross them
// safely — what the blast then does to them is another matter.
func (g *Game) mineTriggered(m *mine) bool {
	if !g.skirmishMode {
		return g.enemyWithin(m.x, m.y, m.trigger)
	}
	r2 := m.trigger * m.trigger
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy || e.hp <= 0 || e.faction == m.faction {
			continue
		}
		dx, dy := e.x-m.x, e.y-m.y
		if dx*dx+dy*dy <= r2 {
			return true
		}
	}
	return false
}

// detonateMine sets the trap off in whichever game it was laid in: the
// campaign's player-centric blast, or the fleet blast that credits kills.
func (g *Game) detonateMine(m *mine) {
	if !g.skirmishMode {
		g.explodeAt(m.x, m.y, m.dmg, m.radius, m.col)
		return
	}
	g.shipBlast(m.x, m.y, m.radius, m.hullDmg, blastSource{faction: m.faction, sid: m.sid, col: m.col})
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
		// In skirmish colour says FACTION, never type: a trap you cannot tell
		// the owner of is a trap nobody can play around. The blink still says
		// "arming" — that reading is in the blinking, not in the hue.
		body, arming := mineColor, mineArmingColor
		if g.skirmishMode {
			body, arming = m.col, m.col
		}

		if m.arm > 0 {
			if (m.arm/mineBlinkPeriod)%2 == 0 {
				vector.FillCircle(dst, cx, cy, r, arming, true)
			}
			continue
		}
		if !glow {
			ring := body
			ring.A = 0x50
			vector.StrokeCircle(dst, cx, cy, float32(m.trigger*scale), float32(1*g.dpr), ring, true)
		}
		if glow {
			r *= 1.6 // a fatter, softer core feeds the bloom
		}
		vector.FillCircle(dst, cx, cy, r, body, true)
	}
}
