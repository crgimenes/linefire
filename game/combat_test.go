package game

import (
	"math"
	"testing"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/level"
)

func newCombatGame() *Game {
	l := level.New()
	l.PlayerStart = level.Start{X: 10, Y: 20, Angle: -90}
	return &Game{
		level:  l,
		health: maxHealth,
		lives:  startLives,
		x:      300, y: 300, angle: 0,
		radius: 10,
	}
}

func TestHurtPlayerJustReducesHealth(t *testing.T) {
	g := newCombatGame()
	g.hurtPlayer(30)
	if g.health != maxHealth-30 || g.lives != startLives {
		t.Fatalf("health=%d lives=%d, want %d/%d", g.health, g.lives, maxHealth-30, startLives)
	}
}

func TestHurtPlayerCostsLifeButKeepsPosition(t *testing.T) {
	g := newCombatGame() // ship at (300,300); the level start is (10,20)
	g.health = 10
	g.hurtPlayer(20) // drops to <=0

	if g.lives != startLives-1 {
		t.Fatalf("lives = %d, want %d", g.lives, startLives-1)
	}
	if g.health != maxHealth {
		t.Fatalf("health = %d, want refilled to %d", g.health, maxHealth)
	}
	if g.x != 300 || g.y != 300 {
		t.Fatalf("losing a life must NOT teleport the ship, got (%v,%v)", g.x, g.y)
	}
	if g.invuln <= 0 {
		t.Fatal("expected brief invulnerability after losing a life")
	}
}

func TestHurtPlayerGameOverOnLastLife(t *testing.T) {
	g := newCombatGame()
	g.lives = 1
	g.health = 5
	g.hurtPlayer(10)

	if g.lives != 0 {
		t.Fatalf("the last life should be spent, got %d", g.lives)
	}
	if g.endKind != endDeath {
		t.Fatalf("the killing blow should queue GAME OVER behind the delay, endKind=%d", g.endKind)
	}
	if g.over {
		t.Fatal("game over must wait for the death to play out, not latch instantly")
	}
	g.endTicks = 1 // fast-forward the aftermath
	_ = g.stepEnding()
	if !g.over {
		t.Fatal("after the delay GAME OVER should latch")
	}
}

func TestInvulnerabilityBlocksDamage(t *testing.T) {
	g := newCombatGame()
	g.invuln = 30
	g.hurtPlayer(50)
	if g.health != maxHealth {
		t.Fatalf("invulnerable player took damage: health=%d", g.health)
	}
}

func TestEnemyFireAimsAtPlayer(t *testing.T) {
	g := newCombatGame()
	g.x, g.y = 100, 0
	g.entities = []entity{{kind: kindEnemy, x: 0, y: 0, hp: enemyHP}}
	g.updateEnemies() // fireCD starts at 0, so it fires this frame

	if len(g.enemyShots) != 1 {
		t.Fatalf("enemyShots = %d, want 1", len(g.enemyShots))
	}
	s := g.enemyShots[0]
	if s.vx <= 0 || s.vy != 0 {
		t.Fatalf("shot not aimed at player to the right: v=(%v,%v)", s.vx, s.vy)
	}
}

func TestEnemyHoldsFireWithoutLineOfSight(t *testing.T) {
	g := newCombatGame()
	g.x, g.y = 100, 0
	g.entities = []entity{{kind: kindEnemy, x: 0, y: 0, hp: enemyHP}}

	// A wall between enemy (0,0) and player (100,0) blocks the shot.
	g.segs = []segment{{ax: 50, ay: -10, bx: 50, by: 10}}
	g.updateEnemies()
	if len(g.enemyShots) != 0 {
		t.Fatalf("enemy should hold fire behind a wall, got %d shots", len(g.enemyShots))
	}

	// Clear line of sight: it fires.
	g.segs = nil
	g.entities[0].fireCD = 0
	g.updateEnemies()
	if len(g.enemyShots) == 0 {
		t.Fatal("enemy with a clear line of sight should fire")
	}
}

func TestLineOfSight(t *testing.T) {
	g := &Game{segs: []segment{{ax: 5, ay: -5, bx: 5, by: 5}}} // vertical wall at x=5
	if g.lineOfSight(0, 0, 10, 0) {
		t.Fatal("a wall across the ray should block sight")
	}
	if !g.lineOfSight(0, 0, 0, 10) {
		t.Fatal("a ray clear of walls should have sight")
	}
}

