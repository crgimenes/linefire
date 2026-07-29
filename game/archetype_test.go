package game

import (
	"testing"

	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/level"
	"github.com/crgimenes/linefire/ship"
)

func TestEnemyArchetypeFromKind(t *testing.T) {
	l := level.New()
	l.Spawns = []level.Spawn{
		{Kind: "enemy", X: 0, Y: 0},
		{Kind: "turret", X: 100, Y: 0},
		{Kind: "rusher", X: 200, Y: 0},
		{Kind: "tank", X: 300, Y: 0},
	}
	es := buildEntities(filoio.OSFS(), l, "")
	if len(es) != 4 {
		t.Fatalf("want 4 entities, got %d", len(es))
	}
	for i, e := range es {
		if e.kind != kindEnemy {
			t.Fatalf("spawn %d should be an enemy, got kind %v", i, e.kind)
		}
	}
	if es[0].stationary {
		t.Fatal("the grunt should not be stationary")
	}
	if !es[1].stationary {
		t.Fatal("the turret should be stationary")
	}
	if es[1].hp != ship.For("turret").HP {
		t.Fatalf("turret hp = %d, want %d", es[1].hp, ship.For("turret").HP)
	}
	if es[2].speedMul != ship.For("rusher").SpeedMul {
		t.Fatalf("rusher speedMul = %v, want %v", es[2].speedMul, ship.For("rusher").SpeedMul)
	}
	if es[3].hp != ship.For("tank").HP {
		t.Fatalf("tank hp = %d, want %d", es[3].hp, ship.For("tank").HP)
	}
}

func TestTurretIsStationaryButFires(t *testing.T) {
	g := newCombatGame()
	g.entities = []entity{{
		kind: kindEnemy, stationary: true, x: 100, y: 100, radius: 10,
		hp: 6, radar: radarRange, fireEvery: 40, shotSpeed: enemyBulletSpeed, shotDmg: 16,
	}}
	g.x, g.y = 150, 100 // within radar, clear line of sight (no walls)
	ex, ey := g.entities[0].x, g.entities[0].y

	g.updateEnemies()
	if g.entities[0].x != ex || g.entities[0].y != ey {
		t.Fatalf("a turret must never move, went to (%v,%v)", g.entities[0].x, g.entities[0].y)
	}
	if len(g.enemyShots) == 0 {
		t.Fatal("a turret with a clear line of sight should fire")
	}
}

func TestEnemyShotUsesArchetypeDamage(t *testing.T) {
	g := newCombatGame() // health=maxHealth, ship at (300,300) radius 10
	g.enemyShots = []projectile{{x: g.x - 1, y: g.y, px: g.x - 1, py: g.y, vx: 1, vy: 0, life: 10, dmg: 32}}
	g.stepEnemyShots()
	if g.health != maxHealth-32 {
		t.Fatalf("enemy shot should deal its carried damage 32, health=%d", g.health)
	}
}

func TestMoveSpeedScalesWithArchetype(t *testing.T) {
	base := entity{} // unset speedMul -> baseline
	slow := entity{speedMul: 0.5}
	fast := entity{speedMul: 1.9}
	if base.moveSpeed() != enemySpeed {
		t.Fatalf("unset speedMul should give the baseline %v, got %v", enemySpeed, base.moveSpeed())
	}
	if slow.moveSpeed() >= base.moveSpeed() || fast.moveSpeed() <= base.moveSpeed() {
		t.Fatalf("speedMul should scale moveSpeed: slow=%v base=%v fast=%v",
			slow.moveSpeed(), base.moveSpeed(), fast.moveSpeed())
	}
}
