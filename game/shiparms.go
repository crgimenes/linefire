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
// archetype's bolt; a hull that has salvaged one fires THAT, and the four
// weapons a ship can carry play very differently:
//
//	missile   slow, heavy, and it explodes. The blast does not ask whose ship
//	          it is — it damages the shooter's own fleet too, which is what
//	          makes an area weapon a decision rather than an upgrade.
//	laser     instant, no travel time, and it burns every foe standing on the
//	          firing line. The prize: a fleet that lines up against a laser
//	          loses a row at a time.
//	mine      not fired at all: LAID where the ship stands, and it waits. A
//	          fleet walks over its own mines safely and nobody else does.
//	devourer  a black hole dropped at the ship's own feet. It plays no
//	          favourites, so firing it while your fleet is around you is how a
//	          fleet ends. The rarest thing on the field, and a trap as much as
//	          a weapon.
//
// The two aimed weapons and the two dropped ones divide along one rule, and it
// is worth stating once because everything here follows it:
//
//	a TRIGGER is aimed — a beam is pointed, a mine reads only foes;
//	a BLAST is blind — it burns whoever is standing there, allies included.
//
// The catalog's Damage is in PLAYER-health units (a missile does 4 to a ship
// that has 100 health). Hulls count hits, so a ship's weapon is priced here in
// hull units instead — the same scale one salvaged damage mod moves.
const (
	shipMissileHullDamage  = 3 // at the centre of the blast, falling off to 1 at the rim
	shipBeamHullDamage     = 1 // per target on the line, and it hits all of them
	shipMineHullDamage     = 3 // a trap you walked into: the same as taking a missile
	shipDevourerHullDamage = 8 // the implosion: far past any hull, so only the rim survives
	shipBeamFrames         = 6 // how long a beam is drawn after it burns
	shipBeamHitRadius      = 14
)

// blastSource is who an explosion belongs to: the fleet that takes the credit
// and the hull the trace attributes it to. They are two different numbers that
// are both small ints, and reading one as the other has already cost this
// project a real bug — naming them here is what keeps them apart.
type blastSource struct {
	faction int
	sid     int
	col     color.RGBA
}

// blastSource is who this shot's explosion answers to.
func (p *projectile) blastSource() blastSource {
	return blastSource{faction: p.faction, sid: p.sid, col: p.rglow}
}

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

// shipCanCarry reports whether salvaging this weapon would change what the
// hull does, which is the only reason to let it take one off the field. Every
// catalogue weapon now has a hull behaviour — the aimed ones are fired at a
// point, the dropped ones are laid where the ship stands — except the front
// gun, which is what an unarmed hull already fires.
func shipCanCarry(key string) bool {
	c := catForKey(key)
	return c >= 0 && c != weapon.CatFront
}

