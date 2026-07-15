package game

import (
	"path/filepath"
	"strings"
	"testing"

	"linefire/filoio"
)

// TestNoShippedMapHasStuckEnemies loads every authored map and fails if any enemy spawn sits inside
// rock — where it could never be reached or would jitter against a wall. It is content-agnostic, so
// every new campaign phase is checked automatically the moment its .lfm is added.
func TestNoShippedMapHasStuckEnemies(t *testing.T) {
	player, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "player"))
	if err != nil {
		t.Skip(err)
	}
	paths, err := filepath.Glob("../gameassets/map*.lfm")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no campaign maps found: %v", err)
	}
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		lvl, err := filoio.LoadLevel(p)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		g := New(player, lvl, "../gameassets", false)
		if g.flood == nil {
			continue // no region field (unexpected for a hand-made map, but nothing to check)
		}
		for i := range g.entities {
			e := &g.entities[i]
			if e.kind != kindEnemy {
				continue
			}
			if g.flood.rockAt(e.x, e.y) {
				t.Errorf("%s: enemy %q at (%.0f,%.0f) spawned inside a wall — it would be stuck", name, e.power, e.x, e.y)
			}
		}
	}
}
