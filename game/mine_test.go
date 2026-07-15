package game

import "testing"

func TestDeployMineRespectsCap(t *testing.T) {
	g := &Game{}
	for range maxMines + 3 {
		g.deployMineWeapon(&weaponMine)
	}
	if len(g.mines) != maxMines {
		t.Fatalf("mines should be capped at %d, got %d", maxMines, len(g.mines))
	}
}

func TestMineArmsThenDetonatesOnProximity(t *testing.T) {
	g := &Game{}
	g.deployMineWeapon(&weaponMine)
	if len(g.mines) != 1 || g.mines[0].arm <= 0 {
		t.Fatalf("a fresh mine should be arming, got %+v", g.mines)
	}

	// While arming, an adjacent enemy must not set it off.
	g.entities = []entity{{kind: kindEnemy, x: 5, y: 0, radius: 3, hp: 99}}
	g.stepMines()
	if len(g.mines) != 1 {
		t.Fatal("an arming mine should not detonate yet")
	}
	if g.entities[0].hp != 99 {
		t.Fatal("an arming mine should not have damaged the enemy")
	}

	// Run out the arming delay with the enemy out of range, so it just arms.
	g.entities[0].x = mineTriggerRadius * 4
	for range mineArmFrames {
		g.stepMines()
	}
	if len(g.mines) != 1 || g.mines[0].arm != 0 {
		t.Fatalf("mine should be armed and waiting, got %+v", g.mines)
	}

	// Enemy walks into range: it detonates, damaging the enemy and ringing a wave.
	g.entities[0].x = 10
	g.stepMines()
	if len(g.mines) != 0 {
		t.Fatalf("armed mine should detonate on proximity, %d left", len(g.mines))
	}
	if g.entities[0].hp >= 99 {
		t.Fatalf("detonation should damage the nearby enemy, hp=%d", g.entities[0].hp)
	}
	if len(g.shocks) != 1 {
		t.Fatalf("detonation should ring a shockwave, got %d", len(g.shocks))
	}
}

func TestArmedMineIgnoresDistantEnemy(t *testing.T) {
	g := &Game{}
	g.deployMineWeapon(&weaponMine)
	for range mineArmFrames {
		g.stepMines() // arm it with no enemies around
	}
	g.entities = []entity{{kind: kindEnemy, x: mineTriggerRadius * 3, y: 0, radius: 3, hp: 99}}
	g.stepMines()
	if len(g.mines) != 1 {
		t.Fatal("an armed mine should not detonate for an out-of-range enemy")
	}
}
