package game

import "testing"

// salvageGame is one ship of faction 1 and whatever pickups a test drops
// around it, in a bare skirmish world.
func salvageGame(t *testing.T) *Game {
	t.Helper()
	g := &Game{}
	g.skirmishMode = true
	g.bounds = bounds{minX: 0, minY: 0, maxX: 1000, maxY: 1000}
	g.match, _ = newMatch(MatchLastFleet, 4, 0, 2)
	g.entities = []entity{{
		kind: kindEnemy, id: 1, x: 500, y: 500, radius: 12,
		hp: 2, hpMax: 4, faction: 1, radar: 400, fireEvery: 60,
	}}
	return g
}

func pickup(power string, x, y float64) entity {
	return entity{kind: kindPowerUp, power: power, x: x, y: y, radius: 8, hp: pickupHP}
}

// A ship flying over a repair takes it, and a hull is never repaired past what
// it was built with.
func TestSalvageRepairsUpToTheHullMax(t *testing.T) {
	g := salvageGame(t)
	g.entities = append(g.entities, pickup(powerHeal, 505, 500))
	g.resolveSalvage()

	e := &g.entities[0]
	if e.hp != 4 {
		t.Fatalf("hull %d after a repair of %d on a 2/4 hull, want 4", e.hp, shipHealAmount)
	}
	if len(g.entities) != 1 {
		t.Fatalf("the pickup should be consumed, %d entities left", len(g.entities))
	}
	if got := g.match.Stats[1].Powerups; got != 1 {
		t.Errorf("the fleet's salvage count is %d, want 1", got)
	}

	// Already whole: a second repair cannot push it past its maximum.
	g.entities = append(g.entities, pickup(powerHeal, 505, 500))
	g.resolveSalvage()
	if e.hp != 4 {
		t.Fatalf("hull %d, want it capped at 4", e.hp)
	}
}

// A shield absorbs damage before the hull, and a hull behind a shield that
// held is not hurt at all.
func TestSalvagedShieldAbsorbsFirst(t *testing.T) {
	g := salvageGame(t)
	g.entities = append(g.entities, pickup(powerShield, 500, 505))
	g.resolveSalvage()

	e := &g.entities[0]
	if e.shield != shipShieldAmount {
		t.Fatalf("shield %d after one canister, want %d", e.shield, shipShieldAmount)
	}
	hullBefore := e.hp
	if killed := g.damageEnemy(0, 2, damageColor, 0); killed {
		t.Fatal("a shielded ship should not die to a glancing hit")
	}
	if e.hp != hullBefore {
		t.Fatalf("hull fell to %d behind a shield that held", e.hp)
	}
	if e.shield != shipShieldAmount-2 {
		t.Fatalf("shield %d after absorbing 2, want %d", e.shield, shipShieldAmount-2)
	}
	// More than the shield can take: the overflow reaches the hull.
	g.damageEnemy(0, 3, damageColor, 0)
	if e.shield != 0 || e.hp != hullBefore-2 {
		t.Fatalf("overflow wrong: shield=%d hull=%d (was %d)", e.shield, e.hp, hullBefore)
	}
}

// Combat mods change what the hull actually does: harder bolts, a shorter
// interval between volleys, and more bolts in one.
func TestSalvagedModsChangeTheVolley(t *testing.T) {
	g := salvageGame(t)
	base := g.entities[0].fireInterval()

	g.entities = append(g.entities,
		pickup(powerDamage, 500, 500), pickup(powerRate, 500, 500), pickup(powerFire, 500, 500))
	g.resolveSalvage()

	e := &g.entities[0] // taken after the appends: they may have moved the slice
	if e.dmgMod != 1 || e.rateMod != 1 || e.fireMod != 1 {
		t.Fatalf("mods not applied: %+v", *e)
	}
	if got := e.fireInterval(); got >= base {
		t.Errorf("a rate mod should shorten the interval: %d -> %d", base, got)
	}

	g.enemyFire(e, 900, 500)
	if len(g.enemyShots) != 2 {
		t.Fatalf("a fire mod should widen the volley to 2 bolts, got %d", len(g.enemyShots))
	}
	if got := g.enemyShots[0].hullDmg; got != shipShotHullDamage+1 {
		t.Errorf("a damage mod should harden the bolt: hullDmg=%d", got)
	}
	// The fan straddles the aim rather than pointing one bolt sideways.
	a, b := g.enemyShots[0], g.enemyShots[1]
	if a.vy >= 0 || b.vy <= 0 {
		t.Errorf("the two bolts should straddle the aim, got vy %.2f and %.2f", a.vy, b.vy)
	}

	// The cap holds however much a hull salvages.
	for range 10 {
		g.entities = append(g.entities, pickup(powerDamage, 500, 500))
		g.resolveSalvage()
	}
	if got := g.entities[0].dmgMod; got != shipMaxMod {
		t.Errorf("damage mod %d, want it capped at %d", got, shipMaxMod)
	}
}

// Loot is only worth racing for if a program can see it — under the same rules
// its sensors use for ships.
func TestLootIsVisibleUnderSensorRules(t *testing.T) {
	g := salvageGame(t)
	g.entities = append(g.entities,
		pickup(powerShield, 700, 500), // in range, clear
		pickup(powerHeal, 500, 100),   // in range, behind a wall
		pickup(powerHeal, 500, 950),   // beyond the radar
	)
	g.segs = []segment{{ax: 400, ay: 300, bx: 600, by: 300}}

	loot := g.lootContacts(&g.entities[0])
	if len(loot) != 1 {
		t.Fatalf("saw %d pickups, want only the clear one in range: %+v", len(loot), loot)
	}
	if loot[0].kind != powerShield || loot[0].dist != 200 {
		t.Fatalf("wrong contact: %+v", loot[0])
	}
}

