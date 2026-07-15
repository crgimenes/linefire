package game

import (
	"math"
	"testing"
)

func TestLaserPiercesEnemiesInBeam(t *testing.T) {
	g := &Game{}
	g.x, g.y = 0, 0
	g.laserX1, g.laserY1 = 200, 0 // beam along +x
	g.laserOn = true
	g.entities = []entity{
		{kind: kindEnemy, x: 50, y: 0, radius: 5, hp: 99},  // on the beam
		{kind: kindEnemy, x: 150, y: 0, radius: 5, hp: 99}, // on the beam (behind the first)
		{kind: kindEnemy, x: 50, y: 50, radius: 5, hp: 99}, // off the beam
	}

	if !g.fireLaserTick(&weaponLaser) {
		t.Fatal("an active laser should tick")
	}
	if g.entities[0].hp != 99-laserDamage || g.entities[1].hp != 99-laserDamage {
		t.Fatalf("the beam should pierce and damage both in-line enemies: %d,%d", g.entities[0].hp, g.entities[1].hp)
	}
	if g.entities[2].hp != 99 {
		t.Fatalf("an enemy off the beam should be untouched, hp=%d", g.entities[2].hp)
	}
}

func TestLaserInactiveDoesNothing(t *testing.T) {
	g := &Game{}
	g.entities = []entity{{kind: kindEnemy, x: 10, y: 0, radius: 5, hp: 99}}
	if g.fireLaserTick(&weaponLaser) {
		t.Fatal("an inactive laser (laserOn false) should not tick")
	}
	if g.entities[0].hp != 99 {
		t.Fatalf("no damage when the laser is off, hp=%d", g.entities[0].hp)
	}
}

func TestLaserEndpointStopsAtWall(t *testing.T) {
	g := &Game{segs: []segment{{ax: 100, ay: -50, bx: 100, by: 50}}} // vertical wall at x=100
	ex, ey, hit := g.laserEndpoint(0, 0, 1, 0, laserRange)
	if math.Abs(ex-100) > 1e-6 || math.Abs(ey) > 1e-6 {
		t.Fatalf("beam should stop at the wall x=100, got (%v,%v)", ex, ey)
	}
	if !hit {
		t.Fatal("a beam that stopped on a wall must report the hit, so its tick melts it")
	}
}

func TestLaserEndpointReachesMaxWithoutWall(t *testing.T) {
	g := &Game{} // no walls
	ex, ey, hit := g.laserEndpoint(0, 0, 1, 0, laserRange)
	if math.Abs(ex-laserRange) > 1e-6 || math.Abs(ey) > 1e-6 {
		t.Fatalf("with no wall the beam should reach its full range, got (%v,%v)", ex, ey)
	}
	if hit {
		t.Fatal("a beam that faded out at max range hit nothing, so it must melt nothing")
	}
}
