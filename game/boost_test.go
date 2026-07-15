package game

import "testing"

// TestBoostRaisesSpeedCap: the afterburner lifts the speed cap; without it the ship is held to
// maxSpeed.
func TestBoostRaisesSpeedCap(t *testing.T) {
	g := &Game{vx: maxSpeed * 2}
	g.boosting = false
	g.clampSpeed()
	if g.vx > maxSpeed+1e-9 {
		t.Fatalf("without boost the cap is maxSpeed, got %.2f", g.vx)
	}

	g.vx = maxSpeed * 2
	g.boosting = true
	g.clampSpeed()
	if g.vx <= maxSpeed || g.vx > maxSpeed*boostSpeedMul+1e-9 {
		t.Fatalf("boosting should cap at maxSpeed*%.2f, got %.2f", boostSpeedMul, g.vx)
	}
}

// TestBoostFuelRegensWhenIdle: with the key up (IsKeyPressed is false in tests) the fuel recharges
// and the ship is not boosting.
func TestBoostFuelRegensWhenIdle(t *testing.T) {
	g := &Game{boostFuel: 0}
	if g.updateBoost(1, 0) {
		t.Fatal("with no key held the booster must not engage")
	}
	if g.boostFuel <= 0 {
		t.Fatal("the booster fuel should recharge while idle")
	}
	if g.boosting {
		t.Fatal("g.boosting must be false when not engaged")
	}
}

// TestBoostRearmsOnlyAfterThreshold: once the fuel is spent the booster stays LOCKED until it
// recharges past the re-arm threshold, so it cannot be ridden on a trickle.
func TestBoostRearmsOnlyAfterThreshold(t *testing.T) {
	g := &Game{boostFuel: 0} // just emptied
	g.updateBoost(1, 0)      // key up in tests: just recharges a little, stays locked
	if g.boostArmed {
		t.Fatal("the booster must stay locked below the re-arm threshold")
	}

	g.boostFuel = boostRearmFrac*boostMax + 1 // recharged past the threshold
	g.updateBoost(1, 0)
	if !g.boostArmed {
		t.Fatal("past the re-arm threshold the booster should be available again")
	}
}

// TestFastMoveDoesNotTunnel: a move longer than maxSpeed (only the afterburner reaches it) is
// sub-stepped, so the ship stops at a thin wall instead of jumping across it.
func TestFastMoveDoesNotTunnel(t *testing.T) {
	g := &Game{
		segs:   []segment{{ax: 5, ay: -20, bx: 5, by: 20}}, // vertical wall at x=5
		radius: 3, x: 0, y: 0, vx: maxSpeed * 2,
		health: maxHealth, lives: startLives,
	}
	g.tryMove(g.vx, 0)
	if g.x >= 5 {
		t.Fatalf("a fast ship must not tunnel through the wall at x=5, got x=%.2f", g.x)
	}
}
