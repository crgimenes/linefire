package game

import (
	"image/color"
	"testing"
)

func TestImpactBurstScalesWithDamage(t *testing.T) {
	col := color.RGBA{0x80, 0xff, 0xff, 0xff}
	light := &projectile{dmg: 1, rglow: col}
	heavy := &projectile{dmg: 4, rglow: col}

	g := &Game{}
	g.impactBurst(0, 0, light, false)
	lightN := len(g.particles)

	g = &Game{}
	g.impactBurst(0, 0, heavy, false)
	heavyN := len(g.particles)

	if heavyN <= lightN {
		t.Fatalf("a heavier shot should throw more sparks: light=%d heavy=%d", lightN, heavyN)
	}
	if lightN == 0 {
		t.Fatal("even a light shot should spark")
	}
}

func TestImpactBurstShakesOnlyOnEnemyHit(t *testing.T) {
	col := color.RGBA{0x80, 0xff, 0xff, 0xff}
	p := &projectile{dmg: 2, rglow: col}

	g := &Game{}
	g.impactBurst(0, 0, p, false) // wall miss: no reward shake
	if g.shakeMag != 0 {
		t.Fatalf("a wall miss should not shake, got %v", g.shakeMag)
	}

	g.impactBurst(0, 0, p, true) // connected: a small punch
	if g.shakeMag <= 0 {
		t.Fatal("connecting with an enemy should shake")
	}
}