func TestEnemyShotKillingPlayerDoesNotPanic(t *testing.T) {
	g := newCombatGame()
	g.x, g.y = 0, 0
	g.radius = 10
	g.health = 10
	g.lives = 2
	// Two shots both sitting on the player; the first drops health to zero and
	// triggers a respawn. The loop must survive that mid-iteration state change.
	g.enemyShots = []projectile{
		{x: 0, y: 0, life: 10},
		{x: 0, y: 0, life: 10},
	}
	g.stepEnemyShots() // must not panic

	if g.lives != 1 {
		t.Fatalf("player should have lost exactly one life, lives=%d", g.lives)
	}
	if g.invuln <= 0 {
		t.Fatal("player should be invulnerable after respawn")
	}
}

func TestEnemyIdleOutsideRadar(t *testing.T) {
	g := newCombatGame()
	g.x, g.y = 10000, 0 // far beyond radarRange
	g.entities = []entity{{kind: kindEnemy, x: 0, y: 0, angle: 90, hp: enemyHP}}
	g.updateEnemies()

	if len(g.enemyShots) != 0 {
		t.Fatalf("enemy out of radar should not fire, got %d shots", len(g.enemyShots))
	}
	if g.entities[0].combat {
		t.Fatal("enemy out of radar should not be in combat (it patrols instead)")
	}
}

func TestEnemyEngagesInsideRadar(t *testing.T) {
	g := newCombatGame()
	g.x, g.y = 100, 0 // within radarRange, clear line of sight
	g.entities = []entity{{kind: kindEnemy, x: 0, y: 0, angle: 90, hp: enemyHP}}
	g.updateEnemies()

	if len(g.enemyShots) != 1 {
		t.Fatalf("enemy in radar with sight should fire, got %d shots", len(g.enemyShots))
	}
	// It turns toward the player (target 0) from its start of 90, rate-limited.
	a := g.entities[0].angle
	if a < 0 || a >= 90 {
		t.Fatalf("enemy should turn toward the player (90 -> 0): angle=%v", a)
	}
}

func TestEnemyRadarExpandsWhenHitAndCaps(t *testing.T) {
	g := &Game{}
	g.entities = []entity{{kind: kindEnemy, hp: 99, radar: radarRange}}

	g.damageEnemy(0, playerShotDamage, damageColor)
	if g.entities[0].radar <= radarRange {
		t.Fatalf("radar should grow after a hit, got %v", g.entities[0].radar)
	}

	for range 6 {
		g.damageEnemy(0, playerShotDamage, damageColor) // keeps surviving (hp 99)
	}
	if g.entities[0].radar > radarRange*radarMaxMul+1e-9 {
		t.Fatalf("radar exceeded the cap: %v > %v", g.entities[0].radar, radarRange*radarMaxMul)
	}
}

func TestHitEnemyRetaliatesFromBeyondBaseRadar(t *testing.T) {
	g := newCombatGame()
	// Player just past the base radar, clear line of sight (no walls).
	g.x, g.y = radarRange+40, 0
	g.entities = []entity{{kind: kindEnemy, x: 0, y: 0, hp: 3, radar: radarRange}}

	// Out of base radar: it ignores the player.
	g.updateEnemies()
	if len(g.enemyShots) != 0 {
		t.Fatalf("enemy beyond radar should not fire yet, got %d", len(g.enemyShots))
	}

	// Hit it: radar widens to include the player, so next update it fires back.
	g.damageEnemy(0, playerShotDamage, damageColor)
	g.updateEnemies()
	if len(g.enemyShots) == 0 {
		t.Fatal("a hit enemy should retaliate against a player just beyond base radar")
	}
}

