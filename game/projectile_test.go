package game

import (
	"math"
	"testing"

	"github.com/crgimenes/linefire/asset"
)

func TestMuzzleWorldPointingUp(t *testing.T) {
	a := asset.New() // origin (32,32)
	a.Hardpoints = []asset.Hardpoint{{Name: "g", Kind: asset.KindWeapon, X: 32, Y: 8, Angle: -90}}
	g := &Game{player: a, x: 100, y: 200, angle: -90}

	// Heading up (-90): the muzzle 24 units ahead of the origin sits directly
	// above the ship in world space.
	mx, my := g.muzzleWorld()
	if math.Abs(mx-100) > 1e-9 || math.Abs(my-176) > 1e-9 {
		t.Fatalf("muzzle = (%v,%v), want (100,176)", mx, my)
	}
}

func TestSegmentsIntersect(t *testing.T) {
	if !segmentsIntersect(0, 0, 10, 10, 0, 10, 10, 0) {
		t.Fatal("crossing segments should intersect")
	}
	if segmentsIntersect(0, 0, 10, 0, 0, 5, 10, 5) {
		t.Fatal("parallel segments should not intersect")
	}
	if segmentsIntersect(0, 0, 1, 0, 5, 0, 6, 0) {
		t.Fatal("disjoint collinear segments should not intersect")
	}
}

func TestStepProjectilesExpiresAndMoves(t *testing.T) {
	g := &Game{}
	g.projectiles = []projectile{
		{x: 0, y: 0, vx: 5, vy: 0, life: 2},
		{x: 0, y: 0, vx: 5, vy: 0, life: 1}, // expires this step
	}
	g.stepProjectiles()

	if len(g.projectiles) != 1 {
		t.Fatalf("projectiles = %d, want 1 after expiry", len(g.projectiles))
	}
	p := g.projectiles[0]
	if p.x != 5 || p.px != 0 {
		t.Fatalf("moved bullet = (x %v, px %v), want (5, 0)", p.x, p.px)
	}
}

func TestStepProjectilesStopsAtWall(t *testing.T) {
	g := &Game{segs: []segment{{ax: 3, ay: -5, bx: 3, by: 5}}} // vertical wall at x=3
	g.projectiles = []projectile{{x: 0, y: 0, vx: 6, vy: 0, life: 10}}
	g.stepProjectiles()

	if len(g.projectiles) != 0 {
		t.Fatalf("bullet should be removed on wall hit, got %d", len(g.projectiles))
	}
}

func TestBulletHitsAndDestroysEnemy(t *testing.T) {
	g := &Game{}
	g.entities = []entity{{kind: kindEnemy, x: 5, y: 0, radius: 2, hp: 1}}
	g.projectiles = []projectile{{x: 0, y: 0, vx: 10, vy: 0, life: 10, dmg: playerShotDamage, col: damageColor}}
	g.stepProjectiles()

	if len(g.projectiles) != 0 {
		t.Fatalf("bullet should be consumed on enemy hit, got %d", len(g.projectiles))
	}
	if g.enemiesLeft() != 0 {
		t.Fatalf("enemy should be destroyed, %d left", g.enemiesLeft())
	}
	if g.score != 1 {
		t.Fatalf("score = %d, want 1", g.score)
	}
}

func TestEnemySurvivesUntilHealthGone(t *testing.T) {
	g := &Game{}
	g.entities = []entity{{kind: kindEnemy, x: 5, y: 0, radius: 2, hp: 2}}
	g.damageEnemy(0, playerShotDamage, damageColor, 0)
	if g.enemiesLeft() != 1 || g.score != 0 {
		t.Fatalf("enemy should survive first hit: left=%d score=%d", g.enemiesLeft(), g.score)
	}
	g.damageEnemy(0, playerShotDamage, damageColor, 0)
	if g.enemiesLeft() != 0 || g.score != 1 {
		t.Fatalf("enemy should die on second hit: left=%d score=%d", g.enemiesLeft(), g.score)
	}
}

func TestWeaponDamageRoutesToNumbers(t *testing.T) {
	// Front gun: deals playerShotDamage and pops that number.
	g := &Game{}
	g.entities = []entity{{kind: kindEnemy, x: 10, y: 0, radius: 4, hp: 5}}
	g.projectiles = []projectile{{x: 0, y: 0, vx: bulletSpeed, vy: 0, life: bulletLife, dmg: playerShotDamage, col: damageColor}}
	g.stepProjectiles()
	if g.entities[0].hp != 5-playerShotDamage {
		t.Fatalf("front gun should deal %d, enemy hp=%d", playerShotDamage, g.entities[0].hp)
	}
	if len(g.floaters) == 0 || g.floaters[0].text != "1" {
		t.Fatalf("front-gun hit should pop \"1\", got %+v", g.floaters)
	}
	if g.floaters[0].col != damageColor {
		t.Fatalf("front-gun number should use damageColor, got %+v", g.floaters[0].col)
	}

	// A heavier hit pops its own number and is colored by the projectile's own col, so
	// different weapons read apart on screen.
	g2 := &Game{}
	g2.entities = []entity{{kind: kindEnemy, x: 10, y: 0, radius: 4, hp: 5}}
	g2.projectiles = []projectile{{x: 0, y: 0, vx: bulletSpeed, vy: 0, life: bulletLife, dmg: 2, col: missileDamageColor}}
	g2.stepProjectiles()
	if g2.entities[0].hp != 5-2 {
		t.Fatalf("a 2-damage shot should leave hp=3, got %d", g2.entities[0].hp)
	}
	if len(g2.floaters) == 0 || g2.floaters[0].text != "2" {
		t.Fatalf("a 2-damage hit should pop \"2\", got %+v", g2.floaters)
	}
	if g2.floaters[0].col != missileDamageColor {
		t.Fatalf("the damage number should use the projectile's col, got %+v", g2.floaters[0].col)
	}
}

func TestBulletHitsWallWhenLandingExactlyOnLine(t *testing.T) {
	// Horizontal wall at y=10; a shot whose travel ends exactly on that line
	// must still be stopped (the old strict-crossing test tunnelled here).
	g := &Game{segs: []segment{{ax: -5, ay: 10, bx: 5, by: 10}}}
	if !g.bulletHitsWall(0, 0, 0, 10) {
		t.Fatal("travel ending exactly on the wall line should count as a hit")
	}
	// A shot that ends just shy of the wall is also caught within tolerance.
	if !g.bulletHitsWall(0, 0, 0, 9) {
		t.Fatal("travel ending within tolerance of the wall should count as a hit")
	}
	// A shot well clear of the wall passes.
	if g.bulletHitsWall(0, 0, 0, 5) {
		t.Fatal("travel clear of the wall should not hit")
	}
}
