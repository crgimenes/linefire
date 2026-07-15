package game

import (
	"testing"

	"linefire/filoio"
)

// TestEnterBonusIsPlayable: taking a bonus portal generates a cave the player can actually
// play — the flood map fills the cavern, the entry and every loot/guard/exit spawn sit in open
// space (not rock), and the exit portal returns to the source map.
func TestEnterBonusIsPlayable(t *testing.T) {
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

	g.enterBonus("map0001:from_bonus")

	if g.mapName != bonusMapName {
		t.Fatalf("should be in the bonus room, got %q", g.mapName)
	}
	if g.flood == nil {
		t.Fatal("bonus room should have a flood map")
	}
	if g.flood.rockAt(g.x, g.y) {
		t.Fatalf("player entry (%.0f,%.0f) is in rock — the cavern did not open", g.x, g.y)
	}

	loot, guards, portals, stuck := 0, 0, 0, 0
	for i := range g.entities {
		e := &g.entities[i]
		switch e.kind {
		case kindEnemy:
			guards++
		case kindPowerUp, kindWeapon:
			loot++
		case kindPortal:
			portals++
			if e.target != "map0001:from_bonus" {
				t.Fatalf("exit portal target = %q, want the return", e.target)
			}
		}
		switch e.kind {
		case kindEnemy, kindPowerUp, kindWeapon, kindPortal:
			if g.flood.rockAt(e.x, e.y) {
				t.Errorf("%q at (%.0f,%.0f) is inside rock (unreachable)", e.power, e.x, e.y)
				stuck++
			}
		}
	}
	if loot < 5 {
		t.Fatalf("bonus should carry loot, got %d", loot)
	}
	if guards < 3 {
		t.Fatalf("bonus should have guards, got %d", guards)
	}
	if portals != 1 {
		t.Fatalf("bonus should have exactly one exit portal, got %d", portals)
	}
	if stuck > 0 {
		t.Fatalf("%d bonus entities are stuck in rock", stuck)
	}
	t.Logf("bonus room: %d loot, %d guards, exit ok; flood valid, nothing stuck", loot, guards)
}

// TestBonusExitReturnsToSource: flying into the bonus room's exit portal warps back to the
// source map's from_bonus entry, so the detour rejoins the campaign.
func TestBonusExitReturnsToSource(t *testing.T) {
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

	g.enterBonus("map0001:from_bonus")
	if g.mapName != bonusMapName {
		t.Fatalf("should be in the bonus room, got %q", g.mapName)
	}

	// Fly onto the exit portal and resolve it.
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind == kindPortal {
			g.x, g.y = e.x, e.y
			g.portalGrace = 0
			g.resolvePortals()
			break
		}
	}
	if g.mapName != "map0001" {
		t.Fatalf("the bonus exit should return to the source map, got %q", g.mapName)
	}
}
