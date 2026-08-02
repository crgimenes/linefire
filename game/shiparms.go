package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/crgimenes/linefire/render"
	"github.com/crgimenes/linefire/weapon"
)

// A ship's arsenal: one salvaged weapon at a time, and the whole reason a
// canister on the field is worth dying for. A hull with no weapon fires its
// archetype's bolt; a hull that has salvaged one fires THAT, and the two
// weapons a ship can carry play very differently:
//
//	missile  slow, heavy, and it explodes. The blast does not ask whose ship
//	         it is — it damages the shooter's own fleet too, which is what
//	         makes an area weapon a decision rather than an upgrade.
//	laser    instant, no travel time, and it burns every foe standing on the
//	         firing line. The prize: a fleet that lines up against a laser
//	         loses a row at a time.
//
// The catalog's Damage is in PLAYER-health units (a missile does 4 to a ship
// that has 100 health). Hulls count hits, so a ship's weapon is priced here in
// hull units instead — the same scale one salvaged damage mod moves.
const (
	shipMissileHullDamage = 3 // at the centre of the blast, falling off to 1 at the rim
	shipBeamHullDamage    = 1 // per target on the line, and it hits all of them
	shipBeamFrames        = 6 // how long a beam is drawn after it burns
	shipBeamHitRadius     = 14
)

// shipBeam is a laser burn drawn for a few frames after it has already done
// its damage: a beam is instant, so what is on screen is its afterimage.
type shipBeam struct {
	x1, y1 float64
	x2, y2 float64
	col    color.RGBA
	life   int
}

// carriedWeapon is the catalog entry this hull fires, or nil for its archetype
// bolt.
func carriedWeapon(key string) *weapon.Weapon {
	c := catForKey(key)
	if c < 0 {
		return nil
	}
	return &weapon.Catalog[c]
}

// shipCanCarry reports whether a ship can actually FIRE a weapon, which is the
// only reason to let it salvage one. Mines and the devourer are deployed, not
// aimed, and a hull has no deployment behaviour yet — so they stay on the
// field rather than being carried unused.
func shipCanCarry(key string) bool {
	w := carriedWeapon(key)
	if w == nil {
		return false
	}
	return w.Kind == weapon.KindProjectile || w.Kind == weapon.KindLaser
}

// fireCarried fires the hull's salvaged weapon at a point, and reports whether
// it handled the shot (an unarmed hull falls back to its archetype volley).
func (g *Game) fireCarried(e *entity, tx, ty float64) bool {
	w := carriedWeapon(e.weaponKey)
	if w == nil {
		return false
	}
	switch w.Kind {
	case weapon.KindLaser:
		g.fireShipBeam(e, tx, ty, w)
	case weapon.KindProjectile:
		g.fireShipProjectile(e, tx, ty, w)
	default:
		return false // deployed weapons: not something a hull knows how to use
	}
	g.match.recordShot(e.faction)
	g.traceShot(e, tx, ty)
	g.playEvent(e.a, "fire")
	return true
}

// fireShipProjectile sends the weapon's own shot: the missile's look, speed and
// blast rather than the hull's bolt.
func (g *Game) fireShipProjectile(e *entity, tx, ty float64, w *weapon.Weapon) {
	dx, dy := tx-e.x, ty-e.y
	d := math.Hypot(dx, dy)
	if d == 0 {
		return
	}
	speed := w.Speed
	if speed <= 0 {
		speed = enemyBulletSpeed
	}
	life := w.Life
	if life <= 0 {
		life = enemyBulletLife
	}
	g.enemyShots = append(g.enemyShots, projectile{
		x: e.x, y: e.y, px: e.x, py: e.y,
		vx:      dx / d * speed,
		vy:      dy / d * speed,
		life:    life,
		dmg:     e.shotDmg,
		hullDmg: shipMissileHullDamage + e.dmgMod,
		aoe:     w.AOE,
		faction: e.faction,
		sid:     e.id,
		rcol:    w.Core, rglow: w.Glow, width: w.Width, glowW: w.GlowWidth,
	})
}