// fireCarried fires the hull's salvaged weapon at a point, and reports whether
// it handled the shot (an unarmed hull falls back to its archetype volley).
//
// The shooter is read into a shot BEFORE anything is fired, and the bookkeeping
// below uses the copy. A laser kills inside its own call, damageEnemy closes
// the slice up, and a pointer into that slice then names whichever hull slid
// into the slot — which is how a real battle came to record a shot fired by a
// ship that never existed.
func (g *Game) fireCarried(e *entity, tx, ty float64) bool {
	w := carriedWeapon(e.weaponKey)
	if w == nil {
		return false
	}
	shooter := *e
	switch w.Kind {
	case weapon.KindLaser:
		g.fireShipBeam(e, tx, ty, w)
	case weapon.KindProjectile:
		g.fireShipProjectile(e, tx, ty, w)
	case weapon.KindMine:
		if !g.deployShipMine(e, w) {
			return false // the fleet is at its cap: this hull falls back to its bolt
		}
		tx, ty = shooter.x, shooter.y // a dropped weapon goes where the ship is, not where it aimed
	case weapon.KindDevourer:
		if !g.deployShipDevourer(e, w) {
			return false // one hole on the field at a time; until it collapses, bolts
		}
		tx, ty = shooter.x, shooter.y
	default:
		return false
	}
	g.match.recordShot(shooter.faction)
	g.traceShot(&shooter, tx, ty)
	g.playEvent(shooter.a, "fire")
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

// shipBlast is a ship weapon's explosion — a missile's, a mine's, or the
// devourer's implosion. It damages EVERY hull in reach, the source's own fleet
// included: an explosion does not ask whose ship it is. That is the price of
// an area weapon, and it is where a fleet flying in formation learns to spread
// out.
func (g *Game) shipBlast(x, y, aoe float64, hullDmg int, src blastSource) {
	g.emitBurst(x, y, missileBlast)
	g.emitBurst(x, y, missileBlastChunks)
	g.spawnShockwave(x, y, aoe)
	g.playEvent(nil, "explosion")
	g.digBlast(x, y, aoe)

	for i := len(g.entities) - 1; i >= 0; i-- {
		e := &g.entities[i]
		if e.kind != kindEnemy || e.hp <= 0 {
			continue
		}
		d := math.Hypot(e.x-x, e.y-y)
		if d > aoe {
			continue
		}
		hull := max(int(math.Ceil(float64(hullDmg)*(1-d/aoe))), 1)
		victim := e.faction
		if !g.damageEnemy(i, hull, src.col, src.sid) {
			continue
		}
		// A fleet that blows up its own does not get to call it a kill; the
		// loss still counts, which is the whole lesson.
		by := src.faction
		if victim == by {
			by = 0
		}
		g.match.recordKill(by, victim)
	}
}

// deployShipMine lays a mine where the ship stands, if its fleet is below the
// cap. The catalogue's MaxLive is a limit on ONE arsenal; here it is per
// faction, or eight fleets would be sharing five mines between them.
func (g *Game) deployShipMine(e *entity, w *weapon.Weapon) bool {
	live := 0
	for i := range g.mines {
		if g.mines[i].faction == e.faction {
			live++
		}
	}
	if live >= w.MaxLive {
		return false
	}
	_, glow := factionShotColors(e.faction)
	g.mines = append(g.mines, mine{
		x: e.x, y: e.y, arm: w.ArmFrames,
		dmg: w.Damage, hullDmg: shipMineHullDamage + e.dmgMod,
		radius: w.AOE, trigger: w.TriggerRadius, col: glow,
		faction: e.faction, sid: e.id,
	})
	g.sfx.play(soundOf(w.Fire))
	return true
}

// deployShipDevourer drops the black hole at the ship and SPENDS the weapon:
// the canister is a single decision, not a mounted gun. There is one hole on
// the field at a time, and it plays no favourites — a fleet that fires this
// while flying in formation is a fleet that just ended itself. Seeing the
// canister in the loot list and steering around it is a legitimate way to
// play; so is picking it up and holding fire until the enemy is close.
func (g *Game) deployShipDevourer(e *entity, w *weapon.Weapon) bool {
	if g.devourer != nil {
		return false
	}
	g.devourer = &devourer{
		x: e.x, y: e.y, arm: devourerArm, active: devourerActive,
		by: e.faction, sid: e.id,
	}
	e.weaponKey = "" // spent: the hull goes back to its own bolt
	g.sfx.play(soundOf(w.Fire))
	return true
}

// fireShipBeam burns a line from the ship toward the target: instant, stopped
// by rock, and every foe standing on it takes the hit. Allies are NOT burned —
// a beam is aimed, and a fleet would never fire through its own — which is the
// opposite of how the blast behaves, deliberately.
// Everything the beam needs about its shooter is read out before the first
// hull is hurt: damageEnemy moves the slice, and e would stop naming the ship
// that pulled the trigger halfway down the line of fire.
func (g *Game) fireShipBeam(e *entity, tx, ty float64, w *weapon.Weapon) {
	ex, ey := e.x, e.y
	faction, sid := e.faction, e.id
	hull := shipBeamHullDamage + e.dmgMod

	dx, dy := tx-ex, ty-ey
	d := math.Hypot(dx, dy)
	if d == 0 {
		return
	}
	dx, dy = dx/d, dy/d
	reach := w.Reach
	if reach <= 0 {
		reach = 2000
	}
	x2, y2, _ := g.laserEndpoint(ex, ey, dx, dy, reach) // rock stops the beam; where it stopped is all we need

	_, glow := factionShotColors(faction)
	for i := len(g.entities) - 1; i >= 0; i-- {
		o := &g.entities[i]
		if o.kind != kindEnemy || o.hp <= 0 || o.faction == faction {
			continue
		}
		if distPointSegmentSq(o.x, o.y, ex, ey, x2, y2) > (o.radius+shipBeamHitRadius)*(o.radius+shipBeamHitRadius) {
			continue
		}
		victim := o.faction
		if g.damageEnemy(i, hull, glow, sid) {
			g.match.recordKill(faction, victim)
		}
	}
	g.beams = append(g.beams, shipBeam{x1: ex, y1: ey, x2: x2, y2: y2, col: glow, life: shipBeamFrames})
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
