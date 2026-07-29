package game

import (
	"testing"

	"github.com/crgimenes/linefire/weapon"
)

// TestDevourerNeedsChargeAndIsUnique: deploy is refused with no charge, spends one when it fires,
// and refuses a second while one is already live (one black hole at a time).
func TestDevourerNeedsChargeAndIsUnique(t *testing.T) {
	g := armSlot0(weapon.CatDevourer)

	g.fireWeaponSlot(0)
	if g.devourer != nil {
		t.Fatal("with no charge, deploy must be refused")
	}

	g.devourerAmmo = 2
	g.fireWeaponSlot(0)
	if g.devourer == nil {
		t.Fatal("with a charge, deploy should drop a black hole")
	}
	if g.devourerAmmo != 1 {
		t.Fatalf("deploy should spend one charge, got %d", g.devourerAmmo)
	}

	g.slots[0].cd = 0 // ignore cooldown for the test
	g.fireWeaponSlot(0)
	if g.devourerAmmo != 1 {
		t.Fatal("a second deploy while one is live must not spend a charge")
	}
}

// TestDevourerDeployNeedsFreshPress guards the debounce: the DEVOURER deploys only on a fresh
// button press, so a HELD button neither spams "no charge" nor auto-burns a charge; ordinary
// weapons still fire while held.
func TestDevourerDeployNeedsFreshPress(t *testing.T) {
	g := armSlot0(weapon.CatDevourer)
	g.devourerAmmo = 3

	g.fireSlotInput(0, false) // button held, not a fresh press
	if g.devourer != nil {
		t.Fatal("a held button must not deploy the devourer")
	}
	g.fireSlotInput(0, true) // fresh press
	if g.devourer == nil {
		t.Fatal("a fresh press should deploy the devourer")
	}

	// A continuous weapon still fires while held (justPressed=false).
	gc := armSlot0(weapon.CatFront)
	gc.fireSlotInput(0, false)
	if len(gc.projectiles) != 1 {
		t.Fatalf("a held button should keep firing an ordinary weapon, got %d shots", len(gc.projectiles))
	}
}

// TestIdkfaStocksDevourerCharges: idkfa grants the DEVOURER, so it must also stock charges or the
// cheated weapon reads "no charge".
func TestIdkfaStocksDevourerCharges(t *testing.T) {
	g := &Game{}
	g.grantFullArsenal()
	if !g.hasWeapon(weapon.CatDevourer) {
		t.Fatal("idkfa should grant the devourer")
	}
	if g.devourerAmmo <= 0 {
		t.Fatalf("idkfa must stock devourer charges, got %d", g.devourerAmmo)
	}
}

// TestDevourerPullsAndSwallows: an active black hole drags a distant enemy inward and destroys
// (scoring) one that reaches the core.
func TestDevourerPullsAndSwallows(t *testing.T) {
	g := &Game{}
	g.devourer = &devourer{active: 10}
	g.entities = []entity{
		{kind: kindEnemy, x: 200, y: 0, hp: 3, spawn: -1}, // far: pulled inward
		{kind: kindEnemy, x: 10, y: 0, hp: 3, spawn: -1},  // at the core: swallowed
	}
	before := g.entities[0].x

	g.devourerPull(g.devourer)

	if len(g.entities) != 1 {
		t.Fatalf("the core enemy should be swallowed, got %d entities left", len(g.entities))
	}
	if g.entities[0].x >= before {
		t.Fatalf("the surviving enemy should be dragged inward: %.0f -> %.0f", before, g.entities[0].x)
	}
	if g.score != 1 {
		t.Fatalf("swallowing an enemy should score, got %d", g.score)
	}
}

// TestDevourerPullStopsAtWall: an object dragged toward the core does NOT clip through a wall in
// the way — it piles up against the rock instead.
func TestDevourerPullStopsAtWall(t *testing.T) {
	g := &Game{segs: []segment{{ax: 5, ay: -40, bx: 5, by: 40}}} // wall at x=5
	g.devourer = &devourer{x: -100, y: 0, active: 10}            // core on the far side of the wall
	g.entities = []entity{{kind: kindPowerUp, x: 20, y: 0, radius: 8, spawn: -1}}

	g.devourerPull(g.devourer)

	if g.entities[0].x <= 5 {
		t.Fatalf("a pulled item must not cross the wall at x=5, got x=%.2f", g.entities[0].x)
	}
}

