package game

import (
	"math"
	"testing"
)

// TestArsenalStartsWithFrontGun: the ship launches with the front gun live in slot 1 and
// slot 2 empty until a second weapon is collected.
func TestArsenalStartsWithFrontGun(t *testing.T) {
	g := &Game{}
	g.initArsenal()
	if g.slotWeaponName(0) != "front" || g.slotWeaponName(1) != "-" {
		t.Fatalf("start = front in slot 1, slot 2 empty; got %q / %q", g.slotWeaponName(0), g.slotWeaponName(1))
	}
}

// TestCollectArmsTheEmptySlotNotThePrimary: a new weapon joins the arsenal and arms the
// first EMPTY slot (the secondary) instead of hijacking the forward primary; collecting
// it again is a no-op.
func TestCollectArmsTheEmptySlotNotThePrimary(t *testing.T) {
	g := &Game{}
	g.initArsenal()

	if !g.collectWeapon(catLaser) {
		t.Fatal("a new weapon should be collected")
	}
	if g.slotWeaponName(0) != "front" {
		t.Fatalf("a pickup must not hijack the primary, got %q", g.slotWeaponName(0))
	}
	if g.slotWeaponName(1) != "laser" {
		t.Fatalf("a new pickup should arm the empty secondary slot, got %q", g.slotWeaponName(1))
	}
	if g.collectWeapon(catLaser) {
		t.Fatal("collecting a duplicate must report false")
	}
	if len(g.arsenal) != 2 {
		t.Fatalf("arsenal should hold front + laser, got %v", g.arsenal)
	}
}

// TestCyclingSwapsNoseAndTurret is the crg playtest fix: with the laser mounted on the
// turret (slot 2) and only the gun otherwise free, pressing 1 must NOT freeze — it swaps
// the two, putting the laser on the nose and the gun on the turret, and pressing 1 again
// swaps back. A plain "skip the other slot's weapon" rule would leave the nose stuck.
func TestCyclingSwapsNoseAndTurret(t *testing.T) {
	g := &Game{}
	g.initArsenal()
	g.collectWeapon(catLaser) // gun on the nose, laser on the turret
	if g.slotWeaponName(0) != "front" || g.slotWeaponName(1) != "laser" {
		t.Fatalf("setup: want front/laser, got %q/%q", g.slotWeaponName(0), g.slotWeaponName(1))
	}
	g.cycleSlot(0)
	if g.slotWeaponName(0) != "laser" || g.slotWeaponName(1) != "front" {
		t.Fatalf("pressing 1 should swap nose<->turret, got %q/%q", g.slotWeaponName(0), g.slotWeaponName(1))
	}
	g.cycleSlot(0)
	if g.slotWeaponName(0) != "front" || g.slotWeaponName(1) != "laser" {
		t.Fatalf("pressing 1 again should swap back, got %q/%q", g.slotWeaponName(0), g.slotWeaponName(1))
	}
}

// TestCyclingNeverDupsAndReachesEvery: with three weapons, cycling the nose always changes
// it, never leaves the two mounts holding the same weapon, and — thanks to the swap — the
// nose can eventually hold every weapon, even one that had to slide off the turret.
func TestCyclingNeverDupsAndReachesEvery(t *testing.T) {
	g := &Game{}
	g.initArsenal()
	g.collectWeapon(catMissile) // turret
	g.collectWeapon(catLaser)   // arsenal: front, missile, laser
	seen := map[string]bool{}
	for range 12 {
		before := g.slotWeaponName(0)
		g.cycleSlot(0)
		if g.slotWeaponName(0) == before {
			t.Fatalf("cycling must always change the nose, stuck on %q", before)
		}
		if g.slotWeaponName(0) == g.slotWeaponName(1) {
			t.Fatalf("the two mounts must never hold the same weapon: %q", g.slotWeaponName(0))
		}
		seen[g.slotWeaponName(0)] = true
	}
	for _, w := range []string{"front", "missile", "laser"} {
		if !seen[w] {
			t.Fatalf("the nose should be able to hold every weapon; never saw %q", w)
		}
	}
}

// TestSlotDecidesAimNotWeapon is the fix for "the front laser aims at the mouse": the
// SLOT sets the direction, not the weapon. Slot 1 always fires forward, slot 2 toward the
// cursor — so a laser in slot 1 is a forward beam, whatever its own aim field says.
func TestSlotDecidesAimNotWeapon(t *testing.T) {
	if slotAim(0) != aimForward {
		t.Fatal("slot 1 (primary) must fire forward")
	}
	if slotAim(1) != aimMouse {
		t.Fatal("slot 2 (secondary) must fire toward the cursor")
	}
	// A MISSILE (its own aim is aimMouse) fired from slot 1 must ignore the cursor and shoot
	// FORWARD, because the slot overrides the weapon's aim. With the ship pointing up, a
	// forward shot travels straight up (vx≈0, vy<0); a cursor-aimed shot would slant.
	g := &Game{arsenal: []int{catMissile}, slotArsIdx: [numSlots]int{0, -1}}
	g.syncSlots()
	g.x, g.y, g.angle = 100, 100, -90 // pointing up
	g.fireWeaponSlot(0)
	if len(g.projectiles) != 1 {
		t.Fatalf("the primary should fire one shot, got %d", len(g.projectiles))
	}
	p := g.projectiles[0]
	if p.vy >= 0 || math.Abs(p.vx) > 1e-6 {
		t.Fatalf("a slot-1 turret must shoot straight forward (up), got velocity (%.3f,%.3f)", p.vx, p.vy)
	}
}

// TestComputerAssistsOnlySecondary is the fix for "the primary laser auto-fires with the
// secondary": the computer drives slot 2 only. The forward primary stays manual.
func TestComputerAssistsOnlySecondary(t *testing.T) {
	g := &Game{arsenal: []int{catLaser, catMissile}, slotArsIdx: [numSlots]int{0, 1}}
	g.syncSlots()
	g.autoFire, g.autoAimOK = true, true
	g.autoAimX, g.autoAimY = 0, -1

	g.runAutoFire()

	// The secondary (missile in slot 2) auto-fired; the primary (laser in slot 1) did not.
	if len(g.projectiles) == 0 {
		t.Fatal("the computer should auto-fire the secondary slot")
	}
	if g.laserOn {
		t.Fatal("the computer must NOT auto-fire the primary slot")
	}
}

// TestComputerNeverAutoDeploysMines: a mine in the secondary slot stays manual (no
// auto-mining), matching the old computer behavior.
func TestComputerNeverAutoDeploysMines(t *testing.T) {
	g := &Game{arsenal: []int{catFront, catMine}, slotArsIdx: [numSlots]int{0, 1}}
	g.syncSlots()
	g.autoFire, g.autoAimOK = true, true
	g.autoAimX, g.autoAimY = 0, -1

	g.runAutoFire()
	if len(g.mines) != 0 {
		t.Fatal("the computer must not auto-deploy mines")
	}
}
