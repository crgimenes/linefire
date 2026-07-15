package game

import (
	"strings"
	"testing"

	"linefire/filoio"
	"linefire/procgen"
)

// TestEnterRiftStartsOneWayChain: taking the boss's portal drops the run into "The Rift" — a
// procedural screen that latches endless mode, deepens the difficulty, and (unlike an edge room)
// has NO return portal, only a forward portal that opens on clear.
func TestEnterRiftStartsOneWayChain(t *testing.T) {
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

	g.enterRift()

	if !g.endless {
		t.Fatal("entering the Rift must latch endless mode (one-way)")
	}
	if g.digDepth != 1 {
		t.Fatalf("the first Rift screen is one level deep, got %d", g.digDepth)
	}
	if g.mapName != "@rift1" {
		t.Fatalf("should be in the first Rift screen, got %q", g.mapName)
	}
	if g.flood == nil {
		t.Fatal("a Rift screen is a destructible-rock cave — its flood field must be built")
	}
	for i := range g.entities {
		if g.entities[i].kind == kindPortal {
			t.Fatalf("a Rift screen must have NO portal until cleared, found target %q", g.entities[i].target)
		}
	}
	fwd := false
	for _, r := range g.level.Resolutions {
		if r.Do == "portal" && r.Target == riftMapName {
			fwd = true
		}
	}
	if !fwd {
		t.Fatal("a Rift screen should reveal a forward portal to the next Rift when cleared")
	}
}

// TestRiftDigOutAdvancesToNextRift: a Rift room whose forward portal never opened (uncleared) is
// never a dead end — digging out advances to the NEXT Rift, still one-way (no return portal).
func TestRiftDigOutAdvancesToNextRift(t *testing.T) {
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

	g.enterRift()
	if g.mapName != "@rift1" {
		t.Fatalf("expected to be in @rift1, got %q", g.mapName)
	}

	g.advancePastEdge(0) // dig out of the (uncleared) Rift room
	if !g.endless {
		t.Fatal("digging out of a Rift must stay in endless mode")
	}
	if g.mapName != "@rift2" {
		t.Fatalf("digging out of a Rift should open the NEXT Rift, got %q", g.mapName)
	}
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind == kindPortal && !strings.HasPrefix(e.target, "@") {
			t.Fatalf("a Rift screen must stay one-way, found a return portal to %q", e.target)
		}
	}
}

// TestBossPortalOpensIntoRift: clearing the finale (escorts then the shielded boss) opens a portal
// into the Rift and begins the victory sequence; flying into that portal falls one-way into "@rift1".
func TestBossPortalOpensIntoRift(t *testing.T) {
	player, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "player"))
	if err != nil {
		t.Skip(err)
	}
	lvl, err := filoio.LoadLevel(filoio.LevelPath("../gameassets", "map0003"))
	if err != nil {
		t.Fatal(err)
	}
	g := New(player, lvl, "../gameassets", false)
	g.mapName, g.startMap = "map0003", "map0003"

	killAllEnemies(g)     // escorts first (the shield drops), then the boss
	g.updateResolutions() // OnCleared -> open the Rift portal, then win

	if g.endKind != endWin {
		t.Fatalf("clearing the finale should begin the victory sequence, endKind=%d", g.endKind)
	}
	var portal *entity
	for i := range g.entities {
		if g.entities[i].kind == kindPortal && g.entities[i].target == riftMapName {
			portal = &g.entities[i]
		}
	}
	if portal == nil {
		t.Fatal("killing the boss should open a portal into the Rift")
	}

	g.resumeAfterWin() // the player chooses to keep going
	g.x, g.y = portal.x, portal.y
	g.portalGrace = 0
	g.resolvePortals()
	if !g.endless || g.mapName != "@rift1" {
		t.Fatalf("taking the boss portal should enter the one-way Rift, endless=%v map=%q", g.endless, g.mapName)
	}
}

