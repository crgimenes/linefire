package game

import (
	"math"
	"testing"
)

func TestAddAllyCaps(t *testing.T) {
	g := &Game{}
	for i := range maxEscorts {
		if !g.addAlly(modeEscort) {
			t.Fatalf("escort should be added at step %d", i)
		}
	}
	if g.addAlly(modeEscort) {
		t.Fatal("an escort past the cap should be refused")
	}
	if g.countAllies(modeEscort) != maxEscorts {
		t.Fatalf("want %d escorts, got %d", maxEscorts, g.countAllies(modeEscort))
	}
	for i := range maxDrones {
		if !g.addAlly(modeOrbit) {
			t.Fatalf("drone should be added at step %d", i)
		}
	}
	if g.addAlly(modeOrbit) {
		t.Fatal("a drone past the cap should be refused")
	}
	if g.countAllies(modeOrbit) != maxDrones {
		t.Fatalf("want %d drones, got %d", maxDrones, g.countAllies(modeOrbit))
	}
}

func TestDroneOrbitsAtRadius(t *testing.T) {
	g := &Game{}
	g.addAlly(modeOrbit)
	for range 5 {
		g.stepAllies()
	}
	a := &g.allies[0]
	r := math.Hypot(a.x-g.x, a.y-g.y)
	if math.Abs(r-droneOrbitRadius) > 1e-6 {
		t.Fatalf("a drone should sit exactly on the orbit radius %.0f, got %.3f", droneOrbitRadius, r)
	}
}

func TestEscortRegroupsWhenNoEnemy(t *testing.T) {
	g := &Game{} // ship at origin, heading 0 (world +x)
	g.addAlly(modeEscort)
	for range 400 {
		g.stepAllies()
	}
	a := &g.allies[0]
	// idx 0: rank 1, side +1, back along -x, lateral along +y.
	wantX, wantY := -escortBack, escortSpacing
	if math.Hypot(a.x-wantX, a.y-wantY) > 0.5 {
		t.Fatalf("with no enemy an escort should settle in its slot (%.0f,%.0f), got (%.2f,%.2f)", wantX, wantY, a.x, a.y)
	}
}

// TestAggressiveEscortChargesEnemy: an escort breaks off to hunt a nearby enemy, closing to
// a shooting standoff instead of holding formation — and stays leashed to the ship.
func TestAggressiveEscortChargesEnemy(t *testing.T) {
	g := &Game{} // ship at origin
	g.addAlly(modeEscort)
	g.entities = []entity{{kind: kindEnemy, x: 250, y: 0, radius: 8, hp: 1000}} // within patrol range, tanky
	a := &g.allies[0]
	start := math.Hypot(a.x-250, a.y)
	for range 200 {
		g.stepAllies()
	}
	got := math.Hypot(a.x-250, a.y)
	if got >= start {
		t.Fatalf("an aggressive escort should close on the enemy: started %.1f away, ended %.1f", start, got)
	}
	if math.Abs(got-huntStandoff) > huntSpeed+1 {
		t.Fatalf("escort should hold ~standoff %.0f from the enemy, ended %.1f", huntStandoff, got)
	}
	if leash := math.Hypot(a.x-g.x, a.y-g.y); leash > huntLeash+1 {
		t.Fatalf("escort should stay within the leash %.0f of the ship, got %.1f", huntLeash, leash)
	}
}

// TestEscortIgnoresFarEnemy: an escort guards the ship — an enemy beyond the patrol range
// is not chased across the map; the escort regroups into its slot instead.
func TestEscortIgnoresFarEnemy(t *testing.T) {
	g := &Game{}
	g.addAlly(modeEscort)
	g.entities = []entity{{kind: kindEnemy, x: huntRange + 200, y: 0, radius: 8, hp: 1000}}
	for range 200 {
		g.stepAllies()
	}
	a := &g.allies[0]
	sx, sy := g.escortSlot(0)
	if math.Hypot(a.x-sx, a.y-sy) > 1.0 {
		t.Fatalf("an escort should ignore a far enemy and regroup at its slot, got (%.1f,%.1f)", a.x, a.y)
	}
}

