package game

import "testing"

func TestParticlesSpawnAgeAndExpire(t *testing.T) {
	g := &Game{}
	g.emitExplosion(100, 100)
	want := explosionStreaks.n + explosionChunks.n
	if len(g.particles) != want {
		t.Fatalf("particles = %d, want %d", len(g.particles), want)
	}

	// After well past the longest lifetime, every particle is gone.
	for range 100 {
		g.stepParticles()
	}
	if len(g.particles) != 0 {
		t.Fatalf("particles should all expire, %d left", len(g.particles))
	}
}

func TestParticlePoolIsBounded(t *testing.T) {
	g := &Game{}
	for range maxParticles { // far more emits than the cap allows
		g.emitExplosion(0, 0)
	}
	if len(g.particles) > maxParticles {
		t.Fatalf("particle pool grew past the cap: %d > %d", len(g.particles), maxParticles)
	}
}

func TestThrusterEmitsBehindShip(t *testing.T) {
	g := &Game{}
	g.x, g.y, g.radius = 100, 50, 8
	g.emitThruster(1, 0) // facing +x, so exhaust starts at the rear and moves -x

	if len(g.particles) != thrusterPerFrame {
		t.Fatalf("thruster particles = %d, want %d", len(g.particles), thrusterPerFrame)
	}
	for _, p := range g.particles {
		if p.x > g.x {
			t.Fatalf("exhaust should start at or behind the ship, got x=%v (ship x=%v)", p.x, g.x)
		}
		if p.vx >= 0 {
			t.Fatalf("exhaust should move backward (vx<0), got vx=%v", p.vx)
		}
	}
}

func TestShieldAbsorbEmitsSparks(t *testing.T) {
	g := &Game{}
	g.shield, g.health, g.lives = 5, maxHealth, 3
	g.hurtPlayer(2)

	if g.shield != 3 {
		t.Fatalf("shield should absorb the hit: shield=%d, want 3", g.shield)
	}
	if g.health != maxHealth {
		t.Fatalf("health should be untouched while the shield holds, got %d", g.health)
	}
	if len(g.particles) == 0 {
		t.Fatal("a shielded hit should emit deflection sparks")
	}
}

func TestBulletHittingWallSparks(t *testing.T) {
	g := &Game{}
	g.segs = []segment{{ax: 105, ay: -50, bx: 105, by: 50}} // vertical wall ahead
	g.projectiles = []projectile{{x: 100, y: 0, vx: bulletSpeed, vy: 0, life: bulletLife}}

	g.stepProjectiles()
	if len(g.projectiles) != 0 {
		t.Fatalf("the bullet should be consumed by the wall, %d left", len(g.projectiles))
	}
	if len(g.particles) == 0 {
		t.Fatal("a bullet striking a wall should emit sparks")
	}
}

func TestShakeAddTakesMaxThenDecays(t *testing.T) {
	g := &Game{}
	g.addShake(5)
	g.addShake(3) // smaller jolt must not lower the current shake
	if g.shakeMag != 5 {
		t.Fatalf("shakeMag = %v, want 5", g.shakeMag)
	}
	g.decayShake()
	if g.shakeMag >= 5 || g.shakeMag <= 0 {
		t.Fatalf("shakeMag = %v, want between 0 and 5 after decay", g.shakeMag)
	}
	for range 200 {
		g.decayShake()
	}
	if g.shakeMag != 0 {
		t.Fatalf("shakeMag = %v, want 0 after long decay", g.shakeMag)
	}
}

func TestDamageEnemyFlashesThenExplodes(t *testing.T) {
	g := &Game{}
	g.entities = []entity{{kind: kindEnemy, x: 40, y: 40, radius: 5, hp: 2}}

	g.damageEnemy(0, playerShotDamage, damageColor)
	if g.enemiesLeft() != 1 || g.entities[0].hitFlash <= 0 {
		t.Fatalf("first hit should flash and survive: left=%d flash=%d", g.enemiesLeft(), g.entities[0].hitFlash)
	}
	if len(g.particles) != 0 {
		t.Fatal("no explosion before the enemy dies")
	}

	g.damageEnemy(0, playerShotDamage, damageColor)
	if g.enemiesLeft() != 0 {
		t.Fatalf("second hit should destroy the enemy, %d left", g.enemiesLeft())
	}
	want := explosionStreaks.n + explosionChunks.n
	if len(g.particles) != want {
		t.Fatalf("death should spawn %d particles, got %d", want, len(g.particles))
	}
	if g.shakeMag <= 0 {
		t.Fatal("death should add screen shake")
	}
}
