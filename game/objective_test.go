package game

import (
	"testing"

	"linefire/asset"
	"linefire/level"
)

func TestZoneContains(t *testing.T) {
	rect := level.Zone{Kind: level.ZoneRect, Points: []asset.Point{{X: 0, Y: 0}, {X: 10, Y: 20}}}
	if !zoneContains(rect, 5, 5) {
		t.Error("point inside the rect should be contained")
	}
	if zoneContains(rect, 15, 5) || zoneContains(rect, 5, 25) {
		t.Error("points outside the rect should not be contained")
	}
	rev := level.Zone{Kind: level.ZoneRect, Points: []asset.Point{{X: 10, Y: 20}, {X: 0, Y: 0}}}
	if !zoneContains(rev, 5, 5) {
		t.Error("reversed corners should still contain")
	}

	circ := level.Zone{Kind: level.ZoneCircle, Points: []asset.Point{{X: 0, Y: 0}}, Radius: 10}
	if !zoneContains(circ, 6, 8) { // distance exactly 10 (edge inclusive)
		t.Error("point on the circle edge should be contained")
	}
	if zoneContains(circ, 8, 8) { // distance ~11.3
		t.Error("point outside the circle should not be contained")
	}

	bad := level.Zone{Kind: level.ZoneRect, Points: []asset.Point{{}}}
	if zoneContains(bad, 0, 0) {
		t.Error("malformed zone should never contain")
	}
}

func TestDeriveObjectives(t *testing.T) {
	// A finite-enemy map with an exit zone: the ONLY objective is "clear all enemies".
	// Reaching the exit is not an objective (it leads to another screen).
	lvl := level.New()
	lvl.Zones = []level.Zone{{Name: "exit", Kind: level.ZoneCircle, Points: []asset.Point{{X: 1, Y: 1}}, Radius: 5, Trigger: "exit"}}
	objs := deriveObjectives(lvl, []entity{{kind: kindEnemy}, {kind: kindPowerUp}})
	if len(objs) != 1 || objs[0].kind != objEliminate {
		t.Fatalf("want a single clear-enemies objective, got %+v", objs)
	}

	// A horde map cannot be cleared, so it gets no clear objective — you escape.
	horde := level.New()
	horde.Horde = &level.Horde{Types: []string{"enemy"}}
	if got := deriveObjectives(horde, []entity{{kind: kindEnemy}}); len(got) != 0 {
		t.Fatalf("a horde map must have no clear objective, got %+v", got)
	}

	none := deriveObjectives(level.New(), []entity{{kind: kindPowerUp}})
	if len(none) != 0 {
		t.Fatalf("no enemies should yield no objectives, got %+v", none)
	}
}

func TestObjectivesCompleteFlow(t *testing.T) {
	lvl := level.New()
	lvl.PlayerStart = level.Start{X: 0, Y: 0, Angle: -90}
	lvl.Zones = []level.Zone{{Name: "exit", Kind: level.ZoneCircle, Points: []asset.Point{{X: 100, Y: 100}}, Radius: 20, Trigger: "exit"}}
	lvl.Spawns = []level.Spawn{{Name: "e", Asset: "enemy", Kind: "enemy", X: 50, Y: 50}}

	g := New(asset.New(), lvl, "", false)
	if len(g.objectives) != 1 || g.objectives[0].kind != objEliminate {
		t.Fatalf("a finite-enemy map has one objective, clear all enemies; got %+v", g.objectives)
	}
	if g.objectivesComplete() {
		t.Fatal("enemy alive -> not complete")
	}

	// Reaching the exit zone still fires its trigger (for future use), but it is NOT an
	// objective, so it does not advance completion.
	g.x, g.y = 100, 100
	g.updateObjectives()
	if !g.firedTriggers["exit"] {
		t.Fatal("entering the exit zone should still fire its trigger")
	}
	if g.objectivesComplete() {
		t.Fatal("reaching the exit is not an objective; enemy still alive -> not complete")
	}

	// Clear the enemy: the sole objective is met.
	g.entities = nil
	g.updateObjectives()
	if !g.objectivesComplete() {
		t.Fatal("cleared enemies -> complete")
	}
}
