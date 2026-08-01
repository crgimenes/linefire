package game

import "github.com/crgimenes/linefire/weapon"

// Weapon pickups keep the game moving: a weapon lies on the map (a colored diamond);
// flying over it COLLECTS it into the arsenal (arsenal.go), arming the first empty slot,
// no dropping and no screen. Nothing is ever swapped out — the only decision is the
// piloting one, fly over it or around it.

// weaponKeys names each catalog weapon for data (asset kinds "weapon-<key>", entity
// power, drops).
var weaponKeys = [...]string{
	weapon.CatFront:    "front",
	weapon.CatMissile:  "missile",
	weapon.CatMine:     "mine",
	weapon.CatLaser:    "laser",
	weapon.CatDevourer: "devourer",
}

// catForKey resolves a weapon key back to its catalog index, or -1.
func catForKey(key string) int {
	for c, k := range weaponKeys {
		if k == key {
			return c
		}
	}
	return -1
}

// resolveWeaponPickups collects weapons the player flies over. A weapon already in the
// arsenal is left on the map (nothing to gain); a new one is taken and consumed for the
// run. Fires once per entry (rising edge), so sitting on a pickup does not churn.
func (g *Game) resolveWeaponPickups() {
	if g.skirmishMode || g.over {
		return // no player: a dropped weapon stays on the field
	}
	kept := g.entities[:0]
	for i := range g.entities {
		e := g.entities[i]
		if e.kind != kindWeapon {
			kept = append(kept, e)
			continue
		}
		rr := g.radius + e.radius
		dx, dy := g.x-e.x, g.y-e.y
		nowInside := dx*dx+dy*dy <= rr*rr
		entered := nowInside && !e.inside
		e.inside = nowInside
		cat := catForKey(e.power)
		if !entered || cat < 0 {
			kept = append(kept, e)
			continue
		}
		isNew := g.collectWeapon(cat)
		// The DEVOURER is charge-based: every pickup grants charges and is consumed, even a
		// refill of one already owned. Ordinary weapons already owned are left on the map.
		if cat == weapon.CatDevourer {
			g.devourerAmmo += devourerCharges
			g.logf("PICKUP  ► DEVOURER ONLINE // %d charges", g.devourerAmmo)
		} else if !isNew {
			kept = append(kept, e) // already owned, nothing to gain: leave it
			continue
		} else {
			g.logf("PICKUP  %s", weaponKeys[cat])
		}
		g.emitBurst(e.x, e.y, pickupMotes)
		g.playEvent(e.a, "pickup")
		g.markConsumed(e.spawn)
	}
	g.entities = kept
}
