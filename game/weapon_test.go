package game

import (
	"testing"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/weapon"
)

// armSlot0 builds a game with one weapon collected and live in slot 0 (nil player:
// muzzleWorld falls back to the ship position).
func armSlot0(cat int) *Game {
	g := &Game{arsenal: []int{cat}, slotArsIdx: [numSlots]int{0, -1}}
	g.syncSlots()
	return g
}

func TestCatalogHasEveryWeapon(t *testing.T) {
	// The turret is not a weapon — it is the slot-2 mount — so the catalog is gun,
	// missile, mine, laser, devourer.
	if len(weapon.Catalog) != 5 || weapon.Catalog[weapon.CatLaser].Kind != weapon.KindLaser {
		t.Fatalf("the catalog should hold all 5 weapon types incl. the devourer, got %d", len(weapon.Catalog))
	}
	if weapon.Catalog[weapon.CatFront].Aim != weapon.AimForward || weapon.Catalog[weapon.CatMine].Kind != weapon.KindMine {
		t.Fatalf("catalog entries out of order: %+v", weapon.Catalog)
	}
	if weapon.Catalog[weapon.CatDevourer].Kind != weapon.KindDevourer {
		t.Fatalf("the devourer should be the black-hole weapon, got %+v", weapon.Catalog[weapon.CatDevourer])
	}
}

func TestFireSlotRespectsCooldown(t *testing.T) {
	g := armSlot0(weapon.CatFront)

	g.fireWeaponSlot(0)
	if len(g.projectiles) != 1 {
		t.Fatalf("first front-gun use should spawn a shot, got %d", len(g.projectiles))
	}
	if g.slots[0].cd <= 0 {
		t.Fatal("firing should start the slot cooldown")
	}

	g.fireWeaponSlot(0) // still on cooldown
	if len(g.projectiles) != 1 {
		t.Fatalf("a use on cooldown must not fire, got %d shots", len(g.projectiles))
	}

	for range weapon.Catalog[weapon.CatFront].Cooldown {
		g.tickWeapons()
	}
	g.fireWeaponSlot(0)
	if len(g.projectiles) != 2 {
		t.Fatalf("after the cooldown it should fire again, got %d", len(g.projectiles))
	}
}

func TestEmptySlotDoesNotFire(t *testing.T) {
	g := &Game{arsenal: []int{weapon.CatFront}, slotArsIdx: [numSlots]int{-1, -1}}
	g.syncSlots()
	g.fireWeaponSlot(0)
	if len(g.projectiles) != 0 {
		t.Fatalf("an empty slot must not fire, got %d shots", len(g.projectiles))
	}
	g.cycleSlot(0) // arm the first weapon
	g.fireWeaponSlot(0)
	if len(g.projectiles) != 1 {
		t.Fatalf("an armed slot should fire, got %d", len(g.projectiles))
	}
}

func TestMineSlotCarriesWeaponPayload(t *testing.T) {
	g := armSlot0(weapon.CatMine)
	g.fireWeaponSlot(0)
	if len(g.mines) != 1 {
		t.Fatalf("the mine slot should deploy a mine, got %d", len(g.mines))
	}
	m := g.mines[0]
	if m.dmg != weapon.Catalog[weapon.CatMine].Damage || m.radius != weapon.Catalog[weapon.CatMine].AOE || m.trigger != weapon.Catalog[weapon.CatMine].TriggerRadius {
		t.Fatalf("the deployed mine should carry the weapon's payload, got %+v", m)
	}
}

func TestUnusableWeaponDoesNotLockCooldown(t *testing.T) {
	// The mine cooldown is 0, so the only limit is its cap. Deploying past the cap must
	// fail (useWeapon returns false) without locking the slot, so exactly the cap deploy.
	g := armSlot0(weapon.CatMine)
	for range weapon.Catalog[weapon.CatMine].MaxLive + 3 {
		g.slots[0].cd = 0 // clear the spacing cooldown so only the cap limits it
		g.fireWeaponSlot(0)
	}
	if len(g.mines) != weapon.Catalog[weapon.CatMine].MaxLive {
		t.Fatalf("mine cap should hold at %d, got %d", weapon.Catalog[weapon.CatMine].MaxLive, len(g.mines))
	}
}

// loadoutTestPlayer is a ship asset with four weapon hardpoints, for tests that need a
// real player mesh/hardpoint (the mount system is gone, but the hardpoints still size the
// muzzle and the collision radius).
func loadoutTestPlayer() *asset.Asset {
	a := asset.New()
	a.Hardpoints = []asset.Hardpoint{
		{Name: "a", Kind: asset.KindWeapon, X: 32, Y: 8},
		{Name: "b", Kind: asset.KindWeapon, X: 16, Y: 46},
		{Name: "c", Kind: asset.KindWeapon, X: 48, Y: 46},
		{Name: "d", Kind: asset.KindWeapon, X: 32, Y: 30},
	}
	return a
}