// A ship takes any weapon that would change what it does — the dropped ones
// included, now that a hull knows how to lay them — and leaves the front gun,
// which is what it already fires.
func TestOnlyWeaponsThatChangeTheHullAreSalvaged(t *testing.T) {
	g := salvageGame(t)
	for _, key := range []string{"missile", "laser", "mine", "devourer"} {
		w := pickup(key, 500, 500)
		w.kind = kindWeapon
		g.entities = append(g.entities, w)
		g.resolveSalvage()

		if got := g.entities[0].weaponKey; got != key {
			t.Fatalf("the ship carries %q, want the %s it flew over", got, key)
		}
		if len(g.entities) != 1 {
			t.Fatalf("the %s should be consumed off the field", key)
		}
	}

	// The front gun stays where it fell: taking it would change nothing.
	front := pickup("front", 500, 500)
	front.kind = kindWeapon
	g.entities = append(g.entities, front)
	g.resolveSalvage()
	if len(g.entities) != 2 {
		t.Fatal("the front gun is what an unarmed hull already fires: it should stay on the field")
	}
	if got := g.entities[0].weaponKey; got != "devourer" {
		t.Fatalf("the ship swapped to %q; the front gun should not have been taken", got)
	}
	// It is still visible either way: a program sees the field, not the rules.
	if loot := g.lootContacts(&g.entities[0]); len(loot) != 1 {
		t.Fatalf("a weapon on the field should be visible loot, got %+v", loot)
	}
}

// Canisters break under ship fire exactly as they break under the player's:
// chipped per hit, gone at pickupHP, and the bolt is spent on the crate.
func TestShipFireBreaksACanister(t *testing.T) {
	g := salvageGame(t)
	g.entities = append(g.entities, pickup(powerHeal, 700, 500))

	for range pickupHP {
		g.enemyShots = []projectile{{
			x: 650, y: 500, px: 650, py: 500, vx: 20, vy: 0, life: 10,
			faction: 2, sid: 9,
		}}
		for range 5 { // let the bolt cover the distance to the crate
			g.stepEnemyShots()
		}
		if len(g.enemyShots) != 0 {
			t.Fatal("the bolt should be spent on the crate it hit")
		}
	}
	if len(g.entities) != 1 {
		t.Fatalf("%d entities left: %d direct hits should have broken the canister", len(g.entities), pickupHP)
	}
}

// Hulls are checked first: a bolt whose path crosses a canister BEFORE the foe
// behind it still lands on the foe — a shot never favors a crate over a threat.
func TestABoltNeverFavorsACrateOverAThreat(t *testing.T) {
	g := salvageGame(t)
	g.entities = append(g.entities, pickup(powerHeal, 600, 500)) // in the bolt's path, nearer than the ship

	g.enemyShots = []projectile{{
		x: 700, y: 500, px: 700, py: 500, vx: -250, vy: 0, life: 10,
		faction: 2, sid: 9, hullDmg: 1,
	}}
	g.stepEnemyShots() // one segment over the canister at 600 and the faction-1 ship at 500

	if len(g.entities) != 2 {
		t.Fatalf("%d entities left, want the wounded ship and the untouched canister", len(g.entities))
	}
	if g.entities[0].hp != 1 {
		t.Errorf("the ship's hull is %d, want 1: the bolt should have hit IT", g.entities[0].hp)
	}
	if g.entities[1].hp != pickupHP {
		t.Errorf("the canister was chipped to %d; the threat behind it should have taken the bolt", g.entities[1].hp)
	}
}

// Campaign parity: outside skirmish, enemy fire leaves pickups alone — the v1
// rule that only the player's deliberate hits break things.
func TestCampaignEnemyFireLeavesPickupsAlone(t *testing.T) {
	g := salvageGame(t)
	g.skirmishMode = false
	g.invuln = 1 // park the player test: this is about the pickup
	g.entities = append(g.entities, pickup(powerHeal, 700, 500))

	g.enemyShots = []projectile{{
		x: 650, y: 500, px: 650, py: 500, vx: 20, vy: 0, life: 10,
	}}
	g.stepEnemyShots()

	if len(g.entities) != 2 {
		t.Fatal("campaign enemy fire must not break pickups")
	}
	if g.entities[1].hp != pickupHP {
		t.Errorf("the pickup was chipped to %d by enemy fire; the campaign rule leaves it whole", g.entities[1].hp)
	}
}

// A missile into a crate detonates AND breaks the crate — and the order
// matters: the blast kills, killing moves g.entities, and the pickup index
// only holds while nothing has moved. The reversed order panicked in a real
// battle. The dying hull sits at a LOWER index than the crate on purpose, so
// a stale index would reach past the shrunk slice again.
func TestAMissileIntoACrateBesideADyingHull(t *testing.T) {
	g := salvageGame(t)
	victim := foe(2, 690, 500, 1) // one hit from dead, inside the blast, BEFORE the crate in the slice
	victim.spawn = -1
	crate := pickup(powerHeal, 700, 500)
	crate.hp = 1 // one chip from broken
	g.entities = append(g.entities, victim, crate)

	g.enemyShots = []projectile{{
		x: 680, y: 500, px: 680, py: 500, vx: 25, vy: 0, life: 10,
		faction: 2, sid: 9, hullDmg: shipMissileHullDamage, aoe: 70,
	}}
	g.stepEnemyShots()

	for i := range g.entities {
		switch {
		case g.entities[i].id == 2:
			t.Fatal("the hull beside the crate survived the blast")
		case g.entities[i].kind == kindPowerUp:
			t.Fatal("the crate survived a direct missile")
		}
	}
}
