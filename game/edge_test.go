package game

import (
	"testing"

	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/procgen"
)

// TestDiggingPastEdgeDetectsBreach: the detector fires only when the ship reaches the flood grid's
// edge, and reports the side it breached.
func TestDiggingPastEdgeDetectsBreach(t *testing.T) {
	lvl := boxLevel(400, 400, nil)
	segs := wallSegments(lvl)
	b := mapBounds(lvl, segs)
	f := buildFloodmap(segs, 200, 200, b)
	if f == nil {
		t.Fatal("expected a floodmap")
	}
	g := &Game{flood: f, bounds: b}

	g.x, g.y = (b.minX+b.maxX)/2, (b.minY+b.maxY)/2 // inside the authored bounds
	if _, ok := g.diggingPastEdge(); ok {
		t.Fatal("a ship inside the map bounds has not dug past any edge")
	}

	g.x = b.maxX + edgeDigDepth + 10 // dug well past the right bound
	g.y = (b.minY + b.maxY) / 2
	side, ok := g.diggingPastEdge()
	if !ok || side != procgen.EdgeRight {
		t.Fatalf("a ship dug past the right bound should breach EdgeRight, got side=%d ok=%v", side, ok)
	}
}

// TestExtendPastEdgeEntersHarderRoom: digging past the edge warps into a fresh destructible-rock
// screen, one level deeper and enemy-dense.
func TestExtendPastEdgeEntersHarderRoom(t *testing.T) {
	player, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "player"))
	if err != nil {
		t.Skip(err)
	}
	lvl, err := filoio.LoadLevel(filoio.LevelPath("../gameassets", "map0001"))
	if err != nil {
		t.Fatal(err)
	}
	g := New(player, lvl, "../gameassets", false)
	g.mapName, g.startMap = "map0001", "map0001"

	g.extendPastEdge(procgen.EdgeRight)

	if g.digDepth != 1 {
		t.Fatalf("digging past the edge should deepen the run, got depth %d", g.digDepth)
	}
	if g.mapName != "@edge1" {
		t.Fatalf("should be in the first edge room, got %q", g.mapName)
	}
	if g.flood == nil {
		t.Fatal("an edge room is a destructible-rock screen — its flood field must be built")
	}
	guards, returns := 0, 0
	for i := range g.entities {
		e := &g.entities[i]
		switch e.kind {
		case kindEnemy:
			guards++
		case kindPortal:
			returns++
			if e.target != "map0001" {
				t.Fatalf("the edge room's return portal should lead back to the authored map, got %q", e.target)
			}
		}
	}
	if guards < 6 {
		t.Fatalf("an edge room should be enemy-dense, got %d guards", guards)
	}
	if returns != 1 {
		t.Fatalf("an edge room needs a return portal to the authored map, got %d", returns)
	}
}

// TestReturnPortalEndsTheDigChain: flying into an edge room's return portal warps back to the
// authored map and resets the dig depth (you are out of the procedural chain).
func TestReturnPortalEndsTheDigChain(t *testing.T) {
	player, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "player"))
	if err != nil {
		t.Skip(err)
	}
	lvl, err := filoio.LoadLevel(filoio.LevelPath("../gameassets", "map0001"))
	if err != nil {
		t.Fatal(err)
	}
	g := New(player, lvl, "../gameassets", false)
	g.mapName, g.startMap = "map0001", "map0001"

	g.extendPastEdge(procgen.EdgeRight)
	if g.digDepth != 1 {
		t.Fatalf("expected to be one level deep, got %d", g.digDepth)
	}

	for i := range g.entities {
		if g.entities[i].kind == kindPortal {
			g.x, g.y = g.entities[i].x, g.entities[i].y
			g.portalGrace = 0
			g.resolvePortals()
			break
		}
	}
	if g.mapName != "map0001" {
		t.Fatalf("the return portal should warp back to the authored map, got %q", g.mapName)
	}
	if g.digDepth != 0 {
		t.Fatalf("returning to an authored map should reset the dig depth, got %d", g.digDepth)
	}
}
