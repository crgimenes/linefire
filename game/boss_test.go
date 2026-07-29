package game

import (
	"testing"

	"github.com/crgimenes/linefire/asset"
)

// TestBossIsShieldedUntilEscortsCleared: a boss takes no damage while any non-boss enemy is alive,
// then becomes vulnerable the moment the room is cleared down to it.
func TestBossIsShieldedUntilEscortsCleared(t *testing.T) {
	lvl := boxLevel(400, 400, nil)
	g := New(asset.New(), lvl, "", false)
	g.hadEnemies = true
	g.entities = []entity{
		{kind: kindEnemy, boss: true, hp: 5, x: 100, y: 100, radius: 20, a: asset.New()},
		{kind: kindEnemy, hp: 3, x: 200, y: 200, radius: 20, a: asset.New()}, // the escort
	}

	if !g.escortsAlive() {
		t.Fatal("an escort is alive, so escortsAlive should be true")
	}

	g.damageEnemy(0, 3, damageColor) // fire at the boss while the escort lives
	if g.entities[0].hp != 5 {
		t.Fatalf("the shielded boss should take no damage, got hp %d", g.entities[0].hp)
	}

	g.damageEnemy(1, 3, damageColor) // kill the escort (hp 3, dmg 3)
	if g.escortsAlive() {
		t.Fatal("the only escort is dead — escortsAlive should be false")
	}
	if len(g.entities) != 1 || !g.entities[0].boss {
		t.Fatalf("only the boss should remain, got %d entities", len(g.entities))
	}

	g.damageEnemy(0, 3, damageColor) // now the boss is exposed
	if g.entities[0].hp != 2 {
		t.Fatalf("the exposed boss should take damage, got hp %d", g.entities[0].hp)
	}
}