// TestDevourerSwallowsAllies: escorts are dragged in like everything else, and destroyed at the
// core (the black hole plays no favourites).
func TestDevourerSwallowsAllies(t *testing.T) {
	g := &Game{}
	g.devourer = &devourer{active: 10}
	g.allies = []ally{{x: 10, y: 0}, {x: 500, y: 0}} // one at the core, one within reach

	g.devourerPullAllies(g.devourer)

	if len(g.allies) != 1 {
		t.Fatalf("the core escort should be swallowed, got %d left", len(g.allies))
	}
	if g.allies[0].x >= 500 {
		t.Fatalf("the surviving escort should be dragged inward, got x=%.0f", g.allies[0].x)
	}
}

// TestDevourerCrushesShipAtCore: a ship pulled all the way into the core is swallowed (lethal),
// not merely shaken until the collapse.
func TestDevourerCrushesShipAtCore(t *testing.T) {
	g := &Game{}
	g.health = maxHealth
	g.devourer = &devourer{active: 10}
	g.x, g.y = 0, 0 // sitting on the core

	g.devourerCrushShip(g.devourer)

	if g.endKind != endDeath {
		t.Fatalf("a ship dragged into the core should take lethal damage (queue death), endKind=%d health=%d", g.endKind, g.health)
	}
}

// TestDevourerDangerZoneHurtsShip: being inside the danger zone (short of the core) shreds the
// hull — you do not have to hit the exact centre to take damage.
func TestDevourerDangerZoneHurtsShip(t *testing.T) {
	g := &Game{}
	g.health = maxHealth
	g.devourer = &devourer{active: devourerHurtEvery} // a multiple of the tick cadence, so it fires
	g.x, g.y = devourerDanger*0.5, 0                  // inside the zone, outside the core

	g.devourerCrushShip(g.devourer)

	if g.health >= maxHealth {
		t.Fatalf("the danger zone should shred the hull, health=%d", g.health)
	}
	if g.over || g.endKind != endNone {
		t.Fatal("a mid-zone tick should hurt, not instantly kill")
	}
}

// TestDevourerPullsPlayer: the active black hole adds velocity toward itself, so the ship is
// dragged in (and must thrust away to hold).
func TestDevourerPullsPlayer(t *testing.T) {
	g := &Game{}
	g.x, g.y = 100, 0
	g.devourer = &devourer{active: 10}

	g.applyDevourerPull()

	if g.vx >= 0 {
		t.Fatalf("the ship should be pulled toward the hole (negative x), got vx=%.3f", g.vx)
	}
	// While still arming there is no pull yet.
	g2 := &Game{}
	g2.x = 100
	g2.devourer = &devourer{arm: 5, active: 10}
	g2.applyDevourerPull()
	if g2.vx != 0 {
		t.Fatalf("an arming hole must not pull yet, got vx=%.3f", g2.vx)
	}
}

// TestDevourerCollapses: after arming and the active window elapse, the next step collapses the
// hole and clears it.
func TestDevourerCollapses(t *testing.T) {
	g := &Game{}
	g.x, g.y = 9000, 9000 // far from the implosion so the collapse cannot end the test early
	g.devourer = &devourer{arm: 1, active: 1}

	g.stepDevourer() // arm 1 -> 0
	g.stepDevourer() // active 1 -> 0
	if g.devourer == nil {
		t.Fatal("the hole should still exist through its arming and pull phases")
	}
	g.stepDevourer() // collapse
	if g.devourer != nil {
		t.Fatal("the hole should collapse and clear after its window")
	}
	if !feedContains(g, "COLLAPSE") {
		t.Fatalf("the collapse should announce itself, got %+v", g.log)
	}
}
