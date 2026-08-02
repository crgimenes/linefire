package game

import (
	"testing"

	"github.com/crgimenes/linefire/weapon"
)

// armedGame is one armed ship of faction 1 and whatever else a test places.
func armedGame(t *testing.T, key string) *Game {
	t.Helper()
	g := salvageGame(t)
	g.entities[0].weaponKey = key
	return g
}

func foe(id int, x, y float64, hp int) entity {
	return entity{kind: kindEnemy, id: id, x: x, y: y, radius: 12, hp: hp, hpMax: hp, faction: 2}
}

// A carried weapon replaces the archetype bolt: the missile flies with the
// catalog's own speed and look, and carries a blast.
func TestCarriedMissileReplacesTheBolt(t *testing.T) {
	g := armedGame(t, "missile")
	g.enemyFire(&g.entities[0], 900, 500)

	if len(g.enemyShots) != 1 {
		t.Fatalf("one missile, got %d shots", len(g.enemyShots))
	}
	s := g.enemyShots[0]
	if s.aoe <= 0 {
		t.Fatal("a missile must carry a blast radius")
	}
	if s.hullDmg != shipMissileHullDamage {
		t.Errorf("missile hull damage %d, want %d", s.hullDmg, shipMissileHullDamage)
	}
	if s.faction != 1 || s.sid != 1 {
		t.Errorf("the missile lost its owner: %+v", s)
	}
}

// The blast does not ask whose ship it is. That is what makes an area weapon a
// decision: it kills the foe it was aimed at AND the ally standing too close,
// and the shooter's fleet is not credited with killing its own.
func TestBlastHurtsEveryoneInReach(t *testing.T) {
	g := armedGame(t, "missile")
	ally := foe(2, 620, 500, 1)
	ally.faction = 1 // the shooter's own fleet, right beside the target
	g.entities = append(g.entities, foe(3, 600, 500, 1), ally)

	g.enemyShots = []projectile{{
		x: 590, y: 500, px: 590, py: 500, vx: 20, vy: 0, life: 10,
		hullDmg: shipMissileHullDamage, aoe: 70, faction: 1, sid: 1,
	}}
	g.stepEnemyShots()

	for _, id := range []int{2, 3} {
		for i := range g.entities {
			if g.entities[i].id == id {
				t.Fatalf("ship %d survived a blast it was standing in", id)
			}
		}
	}
	if got := g.match.Stats[1].Kills; got != 1 {
		t.Errorf("faction 1 is credited %d kills; only the FOE counts", got)
	}
	if got := g.match.Stats[1].Losses; got != 1 {
		t.Errorf("faction 1 recorded %d losses; blowing up its own still costs it", got)
	}
}

// The laser is the prize: instant, and it burns every foe on the firing line —
// a fleet that lines up against one loses a row at a time.
func TestBeamBurnsEveryFoeOnTheLine(t *testing.T) {
	g := armedGame(t, "laser")
	g.entities = append(g.entities,
		foe(2, 600, 500, 1), // on the line
		foe(3, 700, 500, 1), // also on the line, behind the first
		foe(4, 700, 700, 1), // off the line
	)
	ally := foe(5, 650, 500, 1)
	ally.faction = 1 // an ally ON the line: a beam is aimed, it does not burn its own
	g.entities = append(g.entities, ally)

	g.enemyFire(&g.entities[0], 900, 500)

	alive := map[int]bool{}
	for i := range g.entities {
		alive[g.entities[i].id] = true
	}
	if alive[2] || alive[3] {
		t.Fatalf("both foes on the line should be burned: %v", alive)
	}
	if !alive[4] {
		t.Error("a ship off the firing line must not be hit")
	}
	if !alive[5] {
		t.Error("the beam burned its own fleet; only the blast does that")
	}
	if len(g.enemyShots) != 0 {
		t.Error("a beam is instant: it must not leave a projectile in flight")
	}
	if len(g.beams) != 1 {
		t.Fatalf("the burn should be drawn, got %d beams", len(g.beams))
	}
	if got := g.match.Stats[1].Kills; got != 2 {
		t.Errorf("faction 1 credited %d kills, want 2", got)
	}
}

// Rock stops a beam: what is behind a wall is not on the firing line.
func TestBeamIsStoppedByRock(t *testing.T) {
	g := armedGame(t, "laser")
	g.entities = append(g.entities, foe(2, 700, 500, 1))
	g.segs = []segment{{ax: 600, ay: 400, bx: 600, by: 600}}

	g.enemyFire(&g.entities[0], 900, 500)
	for i := range g.entities {
		if g.entities[i].id == 2 {
			return // survived behind the wall, as it should
		}
	}
	t.Fatal("the beam burned a ship standing behind rock")
}

