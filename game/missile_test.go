package game

import "testing"

func TestFireSetsShotPayload(t *testing.T) {
	g := &Game{} // nil player: muzzleWorld falls back to the ship position
	g.fireProjectileWeapon(&weaponFrontGun, aimForward)
	if len(g.projectiles) != 1 {
		t.Fatalf("firing the front gun should spawn one bullet, got %d", len(g.projectiles))
	}
	p := g.projectiles[0]
	if p.dmg != playerShotDamage || p.col != damageColor || p.aoe != 0 {
		t.Fatalf("front-gun shot payload = {dmg %d, col %+v, aoe %v}, want {%d, %+v, 0}",
			p.dmg, p.col, p.aoe, playerShotDamage, damageColor)
	}
}

func TestExplodeFalloffDamagesByDistance(t *testing.T) {
	g := &Game{}
	g.entities = []entity{
		{kind: kindEnemy, x: 0, y: 0, radius: 3, hp: 99},                 // center: full damage
		{kind: kindEnemy, x: missileRadius / 2, y: 0, radius: 3, hp: 99}, // half radius: ~half
		{kind: kindEnemy, x: missileRadius * 2, y: 0, radius: 3, hp: 99}, // out of range: untouched
	}

	g.explodeAt(0, 0, missileDamage, missileRadius, missileDamageColor)

	center := 99 - g.entities[0].hp
	mid := 99 - g.entities[1].hp
	far := 99 - g.entities[2].hp
	if center != missileDamage {
		t.Fatalf("center enemy should take full %d, took %d", missileDamage, center)
	}
	if mid <= 0 || mid >= center {
		t.Fatalf("mid enemy should take partial damage (0 < %d < %d)", mid, center)
	}
	if far != 0 {
		t.Fatalf("enemy beyond the radius should be untouched, took %d", far)
	}
	// Two in-range enemies → two damage numbers + an expanding ring.
	if len(g.floaters) != 2 {
		t.Fatalf("an in-range hit should pop a number per enemy, got %d", len(g.floaters))
	}
	if len(g.shocks) != 1 {
		t.Fatalf("a detonation should ring one shockwave, got %d", len(g.shocks))
	}
}

func TestMissileShotDetonatesOnEnemy(t *testing.T) {
	g := &Game{}
	// Two enemies close together; a missile striking the first should also catch
	// the second through its area of effect.
	g.entities = []entity{
		{kind: kindEnemy, x: 10, y: 0, radius: 4, hp: 99},
		{kind: kindEnemy, x: 10, y: 12, radius: 4, hp: 99},
	}
	g.projectiles = []projectile{{
		x: 0, y: 0, vx: bulletSpeed, vy: 0, life: bulletLife,
		dmg: missileDamage, col: missileDamageColor, aoe: missileRadius,
	}}

	g.stepProjectiles()

	if len(g.projectiles) != 0 {
		t.Fatalf("the missile should be consumed on impact, %d left", len(g.projectiles))
	}
	if g.entities[0].hp >= 99 || g.entities[1].hp >= 99 {
		t.Fatalf("the blast should damage both nearby enemies: hp=%d,%d", g.entities[0].hp, g.entities[1].hp)
	}
	if g.shakeMag <= 0 {
		t.Fatal("a detonation should shake the screen")
	}
}

func TestShockwaveGrowsAndExpires(t *testing.T) {
	g := &Game{}
	g.spawnShockwave(0, 0, 100)
	if len(g.shocks) != 1 {
		t.Fatalf("shocks = %d, want 1", len(g.shocks))
	}
	r := g.shocks[0].radius()
	if r < 0 || r > 100 {
		t.Fatalf("ring radius should be within [0,100], got %v", r)
	}
	for range shockLife {
		g.stepShockwaves()
	}
	if len(g.shocks) != 0 {
		t.Fatalf("shockwaves should expire, %d left", len(g.shocks))
	}
}
