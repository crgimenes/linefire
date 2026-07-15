package game

import (
	"testing"

	"linefire/asset"
	"linefire/filoio"
	"linefire/level"
)

// hordeGame is an open (wall-free) map with the given horde config, so every ring
// position is a clear spawn spot.
func hordeGame(cfg level.Horde) *Game {
	lvl := level.New()
	lvl.Horde = &cfg
	g := New(asset.New(), lvl, "", false)
	g.mapDir = "../gameassets" // resolve enemy assets from the real dir
	// A generous, wall-free world box so every ring position is a clear spawn spot;
	// nav is nil so the reachability check falls back to the bounds test (this
	// exercises the spawner's timing/density/ramp, not the pathfinder).
	g.bounds = bounds{minX: -5000, minY: -5000, maxX: 5000, maxY: 5000}
	g.segs = nil
	g.nav = nil
	g.x, g.y = 0, 0
	return g
}

func run(g *Game, frames int) {
	for range frames {
		g.stepHorde()
	}
}

func TestHordeSpawnsDeterministicallyByTime(t *testing.T) {
	cfg := level.Horde{Types: []string{"enemy"}, Interval: 20, MaxAlive: 100, Ramp: 100000, Seed: 7}

	a := hordeGame(cfg)
	run(a, 205) // one spawn at t=0, then every 20 frames: ~11 spawns
	nA := a.enemiesLeft()

	b := hordeGame(cfg)
	run(b, 205)
	if a.enemiesLeft() != b.enemiesLeft() {
		t.Fatalf("same seed+time must spawn the same count, got %d vs %d", nA, b.enemiesLeft())
	}
	if nA < 8 || nA > 12 {
		t.Fatalf("interval 20 over 205 frames should be ~11 spawns, got %d", nA)
	}
}

func TestHordeHoldsAtDensityCap(t *testing.T) {
	g := hordeGame(level.Horde{Types: []string{"enemy"}, Interval: 5, MaxAlive: 6, Ramp: 100000, Seed: 1})
	run(g, 600) // long enough to hit the cap and keep pressing
	if g.enemiesLeft() != 6 {
		t.Fatalf("the live count must hold at MaxAlive=6, got %d", g.enemiesLeft())
	}
}

func TestHordeRampTightensInterval(t *testing.T) {
	h := newHorde(level.Horde{Types: []string{"enemy"}, Interval: 45, Ramp: 100, Seed: 1})
	start := h.interval
	g := &Game{horde: h, bounds: bounds{minX: -1, minY: -1, maxX: 1, maxY: 1}}
	// Walls everywhere would block spawns; here bounds are tiny so spawns fail and
	// only the ramp is exercised. Advance several ramp steps.
	for range 350 {
		g.stepHorde()
	}
	if h.interval >= start {
		t.Fatalf("the interval should tighten as difficulty ramps: %d -> %d", start, h.interval)
	}
	if h.interval < hordeMinInterval {
		t.Fatalf("the interval must not drop below the floor %d, got %d", hordeMinInterval, h.interval)
	}
}

func TestHordeReachableRejectsOutsideTheMap(t *testing.T) {
	g := &Game{bounds: bounds{minX: 0, minY: 0, maxX: 100, maxY: 100}}
	if g.hordeReachable(500, 500) {
		t.Fatal("a point outside the map bounds must be rejected (enemies dripping off-map)")
	}
	if !g.hordeReachable(50, 50) {
		t.Fatal("an in-bounds clear point (no walls/nav) should be accepted")
	}
	g.segs = []segment{{ax: 40, ay: 40, bx: 60, by: 60}}
	if g.hordeReachable(50, 50) {
		t.Fatal("a point on a wall must be rejected")
	}
}

func TestHordeMap0002SpawnsInsideAndReachable(t *testing.T) {
	lvl, err := filoio.LoadLevel(filoio.LevelPath("../gameassets", "map0002"))
	if err != nil {
		t.Skipf("map0002 not loadable: %v", err)
	}
	if lvl.Horde == nil {
		t.Skip("map0002 has no horde")
	}
	player, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "player"))
	if err != nil {
		t.Skipf("player asset not loadable: %v", err)
	}
	g := New(player, lvl, "", false)
	g.mapDir = "../gameassets"
	g.x, g.y = lvl.PlayerStart.X, lvl.PlayerStart.Y
	g.sw, g.sh, g.dpr = 1600, 1200, 2 // a large screen (the reported case)

	for range 3000 {
		g.stepHorde()
	}

	spawned := 0
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy {
			continue
		}
		spawned++
		if !g.inBounds(e.x, e.y) {
			t.Fatalf("horde enemy spawned OUTSIDE the map at (%.0f,%.0f)", e.x, e.y)
		}
		if g.nav != nil && g.nav.findPath(e.x, e.y, g.x, g.y) == nil {
			t.Fatalf("horde enemy at (%.0f,%.0f) cannot reach the player", e.x, e.y)
		}
	}
	if spawned == 0 {
		t.Fatal("the map0002 horde produced no enemies in 3000 frames (map too tight?)")
	}
}

func TestNewHordeDisabledWithoutTypes(t *testing.T) {
	if newHorde(level.Horde{}) != nil {
		t.Fatal("a horde with no types is disabled (nil)")
	}
}

func TestHordeValidation(t *testing.T) {
	lvl := level.New()
	lvl.Horde = &level.Horde{Types: []string{"enemy", ""}}
	if level.Validate(lvl) == nil {
		t.Fatal("an empty type name should fail validation")
	}
	lvl.Horde = &level.Horde{Types: []string{"enemy"}, Interval: -1}
	if level.Validate(lvl) == nil {
		t.Fatal("a negative interval should fail validation")
	}
	lvl.Horde = &level.Horde{Types: []string{"enemy", "tank"}, Interval: 30, MaxAlive: 20, Ramp: 300}
	if err := level.Validate(lvl); err != nil {
		t.Fatalf("a valid horde was rejected: %v", err)
	}
}
