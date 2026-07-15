package game

import (
	"math"
	"testing"
)

func TestNearestEnemyDirPicksClosestVisible(t *testing.T) {
	g := newCombatGame() // no walls -> clear line of sight
	g.x, g.y = 0, 0
	g.entities = []entity{
		{kind: kindEnemy, x: 200, y: 0, radius: 5, hp: 3}, // far
		{kind: kindEnemy, x: 40, y: 0, radius: 5, hp: 3},  // near (the target)
		{kind: kindPowerUp, x: 10, y: 0, radius: 5},       // not an enemy: ignored
	}
	dx, dy, ok := g.nearestEnemyDir()
	if !ok {
		t.Fatal("a visible enemy should give a target direction")
	}
	if math.Abs(dx-1) > 1e-9 || math.Abs(dy) > 1e-9 {
		t.Fatalf("should aim at the nearer enemy (+x), got (%v,%v)", dx, dy)
	}
}

func TestAutoFireIgnoresEnemiesBeyondRange(t *testing.T) {
	// The computer must not snipe enemies before they reach the screen: the kill
	// happens where the player can watch it.
	g := newCombatGame()
	g.x, g.y = 0, 0
	g.entities = []entity{{kind: kindEnemy, x: autoFireRange + 50, y: 0, radius: 5, hp: 3}}
	if _, _, ok := g.nearestEnemyDir(); ok {
		t.Fatal("an enemy beyond autoFireRange must not be targeted")
	}

	g.entities[0].x = autoFireRange - 50
	if _, _, ok := g.nearestEnemyDir(); !ok {
		t.Fatal("an enemy inside autoFireRange should be targeted")
	}
}

func TestAutoTargetOnlyWhenEngaged(t *testing.T) {
	g := newCombatGame()
	g.entities = []entity{{kind: kindEnemy, x: 40, y: 0, radius: 5, hp: 3}}

	g.autoFire = false
	g.updateAutoTarget()
	if g.autoAimOK {
		t.Fatal("with the computer off there should be no auto target")
	}

	g.autoFire = true
	g.updateAutoTarget()
	if !g.autoAimOK {
		t.Fatal("with the computer on and an enemy in sight there should be a target")
	}
}

func TestWeaponAimUsesAutoTargetWhenEngaged(t *testing.T) {
	g := &Game{}
	g.autoFire = true
	g.autoAimOK = true
	g.autoAimX, g.autoAimY = 0, -1 // straight up
	dx, dy, ok := g.weaponAimDir()
	if !ok || dx != 0 || dy != -1 {
		t.Fatalf("aimed weapons should use the auto target, got (%v,%v,%v)", dx, dy, ok)
	}
}

func TestAutoFireWithNoTargetHoldsFire(t *testing.T) {
	g := &Game{}
	g.autoFire = true
	g.autoAimOK = false // no enemy in sight
	g.runAutoFire()
	if len(g.projectiles) != 0 {
		t.Fatalf("the computer should hold fire without a target, got %d shots", len(g.projectiles))
	}
}