// TestFinaleTrapSealsReturnAndSwarms: the finale starts with a return portal; killing the boss
// seals it (only the Rift portal remains), and choosing to continue arms the endless swarm.
func TestFinaleTrapSealsReturnAndSwarms(t *testing.T) {
	player, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "player"))
	if err != nil {
		t.Skip(err)
	}
	lvl, err := filoio.LoadLevel(filoio.LevelPath("../gameassets", "map0003"))
	if err != nil {
		t.Fatal(err)
	}
	g := New(player, lvl, "../gameassets", false)
	g.mapName, g.startMap = "map0003", "map0003"

	back := 0
	for i := range g.entities {
		if g.entities[i].kind == kindPortal && g.entities[i].target == "map0016" {
			back++
		}
	}
	if back != 1 {
		t.Fatalf("the finale should start with a return portal, got %d", back)
	}

	killAllEnemies(g)     // escorts, then the boss
	g.updateResolutions() // rift portal opens, then win seals the return

	rift, back := 0, 0
	for i := range g.entities {
		if g.entities[i].kind != kindPortal {
			continue
		}
		switch g.entities[i].target {
		case riftMapName:
			rift++
		case "map0016":
			back++
		}
	}
	if rift != 1 {
		t.Fatalf("the rift portal must survive the seal, got %d", rift)
	}
	if back != 0 {
		t.Fatalf("the return portal must be sealed when the boss dies, got %d", back)
	}

	if g.horde != nil {
		t.Fatal("the swarm must not start before the player continues")
	}
	g.resumeAfterWin()
	if g.gameWon {
		t.Fatal("continuing must leave the victory screen")
	}
	if g.horde == nil {
		t.Fatal("continuing should arm the endless swarm")
	}
}

// TestRiftRoomsNeverBuryGuards: a guard buried in rock is unkillable but still counts in
// enemiesLeft(), so a Rift room's cleared-> exit portal would never open — a one-way dead end.
// freeBuriedSpawns (run in New) must leave no guard buried, across many generated rooms.
func TestRiftRoomsNeverBuryGuards(t *testing.T) {
	player, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "player"))
	if err != nil {
		t.Skip(err)
	}
	for s := range 40 {
		seed := int64(s*2654435761 + 999) // #nosec G115 -- test seed spread, not crypto
		lvl := procgen.GenRiftRoom(seed, s%5, riftMapName)
		g := New(player, lvl, "../gameassets", false)
		if g.flood == nil {
			t.Fatalf("seed %d: a rift room must build a flood field", s)
		}
		for i := range g.entities {
			e := &g.entities[i]
			if e.kind == kindEnemy && g.flood.rockAt(e.x, e.y) {
				t.Fatalf("seed %d: guard at (%.0f,%.0f) is buried in rock — the room could never be cleared", s, e.x, e.y)
			}
		}
	}
}

// TestSpawnedPortalCarvesItselfClear: a portal opened by a resolution must never stay buried in
// rock (a clipped exit cell) — it digs a clearing so it is visible and reachable.
func TestSpawnedPortalCarvesItselfClear(t *testing.T) {
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
	g.enterRift() // a Rift room: destructible-rock cave with a flood field
	if g.flood == nil {
		t.Fatal("a Rift room must build a flood field")
	}

	// Find a point that is currently solid rock, and open a portal right there.
	var rx, ry float64
	found := false
	b := g.bounds
	for yy := b.minY; yy < b.maxY && !found; yy += 24 {
		for xx := b.minX; xx < b.maxX && !found; xx += 24 {
			if g.flood.rockAt(xx, yy) {
				rx, ry, found = xx, yy, true
			}
		}
	}
	if !found {
		t.Skip("this Rift room has no interior rock to test against")
	}

	g.spawnPortalEntity(riftMapName, rx, ry)
	if g.flood.rockAt(rx, ry) {
		t.Fatalf("a materializing portal must carve itself clear, still rock at (%.0f,%.0f)", rx, ry)
	}
}

// TestEndlessEvictsEveryOtherMap: mid-campaign only procedural screens are freed; once in the Rift
// (endless) every map but the current one is released, since the run can never go back.
func TestEndlessEvictsEveryOtherMap(t *testing.T) {
	g := &Game{mapStates: map[string]*mapState{
		"@edge1":  {},
		"map0001": {}, // authored
		"@rift2":  {}, // current
	}}

	g.endless = false
	g.evictStaleProcMaps("@rift2")
	if _, ok := g.mapStates["map0001"]; !ok {
		t.Fatal("mid-campaign eviction must keep authored maps")
	}
	if _, ok := g.mapStates["@edge1"]; ok {
		t.Fatal("mid-campaign eviction should drop stale procedural screens")
	}

	g.mapStates["map0002"] = &mapState{}
	g.endless = true
	g.evictStaleProcMaps("@rift2")
	if len(g.mapStates) != 1 {
		t.Fatalf("endless eviction should leave only the current map, got %d", len(g.mapStates))
	}
	if _, ok := g.mapStates["@rift2"]; !ok {
		t.Fatal("endless eviction must keep the current map")
	}
}