func TestAimDirFromUnprojectsMouse(t *testing.T) {
	// Ship at origin facing up (-90): no camera rotation, scale 0.6, center 400.
	g := &Game{x: 0, y: 0, angle: -90}

	// Mouse above the screen center maps to a world point above the ship -> up.
	dx, dy, ok := g.aimDirFrom(400, 300)
	if !ok || math.Abs(dx) > 1e-9 || math.Abs(dy+1) > 1e-9 {
		t.Fatalf("aim = (%v,%v) ok=%v, want (0,-1)", dx, dy, ok)
	}
	// Mouse on the ship center yields no direction.
	_, _, ok = g.aimDirFrom(400, 400)
	if ok {
		t.Fatal("mouse on the ship should give no aim direction")
	}
}

func TestProjectileHitsEnemy(t *testing.T) {
	g := &Game{}
	g.entities = []entity{{kind: kindEnemy, x: 5, y: 0, radius: 2, hp: 1}}
	g.projectiles = []projectile{{x: 0, y: 0, vx: 10, vy: 0, life: 10, dmg: playerShotDamage, col: damageColor}}
	g.stepProjectiles()

	if len(g.projectiles) != 0 {
		t.Fatalf("a shot should be consumed on enemy hit, got %d", len(g.projectiles))
	}
	if g.enemiesLeft() != 0 {
		t.Fatal("the enemy should be destroyed by the shot")
	}
}

func TestWorldLockedAimIgnoresShipRotation(t *testing.T) {
	g := &Game{x: 0, y: 0, angle: -90}
	g.toggleAimMode() // lock at heading -90
	if !g.aimLocked {
		t.Fatal("expected aim to be locked")
	}

	dx0, dy0, ok := g.aimDirFrom(400, 300)
	if !ok {
		t.Fatal("expected an aim direction")
	}
	// Rotate the ship: a locked aim must stay put.
	g.angle = 30
	dx1, dy1, _ := g.aimDirFrom(400, 300)
	if math.Abs(dx0-dx1) > 1e-9 || math.Abs(dy0-dy1) > 1e-9 {
		t.Fatalf("locked aim changed with rotation: (%v,%v) -> (%v,%v)", dx0, dy0, dx1, dy1)
	}

	// Screen mode: the same rotation does move the aim.
	g.aimLocked = false
	dx2, dy2, _ := g.aimDirFrom(400, 300)
	if math.Abs(dx1-dx2) < 1e-9 && math.Abs(dy1-dy2) < 1e-9 {
		t.Fatal("screen-mode aim should follow the ship rotation")
	}
}

func TestEnemyApproachAndOrbitHold(t *testing.T) {
	// Far away (no nav): pursuePath falls back to heading straight at the player.
	g := &Game{}
	g.x, g.y = 0, 0
	g.entities = []entity{{kind: kindEnemy, x: 1000, y: 0, radius: 10, orbitDir: 1}}
	g.pursuePath(&g.entities[0], g.x, g.y)
	if g.entities[0].x >= 1000 {
		t.Fatalf("far enemy should close in, x=%v", g.entities[0].x)
	}

	// Too close: orbitHold backs it off the standoff.
	g.entities = []entity{{kind: kindEnemy, x: 50, y: 0, radius: 10, orbitDir: 1, standoff: enemyStandoff}}
	g.orbitHold(&g.entities[0], g.x, g.y, 50, enemyStandoff)
	if g.entities[0].x <= 50 {
		t.Fatalf("too-close enemy should back off, x=%v", g.entities[0].x)
	}
}

func TestEnemyMoveBlockedByWall(t *testing.T) {
	g := &Game{segs: []segment{{ax: 10, ay: -50, bx: 10, by: 50}}} // wall at x=10
	g.entities = []entity{{kind: kindEnemy, x: 0, y: 0, radius: 5}}
	g.moveEnemy(&g.entities[0], 7, 0) // step into the wall (radius 5 reaches x=10)
	if g.entities[0].x != 0 {
		t.Fatalf("enemy should be blocked by the wall, x=%v", g.entities[0].x)
	}
}

func TestWallImpactDamagesOnlyFastHits(t *testing.T) {
	g := newCombatGame()
	g.health = 100

	g.wallImpact(2) // below the speed floor
	if g.health != 100 {
		t.Fatalf("slow wall contact should not hurt, health=%d", g.health)
	}
	g.wallImpact(6) // hard impact
	if g.health >= 100 {
		t.Fatal("a fast wall impact should hurt")
	}
}

