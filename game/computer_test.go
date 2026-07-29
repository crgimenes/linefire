package game

import (
	"testing"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/level"
	"github.com/crgimenes/linefire/weapon"
)

func TestComputerPickupUnlocksAutoFire(t *testing.T) {
	g := New(asset.New(), level.New(), "", false)
	if g.hasComputer || g.autoFire {
		t.Fatal("a fresh run starts without the combat computer")
	}

	e := entity{kind: kindPowerUp, power: powerComputer}
	g.applyPickup(&e)

	if !g.hasComputer {
		t.Fatal("the computer pickup should grant the module")
	}
	if !g.autoFire {
		t.Fatal("the module should come online firing")
	}
	if !feedContains(g, "combat computer") {
		t.Fatalf("the pickup should hit the feed, got %+v", g.log)
	}
}

func TestComputerStaysEngagedAcrossMaps(t *testing.T) {
	// An automatic module is ON from pickup until the player turns it off (G):
	// warping to another map must not silently disengage it (like the shield,
	// what you picked up keeps working).
	g := New(asset.New(), level.New(), "", false)
	e := entity{kind: kindPowerUp, power: powerComputer}
	g.applyPickup(&e)

	g.enterMap(level.New(), "elsewhere", "", false)

	if !g.hasComputer || !g.autoFire {
		t.Fatalf("the engaged module must survive a warp (has=%v on=%v)", g.hasComputer, g.autoFire)
	}
}

func TestLaserHeatLatchesAndCools(t *testing.T) {
	g := &Game{}

	// Burn until the latch trips.
	for range laserHeatMax {
		g.laserOn = true
		g.stepLaserHeat()
	}
	if !g.laserHot {
		t.Fatal("holding the beam to the cap should overheat it")
	}
	// The overheat is DIEGETIC: the beam wilts as it loads up and dies with a puff of
	// vapour, so nothing is written to the feed. The player watched it fail.
	if feedContains(g, "overheated") {
		t.Fatal("the overheat must not narrate itself in the feed")
	}
	if g.fxPool().Len() == 0 {
		t.Fatal("the beam cutting out should throw a puff of vapour")
	}
	if g.laserHeatFrac() < 1 {
		t.Fatalf("at the cap the beam is at full heat, got %v", g.laserHeatFrac())
	}

	// While hot, the slot refuses to fire the laser.
	g.arsenal = []int{weapon.CatLaser}
	g.slotArsIdx = [numSlots]int{0, -1}
	g.syncSlots()
	g.laserOn = false
	g.fireWeaponSlot(0)
	if g.laserOn {
		t.Fatal("an overheated laser must not fire")
	}

	// Half-cooled is still latched (hysteresis)...
	for range laserHeatMax / laserCoolRate / 2 {
		g.stepLaserHeat()
	}
	if !g.laserHot {
		t.Fatal("the latch should hold until stone cold")
	}
	// ...and stone cold re-arms.
	for range laserHeatMax {
		g.stepLaserHeat()
	}
	if g.laserHot || g.laserHeat != 0 {
		t.Fatalf("fully cooled should re-arm (hot=%v heat=%d)", g.laserHot, g.laserHeat)
	}
	g.fireWeaponSlot(0)
	if !g.laserOn {
		t.Fatal("a cooled laser should fire again")
	}
}
