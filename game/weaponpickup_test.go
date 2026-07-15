package game

import (
	"testing"

	"linefire/asset"
	"linefire/level"
)

// TestFlyingOverWeaponCollectsIt: a weapon on the ground joins the arsenal and arms the
// empty secondary slot the moment the ship touches it — no drop, no swap, no screen.
func TestFlyingOverWeaponCollectsIt(t *testing.T) {
	g := New(loadoutTestPlayer(), level.New(), "", false)
	g.x, g.y = 0, 0
	g.entities = []entity{{kind: kindWeapon, power: "laser", x: 0, y: 0, radius: 16, spawn: 0}}

	g.resolveWeaponPickups()

	if !g.hasWeapon(catLaser) {
		t.Fatalf("flying over the laser should collect it, arsenal=%v", g.arsenal)
	}
	if g.slotWeaponName(1) != "laser" {
		t.Fatalf("a fresh pickup should arm the empty secondary slot, got %q", g.slotWeaponName(1))
	}
	for i := range g.entities {
		if g.entities[i].kind == kindWeapon {
			t.Fatal("collecting a weapon should consume the pickup")
		}
	}
	if !g.curMapState().consumed[0] {
		t.Fatal("the consumed pickup must stay gone on a revisit")
	}
}

// TestCollectingADuplicateLeavesItOnTheGround: you already own it, so there is nothing to
// gain — the pickup stays, and standing on it does not churn.
func TestCollectingADuplicateLeavesItOnTheGround(t *testing.T) {
	g := New(loadoutTestPlayer(), level.New(), "", false)
	g.x, g.y = 0, 0
	g.entities = []entity{{kind: kindWeapon, power: "front", x: 0, y: 0, radius: 16, spawn: 0}}

	g.resolveWeaponPickups() // front is already in the starting arsenal
	if len(g.entities) != 1 {
		t.Fatalf("a weapon already owned should stay on the ground, got %d entities", len(g.entities))
	}
}

func TestWeaponSpawnBecomesEntity(t *testing.T) {
	lvl := level.New()
	lvl.Spawns = []level.Spawn{{Name: "w", Kind: "weapon-laser", X: 5, Y: 5}}
	g := New(asset.New(), lvl, "", false)
	found := false
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind == kindWeapon && e.power == "laser" && e.align == alignGood {
			found = true
		}
	}
	if !found {
		t.Fatal("a weapon-laser spawn should build a good-aligned weapon entity")
	}
}
