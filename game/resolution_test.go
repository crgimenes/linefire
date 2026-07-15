package game

import (
	"testing"

	"linefire/asset"
	"linefire/level"
)

// resolutionLevel is a level with one enemy spawn and the given resolutions.
func resolutionLevel(rs []level.Resolution) *level.Level {
	lvl := level.New()
	lvl.Spawns = []level.Spawn{{Name: "e", Kind: "enemy", X: 100, Y: 100}}
	lvl.Resolutions = rs
	return lvl
}

func TestClearedResolutionSpawnsRewardOnce(t *testing.T) {
	lvl := resolutionLevel([]level.Resolution{
		{On: level.OnCleared, Do: level.DoSpawn, Target: "missing.json", X: 50, Y: 60},
	})
	g := New(asset.New(), lvl, "", false)
	if !g.hadEnemies {
		t.Fatal("the level spawns an enemy: hadEnemies should be true")
	}

	if g.updateResolutions() {
		t.Fatal("no resolution should fire while the enemy lives")
	}
	if len(g.entities) != 1 {
		t.Fatalf("expected just the enemy, got %d entities", len(g.entities))
	}

	g.damageEnemy(0, 999, damageColor) // kill it: the map is now cleared

	if g.updateResolutions() {
		t.Fatal("a spawn routine must not report a world swap")
	}
	var pickups int
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind == kindPowerUp {
			pickups++
			if e.x != 50 || e.y != 60 {
				t.Fatalf("reward at (%v,%v), want (50,60)", e.x, e.y)
			}
			if e.spawn != -1 {
				t.Fatal("a reward is not a level spawn; must not claim a consumed slot")
			}
		}
	}
	if pickups != 1 {
		t.Fatalf("expected 1 reward pickup, got %d", pickups)
	}

	g.updateResolutions() // already resolved: firing again must do nothing
	n := 0
	for i := range g.entities {
		if g.entities[i].kind == kindPowerUp {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("a resolution must fire once, got %d rewards", n)
	}
}

func TestClearedNeedsEnemiesToHaveExisted(t *testing.T) {
	lvl := level.New() // no spawns at all
	lvl.Resolutions = []level.Resolution{
		{On: level.OnCleared, Do: level.DoSpawn, Target: "missing.json"},
	}
	g := New(asset.New(), lvl, "", false)
	g.updateResolutions()
	for i := range g.entities {
		if g.entities[i].kind == kindPowerUp {
			t.Fatal("an enemy-free map must not resolve 'cleared' on arrival")
		}
	}
}

func TestReturnWithoutOriginStaysPut(t *testing.T) {
	lvl := resolutionLevel([]level.Resolution{
		{On: level.OnCleared, Do: level.DoReturn},
	})
	g := New(asset.New(), lvl, "", false)
	g.damageEnemy(0, 999, damageColor)
	if g.updateResolutions() {
		t.Fatal("return with no cameFrom must not swap the world")
	}
	if !g.curMapState().resolved[0] {
		t.Fatal("the resolution should still be spent (outcomes fire once)")
	}
}

func TestExitToSameMapTeleportsInPlace(t *testing.T) {
	lvl := resolutionLevel([]level.Resolution{
		{On: level.OnCleared, Do: level.DoExit, Target: "arena:door"},
	})
	lvl.Entries = []level.Entry{{Name: "door", X: 300, Y: 320, Angle: 0}}
	g := New(asset.New(), lvl, "", false)
	g.mapName = "arena" // the exit targets this same map: teleport, no reload
	g.damageEnemy(0, 999, damageColor)

	if g.updateResolutions() {
		t.Fatal("a same-map exit teleports; it must not report a swap")
	}
	if g.x != 300 || g.y != 320 {
		t.Fatalf("player at (%v,%v), want the door entry (300,320)", g.x, g.y)
	}
	if g.portalGrace <= 0 {
		t.Fatal("arrival should get portal grace")
	}
}

func TestResolutionValidation(t *testing.T) {
	cases := map[string]level.Resolution{
		"unknown condition": {On: "boss-dead", Do: level.DoExit, Target: "m"},
		"unknown routine":   {On: level.OnCleared, Do: "dance"},
		"exit sans target":  {On: level.OnCleared, Do: level.DoExit},
		"spawn sans target": {On: level.OnCleared, Do: level.DoSpawn},
	}
	for name, r := range cases {
		lvl := level.New()
		lvl.Resolutions = []level.Resolution{r}
		if level.Validate(lvl) == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}

	ok := level.New()
	ok.Resolutions = []level.Resolution{
		{On: level.OnCleared, Do: level.DoReturn},
		{On: level.OnCleared, Do: level.DoSpawn, Target: "a.json", X: 1, Y: 2},
	}
	err := level.Validate(ok)
	if err != nil {
		t.Fatalf("valid resolutions rejected: %v", err)
	}
}
