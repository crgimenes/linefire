package weapon

import "testing"

// The front gun is the weapon that always works: a ship with an empty pool is
// out of heavy options, never out of the fight.
func TestTheFrontGunIsAlwaysAffordable(t *testing.T) {
	empty := Pool{Level: 0, Max: PoolMax}
	gun := Catalog[CatFront]
	if !empty.CanAfford(Cost(gun, 0)) {
		t.Fatal("the plain front gun is unaffordable on an empty pool")
	}
	if !empty.Spend(Cost(gun, 0)) {
		t.Fatal("firing the plain front gun on an empty pool was refused")
	}
	if empty.Level != 0 {
		t.Errorf("the front gun took %d energy, want none", -empty.Level)
	}

	// Upgraded, the same gun draws power: that is what the ship has to keep fed,
	// and what it gives up a level of when it cannot.
	if empty.CanAfford(Cost(gun, 1)) {
		t.Error("an upgraded front gun is free on an empty pool")
	}
}

// A refusal has to leave the pool alone, or a caller that drops to a cheaper
// weapon and asks again would have been charged for the shot it never fired.
func TestARefusedShotCostsNothing(t *testing.T) {
	p := Pool{Level: 5, Max: PoolMax}
	if p.Spend(Cost(Catalog[CatMissile], 0)) {
		t.Fatal("paid for a missile with 5 energy")
	}
	if p.Level != 5 {
		t.Errorf("a refused shot moved the pool to %d", p.Level)
	}
	if !p.Spend(Cost(Catalog[CatFront], 0)) {
		t.Fatal("could not fall back to the front gun")
	}
}

// Heavier weapons cost more: the pool is a choice about which weapon a fight is
// worth, and that only holds if the prices are ordered like the punch.
func TestHeavierWeaponsCostMore(t *testing.T) {
	gun, mine := Catalog[CatFront], Catalog[CatMine]
	missile, devourer := Catalog[CatMissile], Catalog[CatDevourer]
	switch {
	case gun.Energy != 0:
		t.Errorf("the front gun costs %d, want free", gun.Energy)
	case mine.Energy >= missile.Energy:
		t.Errorf("the mine (%d) is not cheaper than the missile (%d)", mine.Energy, missile.Energy)
	case devourer.Energy <= missile.Energy:
		t.Errorf("the devourer (%d) is not dearer than the missile (%d)", devourer.Energy, missile.Energy)
	}
}

// A full pool has to be a long fight's worth of heavy fire, not a timer.
func TestAFullPoolLasts(t *testing.T) {
	p := NewPool(0)
	fired := 0
	for p.Spend(Cost(Catalog[CatMissile], 0)) {
		fired++
	}
	if fired < 40 {
		t.Errorf("a full pool covered only %d missiles", fired)
	}
	beam := NewPool(0)
	ticks := 0
	for beam.Spend(Cost(Catalog[CatLaser], 0)) {
		ticks++
	}
	if seconds := float64(ticks) * 4 / 60; seconds < 30 {
		t.Errorf("a full pool covered only %.0f seconds of held beam", seconds)
	}
}

func TestAddStopsAtCapacity(t *testing.T) {
	p := Pool{Level: 10, Max: 100}
	p.Add(50)
	if p.Level != 60 {
		t.Errorf("adding 50 to 10 gave %d", p.Level)
	}
	p.Add(1000)
	if p.Level != 100 {
		t.Errorf("a pickup overfilled the pool to %d, capacity is 100", p.Level)
	}
	p.Add(-5)
	if p.Level != 100 {
		t.Errorf("a negative top-up drained the pool to %d", p.Level)
	}
	if p.Fraction() != 1 {
		t.Errorf("a full pool reads %.2f", p.Fraction())
	}
}

// Stepping down has to reach a shot the ship can always fire, or an empty pool
// would mean a disarmed ship.
func TestSteppingDownReachesAFreeShot(t *testing.T) {
	empty := Pool{Level: 0, Max: PoolMax}
	gun := Catalog[CatFront]
	level := 4
	for level > 0 && !empty.CanAfford(Cost(gun, level)) {
		level--
	}
	if level != 0 {
		t.Fatalf("stopped stepping down at level %d", level)
	}
	if !empty.Spend(Cost(gun, level)) {
		t.Fatal("the plain gun was still unaffordable after stepping all the way down")
	}
}

// The trickle covers a lull without making the pool irrelevant: it must give
// back a missile in a handful of seconds, not instantly.
func TestRegenIsSlow(t *testing.T) {
	p := Pool{Level: 0, Max: PoolMax}
	missile := Cost(Catalog[CatMissile], 0)
	ticks := 0
	for p.Level < missile {
		p.Tick()
		ticks++
	}
	if seconds := float64(ticks) / 60; seconds < 1 || seconds > 10 {
		t.Errorf("a missile's worth of energy took %.1f seconds to come back", seconds)
	}

	full := Pool{Level: PoolMax, Max: PoolMax}
	full.Tick()
	if full.Level != PoolMax {
		t.Errorf("a full pool ticked past its capacity to %d", full.Level)
	}
}