func TestResolveContactsDamagesPlayerAndEnemy(t *testing.T) {
	g := newCombatGame()
	g.x, g.y = 0, 0
	g.radius = 10
	g.health = 100
	g.entities = []entity{{kind: kindEnemy, x: 5, y: 0, radius: 10, hp: 3}}

	g.resolveContacts()
	if g.health >= 100 {
		t.Fatal("ramming an enemy should hurt the player")
	}
	if g.enemiesLeft() != 1 || g.entities[0].hp != 2 {
		t.Fatalf("the rammed enemy should take one hit: left=%d hp=%d", g.enemiesLeft(), g.entities[0].hp)
	}
	if g.invuln <= 0 {
		t.Fatal("contact should grant brief invulnerability")
	}
}

func TestEnemyRadarDoublesOnEngage(t *testing.T) {
	g := newCombatGame()
	g.x, g.y = 100, 0 // within base radar, clear line of sight
	g.entities = []entity{{kind: kindEnemy, x: 0, y: 0, hp: enemyHP, radar: radarRange}}

	g.updateEnemies()
	if g.entities[0].radar < radarRange*radarCombatMul-1e-9 {
		t.Fatalf("radar should double on engaging, got %v want >= %v", g.entities[0].radar, radarRange*radarCombatMul)
	}
}

func TestEnemySeparationPushesApart(t *testing.T) {
	g := &Game{}
	g.entities = []entity{
		{kind: kindEnemy, x: 0, y: 0, radius: 10},
		{kind: kindEnemy, x: 20, y: 0, radius: 10},
	}

	// Neighbor to the right -> push left.
	sx, _ := g.separation(&g.entities[0])
	if sx >= 0 {
		t.Fatalf("enemy with a neighbor to its right should be pushed left, sx=%v", sx)
	}

	// Far apart -> no separation.
	g.entities[1].x = 10000
	sx, sy := g.separation(&g.entities[0])
	if sx != 0 || sy != 0 {
		t.Fatalf("distant enemies should not separate, got (%v,%v)", sx, sy)
	}
}

func TestEnemyInCombatTracksThroughWallButHoldsFire(t *testing.T) {
	g := newCombatGame()
	g.x, g.y = 100, 0
	g.entities = []entity{{kind: kindEnemy, x: 0, y: 0, hp: enemyHP, radar: radarRange, orbitDir: 1}}

	// Engage with a clear line of sight.
	g.updateEnemies()
	if !g.entities[0].combat {
		t.Fatal("enemy should enter combat after engaging in sight")
	}

	// Put a wall between them: it must keep tracking (in combat) but not fire.
	g.enemyShots = nil
	g.entities[0].fireCD = 0
	g.segs = []segment{{ax: 50, ay: -80, bx: 50, by: 80}}
	g.updateEnemies()
	if len(g.enemyShots) != 0 {
		t.Fatalf("enemy must not fire through a wall, got %d shots", len(g.enemyShots))
	}
	if !g.entities[0].combat {
		t.Fatal("enemy should stay in combat tracking a hidden player within radar")
	}

	// Player escapes far beyond the widened radar: after the grace period it
	// drops out of combat.
	g.x = 10000
	g.entities[0].alertGrace = 1
	g.updateEnemies()
	if g.entities[0].combat {
		t.Fatal("enemy should leave combat once the player is gone past the grace period")
	}
}

func TestNavgridHeatHigherNearWalls(t *testing.T) {
	l := level.New()
	l.Walls[0].Paths = []asset.Path{{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 0, Y: 0}, {Op: asset.OpLineTo, X: 0, Y: 500},
	}}}
	segs := wallSegments(l)
	n := buildNavgrid(segs, mapBounds(l, segs))

	ncx, ncy := n.cellOf(60, 250)  // close to the wall at x=0 (passable but warm)
	fcx, fcy := n.cellOf(400, 250) // well clear of the wall
	near := n.heatAt(ncy*n.cols + ncx)
	far := n.heatAt(fcy*n.cols + fcx)
	if !(near > far) {
		t.Fatalf("heat should be higher near a wall: near=%v far=%v", near, far)
	}
}