// shipBlast is a ship weapon's explosion. It damages EVERY hull in reach, the
// shooter's own fleet included: an explosion does not ask whose ship it is.
// That is the price of carrying an area weapon, and it is where a fleet flying
// in formation learns to spread out.
func (g *Game) shipBlast(x, y float64, p *projectile) {
	g.emitBurst(x, y, missileBlast)
	g.emitBurst(x, y, missileBlastChunks)
	g.spawnShockwave(x, y, p.aoe)
	g.playEvent(nil, "explosion")
	g.digBlast(x, y, p.aoe)

	for i := len(g.entities) - 1; i >= 0; i-- {
		e := &g.entities[i]
		if e.kind != kindEnemy || e.hp <= 0 {
			continue
		}
		d := math.Hypot(e.x-x, e.y-y)
		if d > p.aoe {
			continue
		}
		hull := max(int(math.Ceil(float64(p.hullDmg)*(1-d/p.aoe))), 1)
		victim := e.faction
		if !g.damageEnemy(i, hull, p.rglow, p.sid) {
			continue
		}
		// A fleet that blows up its own does not get to call it a kill; the
		// loss still counts, which is the whole lesson.
		by := p.faction
		if victim == by {
			by = 0
		}
		g.match.recordKill(by, victim)
	}
}

// fireShipBeam burns a line from the ship toward the target: instant, stopped
// by rock, and every foe standing on it takes the hit. Allies are NOT burned —
// a beam is aimed, and a fleet would never fire through its own — which is the
// opposite of how the blast behaves, deliberately.
func (g *Game) fireShipBeam(e *entity, tx, ty float64, w *weapon.Weapon) {
	dx, dy := tx-e.x, ty-e.y
	d := math.Hypot(dx, dy)
	if d == 0 {
		return
	}
	dx, dy = dx/d, dy/d
	reach := w.Reach
	if reach <= 0 {
		reach = 2000
	}
	x2, y2, _ := g.laserEndpoint(e.x, e.y, dx, dy, reach) // rock stops the beam; where it stopped is all we need

	hull := shipBeamHullDamage + e.dmgMod
	_, glow := factionShotColors(e.faction)
	for i := len(g.entities) - 1; i >= 0; i-- {
		o := &g.entities[i]
		if o.kind != kindEnemy || o.hp <= 0 || o.faction == e.faction {
			continue
		}
		if distPointSegmentSq(o.x, o.y, e.x, e.y, x2, y2) > (o.radius+shipBeamHitRadius)*(o.radius+shipBeamHitRadius) {
			continue
		}
		victim := o.faction
		if g.damageEnemy(i, hull, glow, e.id) {
			g.match.recordKill(e.faction, victim)
		}
	}
	g.beams = append(g.beams, shipBeam{x1: e.x, y1: e.y, x2: x2, y2: y2, col: glow, life: shipBeamFrames})
}

// stepShipBeams ages the drawn afterimages.
func (g *Game) stepShipBeams() {
	kept := g.beams[:0]
	for _, b := range g.beams {
		b.life--
		if b.life > 0 {
			kept = append(kept, b)
		}
	}
	g.beams = kept
}

// drawShipBeams draws the burns, fading as they age. They go ABOVE the hulls
// with the other fire: a beam crosses other ships, and one ducking behind a
// hull would read as a miss.
func (g *Game) drawShipBeams(dst *ebiten.Image, cam ebiten.GeoM, glow bool) {
	width := 1.8
	if glow {
		width = 3.6
	}
	for i := range g.beams {
		b := &g.beams[i]
		x0, y0 := cam.Apply(b.x1, b.y1)
		x1, y1 := cam.Apply(b.x2, b.y2)
		col := b.col
		col.A = uint8(float64(col.A) * float64(b.life) / shipBeamFrames)
		render.StrokeLine(dst, x0, y0, x1, y1, width*g.dpr, col)
		render.FillCircle(dst, x1, y1, width*g.dpr, col)
	}
}
