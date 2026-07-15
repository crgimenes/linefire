package game

import (
	"testing"

	"linefire/asset"
	"linefire/filoio"
	"linefire/level"
)

// digSpot is a point well inside the rock of a boxLevel: outside the wall ring, but
// within the padded grid. Digging there is unambiguous — it can only be rock.
const digX, digY = 400.0, 200.0

// rockAtSpot reports whether the dig spot is still solid rock.
func rockAtSpot(g *Game) bool {
	return g.flood.rockAt(digX, digY)
}

// portalLevel is a boxLevel with a portal to target at the given point.
func portalLevel(name, target string, px, py float64) *level.Level {
	lvl := boxLevel(400, 400, nil)
	lvl.Name = name
	lvl.PlayerStart = level.Start{X: 200, Y: 200, Angle: -90}
	lvl.Spawns = []level.Spawn{
		{Name: "to_" + target, Asset: "portal", Kind: "portal", Target: target, X: px, Y: py},
	}
	return lvl
}

// saveLevels writes the two levels into a temp map dir and returns it.
func saveLevels(t *testing.T, src, dst *level.Level) string {
	t.Helper()
	dir := t.TempDir()
	for name, lvl := range map[string]*level.Level{"src": src, "dst": dst} {
		err := filoio.SaveLevel(filoio.LevelPath(dir, name), lvl)
		if err != nil {
			t.Fatalf("save %s: %v", name, err)
		}
	}
	return dir
}

// warp drives the player onto the portal and takes it, asserting the map changed.
func warp(t *testing.T, g *Game, px, py float64, want string) {
	t.Helper()
	g.x, g.y = px, py
	g.portalGrace = 0
	if !g.resolvePortals() || g.mapName != want {
		t.Fatalf("did not enter %q: map=%q", want, g.mapName)
	}
}

// TestDugTerrainSurvivesWarpAndReturn is the whole point of persisting the field: a
// tunnel is a place, not an effect. Leaving a map and coming back must not heal the
// rock the player spent a magazine on.
func TestDugTerrainSurvivesWarpAndReturn(t *testing.T) {
	dir := saveLevels(t, portalLevel("src", "dst", 300, 200), portalLevel("dst", "src", 300, 200))
	src, err := filoio.LoadLevel(filoio.LevelPath(dir, "src"))
	if err != nil {
		t.Fatalf("load src: %v", err)
	}

	g := New(asset.New(), src, "", false)
	g.mapDir, g.mapName = dir, "src"

	if !rockAtSpot(g) {
		t.Fatal("the dig spot must start as solid rock")
	}
	g.digAt(digX, digY, 20)
	if rockAtSpot(g) {
		t.Fatal("digging should have opened the spot")
	}

	warp(t, g, 300, 200, "dst")
	if g.flood.dugAt(digX, digY) {
		t.Fatal("the other map must not inherit the tunnel")
	}
	warp(t, g, 300, 200, "src")

	if rockAtSpot(g) {
		t.Fatal("the tunnel must survive a warp out and back")
	}
	if !g.flood.dugAt(digX, digY) {
		t.Fatal("the returned map should remember the spot as dug, not as authored floor")
	}
	if g.flood.clearanceAt(digX, digY) <= 0 {
		t.Fatal("a restored tunnel must be flyable: clearance stayed at zero")
	}
}

// TestCheckpointFreezesTerrain locks the complete-state rule crg asked for: a restore
// rewinds the world to the checkpoint. Rock dug BEFORE it stays dug; rock dug AFTER it
// grows back, exactly as a pickup taken after it is offered again.
func TestCheckpointFreezesTerrain(t *testing.T) {
	lvl := boxLevel(400, 400, nil)
	lvl.PlayerStart = level.Start{X: 200, Y: 200, Angle: -90}
	lvl.Zones = []level.Zone{{
		Name: "cp", Kind: level.ZoneCircle, Trigger: triggerCheckpoint,
		Points: []asset.Point{{X: 200, Y: 200}}, Radius: 10,
	}}

	g := New(asset.New(), lvl, "", false)
	g.mapName = "src"

	const lateX, lateY = 400.0, 260.0
	g.digAt(digX, digY, 20) // before the checkpoint
	g.saveCheckpoint(lvl.Zones[0])
	g.digAt(lateX, lateY, 20) // after it

	g.restoreCheckpoint()

	if rockAtSpot(g) {
		t.Fatal("a tunnel dug before the checkpoint must survive the restore")
	}
	if !g.flood.rockAt(lateX, lateY) {
		t.Fatal("a tunnel dug after the checkpoint must grow back with the rest of the world")
	}
}

