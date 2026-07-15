package game

import (
	"path/filepath"
	"strings"
	"testing"

	"linefire/filoio"
	"linefire/level"
)

// TestShippedGameAssetsLoadAndValidate guards the files in gameassets/: the
// player and enemy assets must load and validate, and the level must reference
// the enemy asset for every enemy spawn. (Asset paths in the level resolve
// relative to the run directory, so the build/mesh step is exercised at runtime
// rather than here.)
func TestShippedGameAssetsLoadAndValidate(t *testing.T) {
	_, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "player"))
	if err != nil {
		t.Fatalf("player asset: %v", err)
	}
	_, err = filoio.LoadAsset(filoio.AssetPath("../gameassets", "enemy"))
	if err != nil {
		t.Fatalf("enemy asset: %v", err)
	}

	lvl, err := filoio.LoadLevel(filoio.LevelPath("../gameassets", "map0001"))
	if err != nil {
		t.Fatalf("level: %v", err)
	}
	// Content-agnostic: crg edits the stages, so the exact enemy count is his call — the test
	// only guards that the stage HAS enemies and that every referenced asset loads.
	enemies := enemySpawns(lvl)
	if len(enemies) == 0 {
		t.Fatal("map0001 should spawn at least one enemy")
	}
	for i, s := range enemies {
		_, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", s.Asset))
		if err != nil {
			t.Fatalf("enemy %d asset %q: %v", i, s.Asset, err)
		}
	}
}

// enemySpawns returns the level's enemy-category spawns (any archetype Kind).
func enemySpawns(lvl *level.Level) []level.Spawn {
	var out []level.Spawn
	for _, s := range lvl.Spawns {
		if spawnEntityKind(s.Kind) == kindEnemy {
			out = append(out, s)
		}
	}
	return out
}

// TestEnemyArchetypeAssetsLoad guards the archetype assets: each loads, validates,
// declares its own Kind, and has a registered behavior profile.
func TestEnemyArchetypeAssetsLoad(t *testing.T) {
	for _, kind := range []string{"turret", "rusher", "sniper", "tank"} {
		a, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", kind))
		if err != nil {
			t.Fatalf("%s asset: %v", kind, err)
		}
		if a.Kind != kind {
			t.Fatalf("%s asset kind = %q, want %q", kind, a.Kind, kind)
		}
		_, ok := enemyArchetypes[kind]
		if !ok {
			t.Fatalf("no archetype registered for %q", kind)
		}
	}
}

// TestShippedMapIsNavigable guards the level layout: every enemy spawn must be
// reachable from the player start through the navgrid, so pursuit can work.
func TestShippedMapIsNavigable(t *testing.T) {
	lvl, err := filoio.LoadLevel(filoio.LevelPath("../gameassets", "map0001"))
	if err != nil {
		t.Fatalf("level: %v", err)
	}
	segs := wallSegments(lvl)
	nav := buildNavgrid(segs, mapBounds(lvl, segs))
	ps := lvl.PlayerStart
	for i, s := range enemySpawns(lvl) {
		path := nav.findPath(ps.X, ps.Y, s.X, s.Y)
		if len(path) == 0 {
			t.Fatalf("enemy %d at (%v,%v) is unreachable from the player start", i, s.X, s.Y)
		}
	}
}

func TestPowerupAssetLoads(t *testing.T) {
	_, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "powerup"))
	if err != nil {
		t.Fatalf("powerup asset: %v", err)
	}
}

func TestPortalAssetLoads(t *testing.T) {
	for _, name := range []string{"portal", "procportal"} {
		a, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", name))
		if err != nil {
			t.Fatalf("%s asset: %v", name, err)
		}
		if a.Kind != "portal" {
			t.Fatalf("%s asset kind = %q, want portal", name, a.Kind)
		}
	}
}

// TestShippedCampaignConnectedAndWinnable guards the campaign wiring content-agnostically:
// every shipped map*.lfm loads, every portal targets an existing map (and named entry, when
// given), and a winnable finale is reachable from map0001 by portals. Adding or reordering
// stages does not break it as long as the chain stays connected and ends in a win.
func TestShippedCampaignConnectedAndWinnable(t *testing.T) {
	paths, err := filepath.Glob("../gameassets/map*.lfm")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no campaign maps found: %v", err)
	}
	maps := map[string]*level.Level{}
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		lvl, err := filoio.LoadLevel(p)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		maps[name] = lvl
	}

	// Every way onward (a portal spawn OR a DoPortal resolution) must target an existing map and,
	// when named, an existing entry.
	for name, lvl := range maps {
		for _, target := range mapForwardTargets(lvl) {
			dst, label := parsePortalTarget(target)
			if strings.HasPrefix(dst, "@") {
				continue // runtime-generated target (@rift / @bonus / @proc): no file to check
			}
			tm, ok := maps[dst]
			if !ok {
				t.Fatalf("%s leads to unknown map %q", name, dst)
			}
			if label != "" {
				if _, ok := tm.EntryByName(label); !ok {
					t.Fatalf("%s leads to missing entry %q in %s", name, label, dst)
				}
			}
		}
	}

	// A winnable finale (cleared -> win) must be reachable from map0001 by following the ways onward.
	if maps["map0001"] == nil {
		t.Fatal("the campaign must start at map0001")
	}
	seen := map[string]bool{"map0001": true}
	queue := []string{"map0001"}
	winnable := false
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if hasWinResolution(maps[cur]) {
			winnable = true
		}
		for _, target := range mapForwardTargets(maps[cur]) {
			dst, _ := parsePortalTarget(target)
			if maps[dst] == nil || seen[dst] {
				continue
			}
			seen[dst] = true
			queue = append(queue, dst)
		}
	}
	if !winnable {
		t.Fatal("a winnable finale (cleared -> win) must be reachable from map0001")
	}
}

// mapForwardTargets lists every map this level can lead to: portal SPAWNS (always present) and
// DoPortal RESOLUTIONS (a boss arena reveals its exit only on clear). Raw target strings, so the
// caller can still split off an entry label.
func mapForwardTargets(lvl *level.Level) []string {
	var out []string
	for _, s := range lvl.Spawns {
		if s.Kind == "portal" {
			out = append(out, s.Target)
		}
	}
	for _, r := range lvl.Resolutions {
		if r.Do == level.DoPortal {
			out = append(out, r.Target)
		}
	}
	return out
}

// hasWinResolution reports whether clearing the level ends the campaign.
func hasWinResolution(lvl *level.Level) bool {
	for _, r := range lvl.Resolutions {
		if r.On == level.OnCleared && r.Do == level.DoWin {
			return true
		}
	}
	return false
}
