package game

import (
	"strings"
	"testing"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/level"
)

// TestFinaleEnemiesAreNotStuckInWalls guards a real authoring hazard: an enemy spawned
// inside solid rock (e.g. the finale's central pillar) can never move — it just sits
// there, only killable because fire now digs through walls. This walks the shipped
// finale and fails if any enemy's spawn point is rock.
func TestFinaleEnemiesAreNotStuckInWalls(t *testing.T) {
	player, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "player"))
	if err != nil {
		t.Skip(err)
	}
	lvl, err := filoio.LoadLevel(filoio.LevelPath("../gameassets", "map0003"))
	if err != nil {
		t.Fatalf("finale: %v", err)
	}
	g := New(player, lvl, "../gameassets", false)
	if g.flood == nil {
		t.Fatal("the finale should build a flood map (it is a hand-made level)")
	}
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy {
			continue
		}
		if g.flood.rockAt(e.x, e.y) {
			t.Fatalf("enemy at (%.0f,%.0f) spawned inside a wall — it would be stuck", e.x, e.y)
		}
	}
}

// winLevel is a boxLevel with an enemy and a (resolution "cleared" "win"): the smallest
// map that ends the campaign when its last enemy dies.
func winLevel() *level.Level {
	lvl := boxLevel(400, 400, nil)
	lvl.Name = "finale"
	lvl.PlayerStart = level.Start{X: 200, Y: 200, Angle: -90}
	lvl.Spawns = []level.Spawn{{Name: "e", Asset: "enemy", Kind: "enemy", X: 300, Y: 200}}
	lvl.Resolutions = []level.Resolution{{On: level.OnCleared, Do: level.DoWin}}
	return lvl
}

// TestClearingFinaleWinsTheGame is the whole point of the ending: the last enemy of a
// map marked (resolution "cleared" "win") drops, and the run reaches its terminal
// victory state — not the per-level results overlay.
func TestClearingFinaleWinsTheGame(t *testing.T) {
	g := New(asset.New(), winLevel(), "", false)
	g.mapName = "finale"

	if g.gameWon {
		t.Fatal("a fresh finale is not already won")
	}
	killAllEnemies(g)
	if g.updateResolutions() != true {
		t.Fatal("clearing the finale must fire a resolution that freezes the frame")
	}
	if g.endKind != endWin {
		t.Fatalf("clearing the finale should queue the victory screen behind the delay, endKind=%d", g.endKind)
	}
	if g.gameWon {
		t.Fatal("the victory screen must wait for the final effects to play, not latch instantly")
	}
	if !feedContains(g, "VICTORY") {
		t.Fatalf("the win should announce itself immediately, got %+v", g.log)
	}
	g.endTicks = 1 // fast-forward the aftermath
	_ = g.stepEnding()
	if !g.gameWon {
		t.Fatal("after the delay the victory screen should latch")
	}
	if g.clearBannerTicks != 0 {
		t.Fatal("winning the game is terminal, not a per-level clear banner")
	}
}

// TestWinFreezesTheClock: the victory screen owns input, so Update must not advance the
// sim (the RTA clock included) while it is up.
func TestWinFreezesTheClock(t *testing.T) {
	g := New(asset.New(), winLevel(), "", false)
	g.mapName = "finale"
	g.winGame()

	before := g.runTicks
	for range 10 {
		_ = g.Update()
	}
	if g.runTicks != before {
		t.Fatalf("the clock must not tick behind the victory screen: %d -> %d", before, g.runTicks)
	}
}

// TestRestartCampaignReturnsToStart: "play again" reloads the first map, not the finale
// the player just beat. It falls back to the current level when there is no start map.
func TestRestartCampaignReturnsToStart(t *testing.T) {
	src := portalLevel("src", "dst", 300, 200)
	dir := saveLevels(t, src, portalLevel("dst", "src", 300, 200))

	g := New(asset.New(), src, dir, false)
	g.mapName, g.startMap = "src", "src"

	// Pretend we warped onward and are watching the credits there (the state R exits from).
	g.mapName = "dst"
	g.gameWon = true
	g.creditsMode = true
	g.restartCampaign()

	if g.gameWon || g.creditsMode {
		t.Fatal("play again must drop the victory and credits states")
	}
	if g.mapName != "src" {
		t.Fatalf("play again must return to the start map, got %q", g.mapName)
	}
}

// TestRunLogSummarizesEachStage: playing the whole campaign leaves one summary row per
// stage, in order, titled by its homage — the per-screen breakdown the victory screen shows.
func TestRunLogSummarizesEachStage(t *testing.T) {
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

	// Walk the actual portal chain to the finale, recording each stage's title in order — so
	// inserting or reordering stages does not need this test rewritten.
	stages := []string{g.level.Title}
	visited := map[string]bool{"map0001": true}
	for step := 0; step < 30 && !hasWinResolution(g.level); step++ {
		g.runTicks += 100 // spend some time on this stage
		if !advanceCampaign(g, visited) {
			break
		}
		stages = append(stages, g.level.Title)
	}
	killAllEnemies(g)
	g.updateObjectives()  // the real loop marks objectives before resolutions fire
	g.updateResolutions() // clears the finale -> winGame -> logs the finale

	if len(g.runLog) != len(stages) {
		t.Fatalf("want a row per stage (%d), got %d: %+v", len(stages), len(g.runLog), g.runLog)
	}
	for i := range stages {
		if g.runLog[i].title != stages[i] {
			t.Fatalf("stage %d title = %q, want %q", i, g.runLog[i].title, stages[i])
		}
	}
	if stages[0] != "Star Raiders" || stages[len(stages)-1] != "Sinistar's Lair" {
		t.Fatalf("campaign should run from Star Raiders to Sinistar's Lair, got %v", stages)
	}
	// The finale row shows its single clear objective as met.
	last := g.runLog[len(g.runLog)-1]
	if last.objTotal != 1 || last.objDone != 1 {
		t.Fatalf("finale should log 1/1 objectives, got %d/%d", last.objDone, last.objTotal)
	}
}

// advanceCampaign flies g into a portal leading to an unvisited map and resolves it,
// returning whether it moved on to a new stage (following the forward chain, not back/self
// portals, which target already-visited maps).
func advanceCampaign(g *Game, visited map[string]bool) bool {
	takePortal := func() bool {
		for i := range g.entities {
			e := &g.entities[i]
			if e.kind != kindPortal {
				continue
			}
			dst, _ := parsePortalTarget(e.target)
			if dst == "" || strings.HasPrefix(dst, "@") || visited[dst] {
				continue // stay on the authored chain; skip runtime targets (@bonus/@proc/@rift)
			}
			g.x, g.y = e.x, e.y
			g.portalGrace = 0
			g.resolvePortals()
			visited[g.mapName] = true
			return true
		}
		return false
	}
	if takePortal() {
		return true
	}
	// No open portal: a boss arena reveals its exit only on clear. Clear the room to fire its
	// DoPortal resolution, then take the portal it opens.
	killAllEnemies(g)
	g.updateObjectives()
	g.updateResolutions()
	return takePortal()
}