// The beams fade rather than lingering forever.
func TestBeamsFadeOut(t *testing.T) {
	g := armedGame(t, "laser")
	g.enemyFire(&g.entities[0], 900, 500)
	for range shipBeamFrames + 1 {
		g.stepShipBeams()
	}
	if len(g.beams) != 0 {
		t.Fatalf("%d beams still drawn after they should have faded", len(g.beams))
	}
}

// A dropped weapon is LAID, not fired: it lands where the hull is standing,
// whatever point the program aimed at, and nothing leaves the barrel.
func TestMineIsLaidWhereTheShipStands(t *testing.T) {
	g := armedGame(t, "mine")
	g.enemyFire(&g.entities[0], 900, 500)

	if len(g.enemyShots) != 0 {
		t.Fatalf("%d shots fired: a mine is placed, not launched", len(g.enemyShots))
	}
	if len(g.mines) != 1 {
		t.Fatalf("%d mines laid, want 1", len(g.mines))
	}
	m := g.mines[0]
	if m.x != 500 || m.y != 500 {
		t.Errorf("the mine landed at (%v,%v), want the ship's own position", m.x, m.y)
	}
	if m.faction != 1 || m.sid != 1 {
		t.Errorf("the mine lost its owner: %+v", m)
	}
	if m.arm <= 0 {
		t.Error("a mine must arm before it can go off")
	}
}

// The catalogue's live-mine cap is one arsenal's; a fleet gets its own. Past
// it the hull falls back to its bolt rather than going quiet.
func TestMineCapIsPerFleet(t *testing.T) {
	g := armedGame(t, "mine")
	cap := weapon.Catalog[weapon.CatMine].MaxLive
	for range cap {
		g.enemyFire(&g.entities[0], 900, 500)
	}
	if len(g.mines) != cap {
		t.Fatalf("%d mines on the field, want the cap of %d", len(g.mines), cap)
	}

	g.enemyFire(&g.entities[0], 900, 500)
	if len(g.mines) != cap {
		t.Errorf("the cap did not hold: %d mines", len(g.mines))
	}
	if len(g.enemyShots) != 1 {
		t.Errorf("a capped hull should fall back to its bolt, got %d shots", len(g.enemyShots))
	}

	// Another fleet's mines are not this one's problem.
	other := foe(9, 400, 400, 2)
	other.weaponKey = "mine"
	g.entities = append(g.entities, other)
	g.enemyFire(&g.entities[1], 900, 500)
	if len(g.mines) != cap+1 {
		t.Errorf("faction 2 was refused a mine by faction 1's cap: %d mines", len(g.mines))
	}
}

// The trigger is AIMED: a fleet crosses its own mines and nothing happens.
// Anyone else sets one off.
func TestMineIgnoresTheFleetThatLaidIt(t *testing.T) {
	g := armedGame(t, "mine")
	g.enemyFire(&g.entities[0], 900, 500)
	g.mines[0].arm = 0 // armed and waiting

	wingman := foe(2, 505, 500, 2)
	wingman.faction = 1
	g.entities = append(g.entities, wingman)
	g.stepMines()
	if len(g.mines) != 1 {
		t.Fatal("a fleet's own hull set off its own mine")
	}

	g.entities = append(g.entities, foe(3, 500, 505, 2))
	g.stepMines()
	if len(g.mines) != 0 {
		t.Fatal("an enemy hull walked onto an armed mine and it did not go off")
	}
}

// The blast is BLIND: once the mine goes off it burns whoever is standing
// there, and the fleet that laid it is not spared — not even the hull that
// laid it, if it is still sitting on top of its own trap. It gets no credit
// for its own dead, but every loss is on its board.
func TestMineBlastBurnsTheFleetThatLaidIt(t *testing.T) {
	g := armedGame(t, "mine")
	g.enemyFire(&g.entities[0], 900, 500)
	g.mines[0].arm = 0

	g.entities[0].spawn = -1
	wingman := foe(2, 520, 500, 1)
	wingman.faction = 1
	wingman.spawn = -1
	victim := foe(3, 500, 520, 1)
	victim.spawn = -1
	g.entities = append(g.entities, wingman, victim)
	g.stepMines()

	if len(g.entities) != 0 {
		t.Fatalf("%d hulls walked out of a blast they were standing in: %+v", len(g.entities), g.entities)
	}
	if got := g.match.Stats[1].Kills; got != 1 {
		t.Errorf("faction 1 is credited %d kills, want only the enemy hull", got)
	}
	if got := g.match.Stats[1].Losses; got != 2 {
		t.Errorf("faction 1 counts %d losses, want its wingman and the hull that lingered on its own mine", got)
	}
	if got := g.match.Stats[2].Losses; got != 1 {
		t.Errorf("faction 2 counts %d losses, want 1", got)
	}
}

