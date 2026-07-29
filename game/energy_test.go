package game

import (
	"testing"

	"github.com/crgimenes/linefire/weapon"
)

// A zero Game starts armed: the pool fills on first use, so a test — or a fresh
// run — is not silently out of ammunition before it fires a shot.
func TestEnergyStartsFull(t *testing.T) {
	g := newCombatGame()
	if p := g.energyPool(); p.Level != p.Max || p.Max == 0 {
		t.Fatalf("a fresh ship has %d of %d energy", p.Level, p.Max)
	}
}

// The plain front gun is what the ship falls back to, so it has to fire on an
// empty pool. Everything heavier is refused rather than fired for free.
func TestAnEmptyPoolStillFiresThePlainGun(t *testing.T) {
	g := newCombatGame()
	g.energyPool().Level = 0

	gun := weapon.Catalog[weapon.CatFront]
	if !g.payForShot(&gun) {
		t.Fatal("the plain front gun was refused on an empty pool")
	}

	missile := weapon.Catalog[weapon.CatMissile]
	if g.payForShot(&missile) {
		t.Fatal("a missile fired on an empty pool")
	}
}

// Upgrades cost energy to fire, so a ship that has run dry gives them up one at
// a time and keeps shooting, instead of falling silent with a full arsenal.
func TestRunningDryStepsUpgradesDown(t *testing.T) {
	g := newCombatGame()
	g.fireLevel, g.damageLevel = 2, 1
	before := g.powerLevel()
	g.energyPool().Level = 0

	gun := weapon.Catalog[weapon.CatFront]
	if !g.payForShot(&gun) {
		t.Fatal("the gun stopped firing entirely instead of stepping down")
	}
	if g.powerLevel() >= before {
		t.Errorf("upgrade level is still %d after firing dry, was %d", g.powerLevel(), before)
	}
	if g.powerLevel() != 0 {
		t.Errorf("stepped down to %d, want all the way to the free shot", g.powerLevel())
	}
	if g.fireLevel < 0 || g.damageLevel < 0 {
		t.Errorf("an upgrade went negative: fire %d, damage %d", g.fireLevel, g.damageLevel)
	}
}

// A shot the ship CAN pay for must not cost it an upgrade.
func TestPayingKeepsTheUpgrades(t *testing.T) {
	g := newCombatGame()
	g.fireLevel = 3
	gun := weapon.Catalog[weapon.CatFront]
	if !g.payForShot(&gun) {
		t.Fatal("a full pool refused a shot")
	}
	if g.fireLevel != 3 {
		t.Errorf("fire level dropped to %d on a shot the ship could afford", g.fireLevel)
	}
}

// Collecting anything puts energy back: that is what makes a pickup worth the
// detour once the heavy weapons have been drawing on the pool.
func TestPickupsRestoreEnergy(t *testing.T) {
	g := newCombatGame()
	pool := g.energyPool()
	pool.Level = 10
	g.applyPickup(&entity{power: powerScore})
	if pool.Level <= 10 {
		t.Errorf("a pickup left the pool at %d", pool.Level)
	}
}
