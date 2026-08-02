package game

import (
	"math"
	"testing"

	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/procgen"
)

// mazeGame is a real generated labyrinth with one faction-1 hull in it.
func mazeGame(t *testing.T, seed int64, x, y float64) *Game {
	t.Helper()
	lvl := procgen.GenCaveArena(seed, 2400, 1584)
	player, err := filoio.LoadAssetFS(filoio.OSFS(), "../gameassets", "player")
	if err != nil {
		t.Fatal(err)
	}
	g := newWithContent(filoio.OSFS(), player, lvl, "../gameassets", false)
	g.skirmishMode = true
	g.entities = append(g.entities, entity{
		kind: kindEnemy, id: 1, x: x, y: y, radius: 12,
		hp: 3, hpMax: 3, faction: 1, radar: 400, fireEvery: 60,
	})
	return g
}

// A point inside the rock cannot be reached, and seeking one must NOT park the
// hull against the wall for the rest of the battle. This is the exact seed and
// pair of coordinates from a real maze run where four ships of one fleet were
// found shoulder to shoulder in the same corner, all seeking (200,200) — which
// the generator had filled with rock.
func TestSeekingIntoRockDoesNotParkTheHull(t *testing.T) {
	const tx, ty = 200.0, 200.0
	g := mazeGame(t, 12, 293, 499)
	e := &g.entities[len(g.entities)-1]

	if !g.collidesAt(tx, ty, e.radius) {
		t.Fatalf("this test needs (%v,%v) to be solid rock in seed 12", tx, ty)
	}
	if p := g.nav.findPath(e.x, e.y, tx, ty); len(p) != 0 {
		t.Fatalf("this test needs (%v,%v) to be unreachable, got a %d-waypoint path", tx, ty, len(p))
	}

	start := [2]float64{e.x, e.y}
	far := 0.0
	for range 600 { // ten seconds of pressing
		g.pursuePath(e, tx, ty)
		far = math.Max(far, math.Hypot(e.x-start[0], e.y-start[1]))
	}
	if !e.navBlocked {
		t.Error("the engine could not deliver and did not say so: self-nav-blocked is false")
	}
	if far < 100 {
		t.Errorf("the hull only ever got %.0f units from where it was wedged; it is grinding the wall", far)
	}
}

// The fallback that heads straight is still right where it belongs: a target in
// the open with a clear line stays a straight run, and nothing is flagged.
func TestSeekingAClearTargetStillHeadsStraight(t *testing.T) {
	g := mazeGame(t, 12, 293, 499)
	e := &g.entities[len(g.entities)-1]
	// Probe around the hull for a step with a genuinely clear line, so the rule
	// under test is exercised rather than skipped past.
	var tx, ty float64
	found := false
	for i := range 16 {
		a := float64(i) * math.Pi / 8
		cx, cy := e.x+60*math.Cos(a), e.y+60*math.Sin(a)
		if g.clearPath(e.x, e.y, cx, cy, e.radius) {
			tx, ty, found = cx, cy, true
			break
		}
	}
	if !found {
		t.Fatal("no clear step anywhere around the hull: the fixture is wrong, not the rule")
	}

	before := math.Hypot(tx-e.x, ty-e.y)
	for range 30 {
		g.pursuePath(e, tx, ty)
	}
	if e.navBlocked {
		t.Error("a reachable target was reported as blocked")
	}
	if after := math.Hypot(tx-e.x, ty-e.y); after >= before {
		t.Errorf("the hull did not close on a clear target: %.0f -> %.0f", before, after)
	}
}