// TestFogSurvivesWarpAndReturn: what the player has explored is knowledge. Flying to
// another map and back must not re-seal the corridors they already lit up.
func TestFogSurvivesWarpAndReturn(t *testing.T) {
	dir := saveLevels(t, portalLevel("src", "dst", 300, 200), portalLevel("dst", "src", 300, 200))
	src, err := filoio.LoadLevel(filoio.LevelPath(dir, "src"))
	if err != nil {
		t.Fatalf("load src: %v", err)
	}

	g := New(asset.New(), src, "", false)
	g.mapDir, g.mapName = dir, "src"

	if g.disc.discoveredAt(200, 200) {
		t.Fatal("a fresh map starts under fog")
	}
	g.updateDiscovery() // the ship sits at the start and lights up what it sees

	seen := 0
	for _, s := range g.disc.seen {
		if s {
			seen++
		}
	}
	if seen == 0 {
		t.Fatal("the ship should have cleared something around itself")
	}

	warp(t, g, 300, 200, "dst")
	if g.disc.discoveredAt(200, 200) {
		t.Fatal("the other map must not inherit the cleared fog")
	}
	warp(t, g, 300, 200, "src")

	if !g.disc.discoveredAt(200, 200) {
		t.Fatal("the fog lifted on this map must survive a warp out and back")
	}
}

// TestFogSurvivesDeath is the counterpart to TestCheckpointFreezesTerrain: rock is world
// state and rewinds, fog is knowledge and does not. Dying must not un-see a corridor.
func TestFogSurvivesDeath(t *testing.T) {
	lvl := boxLevel(400, 400, nil)
	lvl.PlayerStart = level.Start{X: 200, Y: 200, Angle: -90}
	lvl.Zones = []level.Zone{{
		Name: "cp", Kind: level.ZoneCircle, Trigger: triggerCheckpoint,
		Points: []asset.Point{{X: 200, Y: 200}}, Radius: 10,
	}}

	g := New(asset.New(), lvl, "", false)
	g.mapName = "src"
	g.saveCheckpoint(lvl.Zones[0])

	// Explore AFTER the checkpoint: the snapshot must not roll this back.
	g.x, g.y = 100, 100
	g.updateDiscovery()
	if !g.disc.discoveredAt(100, 100) {
		t.Fatal("the ship should have cleared the fog where it flew")
	}

	g.restoreCheckpoint()

	if !g.disc.discoveredAt(100, 100) {
		t.Fatal("a death must not re-seal fog the player already lifted")
	}
}

// TestApplyDugIsIdempotent guards the aliasing invariant: the store hands the field its
// own slice back on every bind, so a bind must be a no-op once the rock has moved.
func TestApplyDugIsIdempotent(t *testing.T) {
	lvl := boxLevel(400, 400, nil)
	g := New(asset.New(), lvl, "", false)
	g.mapName = "src"

	g.digAt(digX, digY, 20)
	g.bindMapState() // registers the alias
	before := g.flood.holes

	if g.flood.applyDug(g.flood.dug) {
		t.Fatal("re-applying the field's own excavation must report no change")
	}
	g.bindMapState()
	if g.flood.holes != before {
		t.Fatalf("binding twice must not carve: holes %d -> %d", before, g.flood.holes)
	}
	if !g.flood.dugAt(digX, digY) {
		t.Fatal("the hole must still be there after two binds")
	}

	// A grid of another shape is ignored rather than trusted.
	if g.flood.applyDug(make([]bool, len(g.flood.dug)+1)) {
		t.Fatal("a mismatched grid must be refused")
	}
}
