package game

import (
	"testing"

	"github.com/crgimenes/linefire/filoio"
)

func TestShieldAbsorbsDamageThenHealth(t *testing.T) {
	g := newCombatGame()
	g.health = 100
	g.shield = 30

	g.hurtPlayer(20) // fully absorbed by the shield
	if g.shield != 10 || g.health != 100 {
		t.Fatalf("shield should absorb: shield=%d health=%d, want 10/100", g.shield, g.health)
	}

	g.invuln = 0
	g.hurtPlayer(25) // 10 from shield, 15 from health
	if g.shield != 0 || g.health != 85 {
		t.Fatalf("overflow should hit health: shield=%d health=%d, want 0/85", g.shield, g.health)
	}
}

func TestShieldPickupGrantsAndCaps(t *testing.T) {
	g := newCombatGame()
	g.x, g.y = 0, 0
	g.radius = 10
	g.shield = maxShield - 10
	g.entities = []entity{{kind: kindPowerUp, x: 0, y: 0, radius: 10, power: powerShield}}
	g.resolvePickups()
	if g.shield != maxShield {
		t.Fatalf("shield pickup should cap at max: shield=%d, want %d", g.shield, maxShield)
	}
}

func TestShieldAssetLoads(t *testing.T) {
	_, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "shield"))
	if err != nil {
		t.Fatalf("shield asset: %v", err)
	}
}
