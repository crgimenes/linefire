package game

import (
	"math/rand/v2"
	"testing"
)

// In the campaign the horde has exactly one prey: the player. That contract is
// what keeps every faction change invisible to linefire.
func TestEnemyTargetInCampaignIsThePlayer(t *testing.T) {
	g := &Game{}
	g.x, g.y = 77, 33
	g.entities = []entity{
		{kind: kindEnemy, x: 0, y: 0, hp: 3},
		{kind: kindEnemy, x: 10, y: 0, hp: 3, faction: 2}, // even a "foe" nearby changes nothing
	}
	tx, ty, ok := g.enemyTarget(&g.entities[0])
	if !ok || tx != g.x || ty != g.y {
		t.Fatalf("campaign target = (%v,%v,%v), want the player at (77,33)", tx, ty, ok)
	}
}

// In skirmish a ship hunts the NEAREST ship of another faction — never its own
// kind, never a corpse — and patrols when no foe is left.
func TestEnemyTargetInSkirmishIsTheNearestFoe(t *testing.T) {
	g := &Game{}
	g.skirmishMode = true
	g.entities = []entity{
		{kind: kindEnemy, x: 0, y: 0, hp: 3, faction: 1},
		{kind: kindEnemy, x: 50, y: 0, hp: 3, faction: 1},   // closest of all, but a teammate
		{kind: kindEnemy, x: 100, y: 0, hp: 3, faction: 2},  // the answer
		{kind: kindEnemy, x: 80, y: 0, hp: 0, faction: 2},   // nearer foe, but dead
		{kind: kindEnemy, x: 500, y: 0, hp: 3, faction: 3},  // live foe, farther
		{kind: kindPowerUp, x: 20, y: 0, hp: 0, faction: 2}, // loot is nobody's foe
	}
	tx, ty, ok := g.enemyTarget(&g.entities[0])
	if !ok || tx != 100 || ty != 0 {
		t.Fatalf("skirmish target = (%v,%v,%v), want the live faction-2 ship at (100,0)", tx, ty, ok)
	}

	g.entities = g.entities[:2] // only teammates left
	if _, _, ok := g.enemyTarget(&g.entities[0]); ok {
		t.Fatal("with no foe alive the ship should patrol, not hunt")
	}
}

// A faction bolt lands on the first hull of ANOTHER faction and passes through
// its own kind — which in the campaign is every hull (all faction 0).
func TestFactionBoltHitsOnlyFoes(t *testing.T) {
	g := &Game{}
	g.entities = []entity{
		{kind: kindEnemy, x: 30, y: 0, radius: 8, hp: 3, faction: 1}, // shooter's teammate, in the way
		{kind: kindEnemy, x: 60, y: 0, radius: 8, hp: 3, faction: 2}, // the foe behind it
	}
	if hit := g.bulletHitsFoe(0, 0, 100, 0, 1); hit != 1 {
		t.Fatalf("faction-1 bolt hit index %d, want the faction-2 ship at 1", hit)
	}
	if hit := g.bulletHitsFoe(0, 0, 100, 0, 0); hit < 0 {
		t.Fatal("a faction-0 bolt should hit faction ships (they are foes to the horde)")
	}
	g.entities[1].faction = 0
	g.entities[0].faction = 0
	if hit := g.bulletHitsFoe(0, 0, 100, 0, 0); hit >= 0 {
		t.Fatalf("the campaign horde's own fire hit its own kind (index %d)", hit)
	}
}

// One faction bolt does one point of HULL damage: shotDmg is in player-health
// units and must never reach a hull raw (a 24-damage hit would one-shot a tank).
func TestFactionBoltDoesHullScaleDamage(t *testing.T) {
	g := &Game{}
	g.skirmishMode = true
	g.entities = []entity{
		{kind: kindEnemy, x: 40, y: 0, radius: 8, hp: 3, faction: 2},
	}
	g.enemyShots = []projectile{{
		x: 0, y: 0, vx: 50, vy: 0, life: 10, dmg: 24, faction: 1,
	}}
	g.stepEnemyShots()
	if got := g.entities[0].hp; got != 3-shipShotHullDamage {
		t.Fatalf("hull hp after one faction bolt = %d, want %d", got, 3-shipShotHullDamage)
	}
	if len(g.enemyShots) != 0 {
		t.Fatal("the bolt should be consumed by the hit")
	}
}

// Arrivals deal teams round-robin so the factions stay even, and the cursor
// survives however many arrivals a battle runs through.
func TestArrivalsDealFactionsRoundRobin(t *testing.T) {
	g := &Game{}
	g.skirmishMode = true
	g.factions = 3
	g.bounds = bounds{minX: 0, minY: 0, maxX: 1000, maxY: 1000}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 6 {
		g.spawnArrival("enemy", rng)
	}
	want := []int{1, 2, 3, 1, 2, 3}
	for i, a := range g.arrivals {
		if a.faction != want[i] {
			t.Fatalf("arrival %d got faction %d, want %d", i, a.faction, want[i])
		}
	}
}
