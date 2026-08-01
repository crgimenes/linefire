package game

import (
	"math"
	"testing"

	"github.com/crgimenes/linefire/filoio"
)

// newTestMaze is newTestSkirmish in a labyrinth: with no Layout yet the maze
// comes up as the cave generator's classic square, which is all the test needs.
func newTestMaze(t *testing.T) *Game {
	t.Helper()
	g, err := NewSkirmish(filoio.OSFS(), "../gameassets", SkirmishOptions{Map: SkirmishMapMaze})
	if err != nil {
		t.Fatalf("NewSkirmish: %v", err)
	}
	if g.level.Title != "Labyrinth" {
		t.Fatalf("the skirmish came up in %q, not the maze", g.level.Title)
	}
	return g
}

// A ship must never be delivered wedged into rock: a hull overlapping a wall
// cannot move in any direction and sits dead at the map edge forever (the bug
// crg saw). The spawn clearance has to cover the HULL of what is arriving, not
// a fixed number smaller than the biggest ship.
func TestMazeArrivalsLandClearOfRock(t *testing.T) {
	g := newTestMaze(t)
	for range 200 {
		g.spawnHordeEnemy()
		g.stepArrivals()
	}
	for range 40 { // land everything still in a vortex
		g.stepArrivals()
	}
	if len(g.entities) < 20 {
		t.Fatalf("only %d ships landed; the maze should take far more", len(g.entities))
	}
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy {
			continue
		}
		if g.collidesAt(e.x, e.y, e.radius) {
			t.Fatalf("ship %d (r=%.0f) landed at %.0f,%.0f overlapping rock: it can never move again",
				i, e.radius, e.x, e.y)
		}
	}
}

// unstick's escape direction is sideways relative to the ship's TARGET. It once
// read the player — in skirmish the parked ghost — and a ship wedged in a wall
// nook could be handed two escape directions that were both rock, forever.
func TestUnstickEscapesSidewaysToTheTarget(t *testing.T) {
	g := &Game{}
	g.x, g.y = 0, -500 // the ghost, due north: the OLD bug escaped along east-west
	g.entities = []entity{{kind: kindEnemy, x: 0, y: 0, radius: 5}}
	e := &g.entities[0]

	g.unstick(e, 500, 0) // the real target is due east: escape must be north-south
	if math.Abs(e.vx) > 1e-9 || math.Abs(e.vy) == 0 {
		t.Fatalf("escape velocity (%.2f,%.2f) is not perpendicular to the target direction", e.vx, e.vy)
	}
}