// TestEscortFacesTravelDirection is the fix for "the escort doesn't point where it flies":
// with nothing to fight it heads back to its slot, facing its travel direction, not the
// ship's heading.
func TestEscortFacesTravelDirection(t *testing.T) {
	g := &Game{}
	g.addAlly(modeEscort)
	a := &g.allies[0]
	a.x, a.y = 300, 0 // off to the side, no enemy: it flies back toward the regroup slot
	sx, sy := g.escortSlot(0)
	want := math.Atan2(sy-a.y, sx-a.x) * 180 / math.Pi
	g.stepAllies()
	if d := math.Abs(normDeg(a.angle - want)); d > 1.0 {
		t.Fatalf("escort should face its travel direction %.1f, got %.1f", want, a.angle)
	}
}

// TestEscortWaypointRoutesAroundWall: when the straight line to its goal is blocked, the
// escort's next A* waypoint detours around the wall — a reachable point off the direct
// line — instead of a straight shot that would clip through the wall.
func TestEscortWaypointRoutesAroundWall(t *testing.T) {
	g := &Game{}
	g.bounds = bounds{minX: -200, minY: -200, maxX: 400, maxY: 400}
	g.segs = []segment{{ax: 100, ay: -60, bx: 100, by: 60}} // vertical wall, gaps above/below
	g.nav = buildNavgrid(g.segs, g.bounds)
	a := &ally{x: 0, y: 0}
	gx, gy := 200.0, 0.0 // straight across the wall
	if g.clearPath(a.x, a.y, gx, gy, 0) {
		t.Fatal("test setup: the direct line to the goal should be blocked by the wall")
	}
	wx, wy := g.allyWaypoint(a, gx, gy)
	if !g.clearPath(a.x, a.y, wx, wy, 0) {
		t.Fatalf("the escort's A* waypoint (%.0f,%.0f) should be reachable in a clear line", wx, wy)
	}
	if math.Hypot(wx-gx, wy-gy) < 1 {
		t.Fatal("the waypoint should detour, not head straight at the blocked goal")
	}
}

func TestAllyFiresOnlyAtEnemyInRange(t *testing.T) {
	g := &Game{}
	g.entities = []entity{{kind: kindEnemy, x: 100, y: 0, radius: 10}}
	g.allies = []ally{{mode: modeEscort, x: 0, y: 0}}

	g.allyFire(&g.allies[0])
	if len(g.projectiles) != 1 {
		t.Fatalf("an ally should fire at an enemy in range, got %d shots", len(g.projectiles))
	}
	if g.projectiles[0].vx <= 0 {
		t.Fatal("the ally shot should travel toward the enemy (+x)")
	}

	// Out of range: no shot (and the cooldown from the first shot is not the reason).
	g.projectiles = nil
	g.allies[0].fireCD = 0
	g.entities[0].x = allyRange + 200
	g.allyFire(&g.allies[0])
	if len(g.projectiles) != 0 {
		t.Fatal("an ally should not fire at an enemy beyond its range")
	}
}

func TestSnapAlliesToShip(t *testing.T) {
	g := &Game{}
	g.allies = []ally{{mode: modeEscort, x: 999, y: -999}, {mode: modeOrbit, x: -50, y: 50}}
	g.x, g.y = 300, 400
	g.snapAlliesToShip()
	for i := range g.allies {
		if g.allies[i].x != 300 || g.allies[i].y != 400 {
			t.Fatalf("ally %d should snap to the ship, got (%.0f,%.0f)", i, g.allies[i].x, g.allies[i].y)
		}
	}
}

// TestAllyTakesHitsThenDies: a companion is no longer immortal — it absorbs a few enemy shots,
// then is destroyed.
func TestAllyTakesHitsThenDies(t *testing.T) {
	g := &Game{}
	g.allies = []ally{{mode: modeEscort, x: 0, y: 0, hits: allyHits}}

	for h := range allyHits - 1 {
		if !g.enemyShotHitsAlly(-5, 0, 5, 0) { // a shot crossing the escort
			t.Fatalf("hit %d: a shot through the escort should be absorbed", h)
		}
	}
	if len(g.allies) != 1 {
		t.Fatalf("the escort should survive its first hits, got %d allies", len(g.allies))
	}
	if !g.enemyShotHitsAlly(-5, 0, 5, 0) {
		t.Fatal("the finishing shot should still register on the escort")
	}
	if len(g.allies) != 0 {
		t.Fatal("the escort should be destroyed once its hits are spent")
	}
}