func TestUnstickNudgesSideways(t *testing.T) {
	g := &Game{x: 100, y: 0} // player to the right
	g.entities = []entity{{kind: kindEnemy, x: 0, y: 0, radius: 5, orbitDir: 1}}

	g.unstick(&g.entities[0])
	if g.entities[0].y == 0 {
		t.Fatalf("unstick should nudge sideways (perpendicular to the player), y=%v", g.entities[0].y)
	}
	if g.entities[0].repathCD != 0 {
		t.Fatal("unstick should force a repath")
	}
}

func TestTurnTowardLimitsAndWraps(t *testing.T) {
	got := turnToward(0, 90, 8)
	if got != 8 {
		t.Fatalf("turnToward(0,90,8) = %v, want 8", got)
	}
	got = turnToward(0, -90, 8)
	if got != -8 {
		t.Fatalf("turnToward(0,-90,8) = %v, want -8", got)
	}
	got = turnToward(0, 5, 8)
	if got != 5 {
		t.Fatalf("turnToward should reach a near target: %v, want 5", got)
	}
	// Across the +180/-180 seam: 170 -> -170 is +20, clamped to +8 -> 178.
	got = turnToward(170, -170, 8)
	if math.Abs(got-178) > 1e-9 {
		t.Fatalf("turnToward wrap = %v, want 178", got)
	}
}

func TestPickupHealAndScore(t *testing.T) {
	g := newCombatGame()
	g.x, g.y = 0, 0
	g.radius = 10
	g.health = 20

	// Heal power-up overlapping the player.
	g.entities = []entity{{kind: kindPowerUp, x: 5, y: 0, radius: 10, power: powerHeal}}
	g.resolvePickups()
	if g.health != 20+healAmount {
		t.Fatalf("heal pickup: health=%d, want %d", g.health, 20+healAmount)
	}
	if len(g.entities) != 0 {
		t.Fatal("heal pickup should be consumed")
	}

	// Score power-up.
	g.entities = []entity{{kind: kindPowerUp, x: 5, y: 0, radius: 10, power: powerScore}}
	g.resolvePickups()
	if g.score != scoreBonus {
		t.Fatalf("score pickup: score=%d, want %d", g.score, scoreBonus)
	}

	// Out of reach: not collected.
	g.entities = []entity{{kind: kindPowerUp, x: 1000, y: 0, radius: 10, power: powerHeal}}
	g.resolvePickups()
	if len(g.entities) != 1 {
		t.Fatal("distant power-up should not be collected")
	}
}

func TestHealPickupCapsAtMaxHealth(t *testing.T) {
	g := newCombatGame()
	g.x, g.y = 0, 0
	g.radius = 10
	g.health = maxHealth - 5
	g.entities = []entity{{kind: kindPowerUp, x: 0, y: 0, radius: 10, power: powerHeal}}
	g.resolvePickups()
	if g.health != maxHealth {
		t.Fatalf("heal should cap at max: health=%d, want %d", g.health, maxHealth)
	}
}

func TestIdleEnemyPatrols(t *testing.T) {
	g := newCombatGame()
	g.x, g.y = 100000, 0 // player far away: enemy stays idle
	g.entities = []entity{{kind: kindEnemy, x: 0, y: 0, radius: 10, hp: enemyHP, wanderHead: 0}}

	bx, by := g.entities[0].x, g.entities[0].y
	for range 5 {
		g.updateEnemies()
	}
	if g.entities[0].x == bx && g.entities[0].y == by {
		t.Fatal("an idle enemy should wander, not stand still")
	}
}

func TestCombatGraceKeepsChasingBriefly(t *testing.T) {
	g := newCombatGame()
	e := &entity{kind: kindEnemy, radar: radarRange, combat: true, alertGrace: 2}

	// In range refreshes the grace and stays in combat.
	if !g.enemyEngaged(e, true, true) || e.alertGrace != combatGrace {
		t.Fatalf("in-range should refresh grace: combat=%v grace=%d", e.combat, e.alertGrace)
	}

	// Out of range: counts down but stays in combat until it expires.
	e.alertGrace = 2
	if !g.enemyEngaged(e, false, false) || !e.combat {
		t.Fatal("should keep combat during the grace period")
	}
	g.enemyEngaged(e, false, false)
	if e.combat {
		t.Fatal("should drop combat after the grace period expires")
	}
}
