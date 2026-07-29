package game

import (
	"math"
	"testing"

	"github.com/crgimenes/linefire/weapon"
)

func TestShotsForLevelAndSpread(t *testing.T) {
	if shotsForLevel(0) != 1 {
		t.Fatalf("level 0 should be 1 shot, got %d", shotsForLevel(0))
	}
	if shotsForLevel(2) != 3 {
		t.Fatalf("level 2 should be 3 shots, got %d", shotsForLevel(2))
	}
	if shotsForLevel(99) != maxFireShots {
		t.Fatalf("shots should cap at %d, got %d", maxFireShots, shotsForLevel(99))
	}
	if spreadOffset(0, 1) != 0 {
		t.Fatal("a single shot has no fan offset")
	}
	if spreadOffset(1, 3) != 0 {
		t.Fatal("the middle of three shots flies straight")
	}
	if spreadOffset(0, 3) != -spreadOffset(2, 3) {
		t.Fatal("the outer fan shots should be symmetric")
	}
}

func TestFirePowerStacksToCap(t *testing.T) {
	g := &Game{}
	for i := range maxFireLevel {
		if !g.addFirePower() {
			t.Fatalf("fire power should rise at step %d", i)
		}
	}
	if g.fireLevel != maxFireLevel {
		t.Fatalf("fire power should reach the cap %d, got %d", maxFireLevel, g.fireLevel)
	}
	if g.addFirePower() {
		t.Fatal("a pickup past the cap should not raise fire power")
	}
}

func TestCooldownForLevelShortensAndFloors(t *testing.T) {
	if cooldownForLevel(20, 0) != 20 {
		t.Fatal("level 0 must leave the base cooldown unchanged")
	}
	prev := 20
	for level := 1; level <= maxRateLevel; level++ {
		cd := cooldownForLevel(20, level)
		if cd > prev {
			t.Fatalf("cooldown should not rise with level: level %d gave %d after %d", level, cd, prev)
		}
		if cd < minCooldown {
			t.Fatalf("cooldown should never drop below the floor %d, got %d", minCooldown, cd)
		}
		prev = cd
	}
	if got := cooldownForLevel(20, 4); got >= 20 {
		t.Fatalf("a maxed fire rate should clearly shorten the cooldown, got %d", got)
	}
	// A base already under the floor must not be raised by the rate mod.
	if got := cooldownForLevel(2, 3); got > 2 {
		t.Fatalf("a sub-floor base must not be slowed down, got %d", got)
	}
}

func TestDamageForLevelGrowsFromBase(t *testing.T) {
	if damageForLevel(10, 0) != 10 {
		t.Fatal("level 0 must leave the base damage unchanged")
	}
	prev := 10
	for level := 1; level <= maxDamageLevel; level++ {
		d := damageForLevel(10, level)
		if d < prev {
			t.Fatalf("damage should not fall with level: level %d gave %d after %d", level, d, prev)
		}
		prev = d
	}
	if got := damageForLevel(10, maxDamageLevel); got <= 10 {
		t.Fatalf("a maxed damage mod should clearly raise damage, got %d", got)
	}
}

func TestRateAndDamageStackToCap(t *testing.T) {
	g := &Game{}
	for i := range maxRateLevel {
		if !g.addRatePower() {
			t.Fatalf("fire rate should rise at step %d", i)
		}
	}
	if g.addRatePower() {
		t.Fatal("a rate pickup past the cap should not raise the level")
	}
	for i := range maxDamageLevel {
		if !g.addDamagePower() {
			t.Fatalf("damage should rise at step %d", i)
		}
	}
	if g.addDamagePower() {
		t.Fatal("a damage pickup past the cap should not raise the level")
	}
}

func TestMultishotFansDirectWeaponButNotMissile(t *testing.T) {
	g := &Game{angle: -90}
	g.fireLevel = 2 // -> 3 shots

	g.fireProjectileWeapon(&weapon.Catalog[weapon.CatFront], weapon.AimForward)
	if len(g.projectiles) != 3 {
		t.Fatalf("a direct weapon at fire level 2 should fan 3 shots, got %d", len(g.projectiles))
	}
	if g.projectiles[0].vx == g.projectiles[2].vx && g.projectiles[0].vy == g.projectiles[2].vy {
		t.Fatal("the fan's outer shots should travel in different directions")
	}

	g.projectiles = nil
	g.fireProjectileWeapon(&weapon.Catalog[weapon.CatMissile], weapon.AimForward) // aoe > 0
	if len(g.projectiles) != 1 {
		t.Fatalf("an AoE weapon must still fire a single shot, got %d", len(g.projectiles))
	}
}

func TestSeekTurnAndSteering(t *testing.T) {
	if seekTurnForLevel(0) != 0 {
		t.Fatal("no homing at level 0")
	}
	if seekTurnForLevel(2) <= seekTurnForLevel(1) {
		t.Fatal("homing should steepen with level")
	}
	// A shot flying +x with an enemy straight up (+y) should turn toward +y: vy grows.
	vx, vy := steerToward(5, 0, 0, 10, 0.2)
	if vy <= 0 {
		t.Fatalf("steering should turn velocity toward the target (+y), got vy=%.3f", vy)
	}
	if s := math.Hypot(vx, vy); math.Abs(s-5) > 1e-9 {
		t.Fatalf("steering must preserve speed 5, got %.6f", s)
	}
	// The turn is capped: one step cannot flip a +x shot straight to -x.
	vx2, _ := steerToward(5, 0, -5, 0, 0.2)
	if vx2 < 0 {
		t.Fatal("a single homing step should not exceed the max turn")
	}
}
