package game

import (
	"testing"

	"linefire/asset"
	"linefire/filoio"
	"linefire/level"
)

// TestPortalEntersTargetMap drives a portal end to end: a source map with a
// portal targeting a second map, the player touching it, and the run carrying
// over (health/score kept, player placed at the new map's start).
func TestPortalEntersTargetMap(t *testing.T) {
	dir := t.TempDir()

	dst := level.New()
	dst.Name = "dst"
	dst.PlayerStart = level.Start{X: 10, Y: 20, Angle: -90}
	err := filoio.SaveLevel(filoio.LevelPath(dir, "dst"), dst)
	if err != nil {
		t.Fatalf("save dst: %v", err)
	}

	src := level.New()
	src.Name = "src"
	src.PlayerStart = level.Start{X: 100, Y: 100, Angle: -90}
	src.Spawns = []level.Spawn{
		{Name: "p", Asset: "portal", Kind: "portal", Target: "dst", X: 100, Y: 100},
	}

	g := New(asset.New(), src, "", false)
	g.mapDir = dir
	g.mapName = "src"
	g.portalGrace = 0 // make the portal live immediately
	g.health, g.score, g.lives = 33, 7, 2

	if !g.resolvePortals() {
		t.Fatal("expected the portal to trigger when overlapped")
	}
	if g.level.Name != "dst" {
		t.Fatalf("did not enter target map: level = %q", g.level.Name)
	}
	if g.x != 10 || g.y != 20 {
		t.Fatalf("player not placed at dst start: (%v,%v)", g.x, g.y)
	}
	if g.health != 33 || g.score != 7 || g.lives != 2 {
		t.Fatalf("run state not carried over: health=%d score=%d lives=%d", g.health, g.score, g.lives)
	}
	if g.mapDir != dir {
		t.Fatalf("mapDir lost across transition: %q", g.mapDir)
	}
}

// TestPortalEntersNamedEntry checks a portal that names an entry drops the player
// at that entry's position and facing, with a fallback to the start when the named
// entry is missing.
func TestPortalEntersNamedEntry(t *testing.T) {
	dir := t.TempDir()

	dst := level.New()
	dst.Name = "dst"
	dst.PlayerStart = level.Start{X: 10, Y: 20, Angle: -90}
	dst.Entries = []level.Entry{{Name: "gate_b", X: 333, Y: 444, Angle: 90}}
	err := filoio.SaveLevel(filoio.LevelPath(dir, "dst"), dst)
	if err != nil {
		t.Fatalf("save dst: %v", err)
	}

	newSrc := func(target string) *Game {
		src := level.New()
		src.Name = "src"
		src.PlayerStart = level.Start{X: 100, Y: 100, Angle: -90}
		src.Spawns = []level.Spawn{
			{Name: "p", Asset: "portal", Kind: "portal", Target: target, X: 100, Y: 100},
		}
		g := New(asset.New(), src, "", false)
		g.mapDir = dir
		g.mapName = "src"
		g.portalGrace = 0
		return g
	}

	g := newSrc("dst:gate_b")
	if !g.resolvePortals() {
		t.Fatal("expected the portal to trigger")
	}
	if g.x != 333 || g.y != 444 || g.angle != 90 {
		t.Fatalf("player not at named entry: (%v,%v)@%v", g.x, g.y, g.angle)
	}

	// An unknown entry name falls back to the map's PlayerStart.
	g = newSrc("dst:does_not_exist")
	if !g.resolvePortals() {
		t.Fatal("expected the portal to trigger")
	}
	if g.x != 10 || g.y != 20 {
		t.Fatalf("unknown entry should fall back to start, got (%v,%v)", g.x, g.y)
	}
}

// TestPortalSameMapTeleport checks a portal whose target names the current map
// teleports the player in place (to the entry) without reloading the map.
func TestPortalSameMapTeleport(t *testing.T) {
	l := level.New()
	l.Name = "here"
	l.PlayerStart = level.Start{X: 100, Y: 100, Angle: -90}
	l.Entries = []level.Entry{{Name: "corner", X: 700, Y: 700, Angle: 45}}
	l.Spawns = []level.Spawn{
		{Name: "p", Asset: "portal", Kind: "portal", Target: "here:corner", X: 100, Y: 100},
	}
	g := New(asset.New(), l, "", false)
	g.mapName = "here"
	g.portalGrace = 0
	lvlBefore := g.level

	if g.resolvePortals() {
		t.Fatal("same-map teleport should not swap the world")
	}
	if g.level != lvlBefore {
		t.Fatal("same-map teleport should not reload the level")
	}
	if g.x != 700 || g.y != 700 || g.angle != 45 {
		t.Fatalf("player not teleported to the entry: (%v,%v)@%v", g.x, g.y, g.angle)
	}
}

// TestPortalGraceBlocksTrigger checks portals stay inert during the post-entry
// grace period so the player does not instantly warp again.
func TestPortalGraceBlocksTrigger(t *testing.T) {
	l := level.New()
	l.PlayerStart = level.Start{X: 0, Y: 0, Angle: -90}
	l.Spawns = []level.Spawn{
		{Name: "p", Asset: "portal", Kind: "portal", Target: "whatever", X: 0, Y: 0},
	}
	g := New(asset.New(), l, "", false)
	g.portalGrace = 3

	if g.resolvePortals() {
		t.Fatal("portal should be inert during the grace period")
	}
	if g.portalGrace != 2 {
		t.Fatalf("grace not decremented: %d", g.portalGrace)
	}
}