// The devourer is dropped once and SPENT: the hull goes back to its own bolt,
// and no second hole can be opened while the first is live.
func TestDevourerIsDroppedAndSpent(t *testing.T) {
	g := armedGame(t, "devourer")
	g.enemyFire(&g.entities[0], 900, 500)

	if g.devourer == nil {
		t.Fatal("firing while carrying the devourer must drop it")
	}
	if g.devourer.x != 500 || g.devourer.y != 500 {
		t.Errorf("the hole opened at (%v,%v), want the ship's own position", g.devourer.x, g.devourer.y)
	}
	if g.devourer.by != 1 || g.devourer.sid != 1 {
		t.Errorf("the hole lost who dropped it: %+v", g.devourer)
	}
	if got := g.entities[0].weaponKey; got != "" {
		t.Errorf("the hull still carries %q after spending the devourer", got)
	}
	if len(g.enemyShots) != 0 {
		t.Errorf("the deploy also fired %d bolts", len(g.enemyShots))
	}

	g.enemyFire(&g.entities[0], 900, 500)
	if len(g.enemyShots) != 1 {
		t.Errorf("a spent hull should be back on its bolt, got %d shots", len(g.enemyShots))
	}
}

// The TPK. The hole answers to nobody: every hull that reaches the core is
// swallowed, the fleet that dropped it included — which is exactly why picking
// the canister up is a decision and not a prize. Credit follows the same rule
// as the blast: the enemy counts, your own does not.
func TestDevourerSwallowsEveryFleet(t *testing.T) {
	g := armedGame(t, "devourer")
	g.enemyFire(&g.entities[0], 900, 500)
	g.devourer.arm = 0

	own := foe(2, 505, 500, 3)
	own.faction = 1
	own.spawn = -1
	enemy := foe(3, 500, 505, 3)
	enemy.spawn = -1
	g.entities[0].spawn = -1
	g.entities = append(g.entities, own, enemy)

	g.stepDevourer()

	if len(g.entities) != 0 {
		t.Fatalf("%d hulls survived the core they were sitting in: %+v", len(g.entities), g.entities)
	}
	if got := g.match.Stats[1].Kills; got != 1 {
		t.Errorf("faction 1 is credited %d kills, want only the enemy hull", got)
	}
	if got := g.match.Stats[1].Losses; got != 2 {
		t.Errorf("faction 1 counts %d losses, want the two hulls of its own it fed the hole", got)
	}
	if got := g.match.Stats[2].Losses; got != 1 {
		t.Errorf("faction 2 counts %d losses, want 1", got)
	}
}

// A laser kills in the same call that fires it, which means a hull can leave
// g.entities while updateEnemies is walking it. That crashed the game the
// first time a beam landed a kill in a real battle — the loop must survive its
// own casualties, and the ships behind the wreck must still get their tick.
func TestAKillDuringTheTickDoesNotBreakTheLoop(t *testing.T) {
	g := armedGame(t, "laser")
	shooter := &g.entities[0]
	shooter.stationary = true // a turret skips the pathfinding a bare test world has no map for
	shooter.spawn = -1

	victim := foe(2, 700, 500, 1) // straight down the firing line, one hit from dead
	victim.stationary = true
	victim.fireCD = 600
	victim.spawn = -1
	bystander := foe(3, 300, 900, 5) // far away, and last in the slice
	bystander.stationary = true
	bystander.fireCD = 600
	bystander.spawn = -1
	g.entities = append(g.entities, victim, bystander)

	g.updateEnemies() // the panic was here, not in the weapon

	if len(g.entities) != 2 {
		t.Fatalf("%d hulls left, want the shooter and the bystander", len(g.entities))
	}
	for i := range g.entities {
		if g.entities[i].id == 2 {
			t.Fatal("the victim stood in a beam and lived")
		}
	}
	// The bystander slid down a slot when the wreck was removed. It must not
	// have lost its tick to somebody else's death.
	for i := range g.entities {
		if g.entities[i].id != 3 {
			continue
		}
		if g.entities[i].fireCD != 599 {
			t.Errorf("the bystander's cooldown is %d, want 599: it was skipped", g.entities[i].fireCD)
		}
		return
	}
	t.Fatal("the bystander vanished")
}
