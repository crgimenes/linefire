package game

// Pickups are destructible: enough PLAYER fire breaks a power-up or a weapon on the
// ground — no score, just gone (and gone for the run). It cuts
// both ways by design: shoot a pickup out of your path instead of swapping into
// it, or be careful NOT to hit the one weapon you need. Enemy fire, blasts and
// the laser leave pickups alone (v1): only deliberate direct hits break things.

const pickupHP = 6 // direct player hits a power-up/weapon takes (~1s of focused fire)

// bulletHitsPickup returns the index of the first breakable pickup entity whose
// circle the travel segment crosses, or -1. Enemies are checked first by the
// caller, so a shot never favors a crate over a threat.
func (g *Game) bulletHitsPickup(ax, ay, bx, by float64) int {
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindPowerUp && e.kind != kindWeapon {
			continue
		}
		if distPointSegmentSq(e.x, e.y, ax, ay, bx, by) <= e.radius*e.radius {
			return i
		}
	}
	return -1
}

// damagePickup chips a pickup entity; on breaking it is removed for the run — no
// score, no reward shake, just a burst and a crack.
func (g *Game) damagePickup(i, dmg int) {
	e := &g.entities[i]
	e.hp -= dmg
	if e.hp > 0 {
		e.hitFlash = hitFlashFrames
		return
	}
	g.playEvent(e.a, "break")
	g.emitBurst(e.x, e.y, pickupMotes)
	g.markConsumed(e.spawn) // stays destroyed if the player revisits this map
	label := e.power
	if cat := catForKey(e.power); cat >= 0 {
		label = weaponKeys[cat]
	}
	g.logf("DESTROYED  %s", label)
	g.entities = append(g.entities[:i], g.entities[i+1:]...)
}
