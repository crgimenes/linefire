package game

import (
	"testing"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/level"
)

// breakableGame is a game with one power-up in the path of a rightward bullet.
func breakableGame() *Game {
	g := New(asset.New(), level.New(), "", false)
	g.entities = []entity{{kind: kindPowerUp, power: powerHeal, x: 30, y: 0, radius: 10, hp: pickupHP, spawn: 0}}
	return g
}

func shootRight(g *Game, dmg int) {
	g.projectiles = append(g.projectiles, projectile{
		x: 0, y: 0, vx: bulletSpeed, vy: 0, life: bulletLife, dmg: dmg,
	})
	for range 8 { // enough steps to cross the pickup
		g.stepProjectiles()
	}
}

func TestPlayerFireChipsAndBreaksPickup(t *testing.T) {
	g := breakableGame()

	shootRight(g, 1)
	if len(g.entities) != 1 || g.entities[0].hp != pickupHP-1 {
		t.Fatalf("one hit should chip the pickup to %d, got %+v", pickupHP-1, g.entities)
	}
	if len(g.projectiles) != 0 {
		t.Fatal("the bullet is consumed by the hit (pickups are solid cover)")
	}

	shootRight(g, 99) // overkill: breaks it
	if len(g.entities) != 0 {
		t.Fatal("enough fire should destroy the pickup")
	}
	if g.score != 0 {
		t.Fatal("breaking a pickup must not score")
	}
	if !g.curMapState().consumed[0] {
		t.Fatal("a destroyed pickup must stay gone for the run")
	}
}

func TestEnemyTakesTheHitOverAPickup(t *testing.T) {
	g := breakableGame()
	g.entities = append(g.entities, entity{kind: kindEnemy, x: 30, y: 0, radius: 10, hp: 99})

	shootRight(g, 1)

	for i := range g.entities {
		e := &g.entities[i]
		if e.kind == kindPowerUp && e.hp != pickupHP {
			t.Fatal("with an enemy on the same line, the pickup must not take the hit")
		}
		if e.kind == kindEnemy && e.hp != 98 {
			t.Fatalf("the enemy should take the hit, hp=%d", e.hp)
		}
	}
}
