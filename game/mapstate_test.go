package game

import (
	"testing"

	"linefire/asset"
	"linefire/filoio"
	"linefire/level"
)

// TestMapStatePersistsClears clears a map (kill the enemy, take the pickup),
// portals away and back, and checks the cleared spawns do not return while the
// portal does.
func TestMapStatePersistsClears(t *testing.T) {
	dir := t.TempDir()

	dst := level.New()
	dst.Name = "dst"
	dst.PlayerStart = level.Start{X: 10, Y: 10, Angle: -90}
	dst.Spawns = []level.Spawn{
		{Name: "back", Asset: "portal", Kind: "portal", Target: "src", X: 10, Y: 10},
	}
	err := filoio.SaveLevel(filoio.LevelPath(dir, "dst"), dst)
	if err != nil {
		t.Fatalf("save dst: %v", err)
	}

	src := level.New()
	src.Name = "src"
	src.PlayerStart = level.Start{X: 0, Y: 0, Angle: -90}
	src.Spawns = []level.Spawn{
		{Name: "e", Asset: "enemy", Kind: "enemy", X: 30, Y: 0},
		{Name: "h", Asset: "powerup", Kind: "heal", X: 0, Y: 0},
		{Name: "to_dst", Asset: "portal", Kind: "portal", Target: "dst", X: 100, Y: 0},
	}
	err = filoio.SaveLevel(filoio.LevelPath(dir, "src"), src)
	if err != nil {
		t.Fatalf("save src: %v", err)
	}

	g := New(asset.New(), src, "", false)
	g.mapDir = dir
	g.mapName = "src"

	// Collect the pickup (at the start) and kill the enemy.
	g.resolvePickups()
	killAllEnemies(g)
	if countKind(g, kindPowerUp) != 0 || g.enemiesLeft() != 0 {
		t.Fatalf("map not cleared: enemies=%d pickups=%d", g.enemiesLeft(), countKind(g, kindPowerUp))
	}

	// Portal to dst, then straight back to src.
	g.x, g.y = 100, 0
	g.portalGrace = 0
	if !g.resolvePortals() || g.mapName != "dst" {
		t.Fatalf("did not enter dst: map=%q", g.mapName)
	}
	g.x, g.y = 10, 10
	g.portalGrace = 0
	if !g.resolvePortals() || g.mapName != "src" {
		t.Fatalf("did not return to src: map=%q", g.mapName)
	}

	// The cleared enemy and pickup stay gone; only the portal remains.
	if g.enemiesLeft() != 0 {
		t.Fatalf("cleared enemy respawned: %d", g.enemiesLeft())
	}
	if countKind(g, kindPowerUp) != 0 {
		t.Fatalf("collected pickup respawned: %d", countKind(g, kindPowerUp))
	}
	if countKind(g, kindPortal) != 1 {
		t.Fatalf("portal should still be present, got %d", countKind(g, kindPortal))
	}
}

func killAllEnemies(g *Game) {
	for {
		idx := -1
		for i := range g.entities {
			if g.entities[i].kind != kindEnemy {
				continue
			}
			idx = i
			if !g.entities[i].boss {
				break // clear escorts first: a shielded boss is invulnerable until they are gone
			}
		}
		if idx < 0 {
			return
		}
		g.damageEnemy(idx, playerShotDamage, damageColor)
	}
}

func countKind(g *Game, k entityKind) int {
	n := 0
	for i := range g.entities {
		if g.entities[i].kind == k {
			n++
		}
	}
	return n
}
