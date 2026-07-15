package game

import (
	"linefire/asset"
	"linefire/filoio"
	"linefire/level"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestCreditsReturnsToPreviousScreen: opening the credits from a non-playable screen stashes
// that screen so backing out restores it exactly, instead of dropping into a game (the bug
// where Esc from the title's credits started a run).
func TestCreditsReturnsToPreviousScreen(t *testing.T) {
	g := New(asset.New(), level.New(), "", false)
	g.titleMode = true
	g.score = 777 // a marker that must survive the round trip

	g.enterCreditsReturning()
	if g.screenReturn == nil {
		t.Fatal("entering credits should stash the previous screen")
	}
	if !g.creditsMode || g.titleMode {
		t.Fatalf("should be in the credits proper now (creditsMode=%v titleMode=%v)", g.creditsMode, g.titleMode)
	}

	// Back out the way the Esc handler does.
	*g = *g.screenReturn
	if !g.titleMode {
		t.Fatal("backing out of the credits should restore the title screen")
	}
	if g.score != 777 {
		t.Fatalf("the restored screen should keep its state, got score %d", g.score)
	}
	if g.screenReturn != nil {
		t.Fatal("the return chain should be cleared after backing out")
	}
}

// TestFacingWithin: the demo's nose gun gate fires only when the target lies within the
// aim cone of the ship's heading — inside the cone true, past it false, with angle wrap.
func TestFacingWithin(t *testing.T) {
	g := &Game{}

	g.angle = 0 // facing +x
	if !g.facingWithin(1, 0, autoAimConeDeg) {
		t.Fatal("dead-ahead target should be within the cone")
	}
	if g.facingWithin(0, 1, autoAimConeDeg) {
		t.Fatal("a target at 90 degrees should be outside the cone")
	}
	if g.facingWithin(-1, 0, autoAimConeDeg) {
		t.Fatal("a target dead behind should be outside the cone")
	}

	// Wrap-around: heading 350, target at +10 -> 20 apart, inside a 26 cone.
	g.angle = 350
	if !g.facingWithin(0.9848, 0.1736, autoAimConeDeg) { // ~10 degrees
		t.Fatal("wrap-around within-cone should be true")
	}
}

func TestKonamiStepCompletesAndResets(t *testing.T) {
	n := 0
	for _, k := range konamiSeq {
		n = konamiStep(n, k)
	}
	if n != len(konamiSeq) {
		t.Fatalf("the full Konami sequence should complete, got %d/%d", n, len(konamiSeq))
	}
	if konamiStep(3, ebiten.KeyZ) != 0 {
		t.Fatal("a wrong key should reset progress")
	}
	if konamiStep(3, konamiSeq[0]) != 1 {
		t.Fatal("the first key should restart progress to 1")
	}
}

func TestCreditsLinesNonEmpty(t *testing.T) {
	if len(creditsLines()) == 0 {
		t.Fatal("the credits should have content")
	}
}

// TestCreditsDemoNeverFiresResolutions is the fix for "the ship dies in the credits": the
// autonomous demo would clear its map's enemies and trip a (cleared -> win) resolution,
// freezing on the victory screen. Resolutions must not fire while the demo runs.
func TestCreditsDemoNeverFiresResolutions(t *testing.T) {
	lvl := boxLevel(400, 400, nil)
	lvl.PlayerStart = level.Start{X: 200, Y: 200, Angle: -90}
	lvl.Spawns = []level.Spawn{{Name: "e", Asset: "enemy", Kind: "enemy", X: 300, Y: 200}}
	lvl.Resolutions = []level.Resolution{{On: level.OnCleared, Do: level.DoWin}}

	g := New(asset.New(), lvl, "", false)
	g.mapName = "arena"
	g.creditsMode = true // the attract demo is running
	killAllEnemies(g)    // the immortal maxed demo ship clears the map

	if g.updateResolutions() {
		t.Fatal("resolutions must not fire during the credits demo")
	}
	if g.gameWon {
		t.Fatal("clearing the map in the demo must NOT win the game (the frozen-screen bug)")
	}
}

// TestCreditsLoadsDedicatedArena: the demo runs on the credits map, not whatever level
// the player was on, so it never inherits that level's win/exit resolutions.
func TestCreditsLoadsDedicatedArena(t *testing.T) {
	player, err := filoio.LoadAsset(filoio.AssetPath("../gameassets", "player"))
	if err != nil {
		t.Skip(err)
	}
	lvl, err := filoio.LoadLevel(filoio.LevelPath("../gameassets", "map0003")) // the finale, with (cleared -> win)
	if err != nil {
		t.Fatal(err)
	}
	g := New(player, lvl, "../gameassets", false)
	g.mapName, g.startMap = "map0003", "map0001"

	g.enterCredits()

	if g.mapName != creditsMapName {
		t.Fatalf("the demo should run on the credits arena, got %q", g.mapName)
	}
	if !g.creditsMode {
		t.Fatal("enterCredits should be in credits mode")
	}
	// The credits arena has no win resolution to trip.
	for _, r := range g.level.Resolutions {
		if r.Do == level.DoWin {
			t.Fatal("the credits arena must not carry a win resolution")
		}
	}
}

// TestEnterTitleThenStart: the front door opens on the attract demo (title over the autonomous
// sim), and pressing Start (restartCampaign) drops the title into a real run.
func TestEnterTitleThenStart(t *testing.T) {
	lvl := boxLevel(400, 400, nil)
	lvl.PlayerStart = level.Start{X: 200, Y: 200, Angle: -90}
	g := New(asset.New(), lvl, "", false)
	g.mapName, g.startMap = "arena", "arena"

	g.enterTitle()
	if !g.titleMode || !g.creditsMode {
		t.Fatalf("the title should run the attract demo: titleMode=%v creditsMode=%v", g.titleMode, g.creditsMode)
	}

	g.restartCampaign() // what Space does
	if g.titleMode || g.creditsMode {
		t.Fatal("starting the game must leave the title/attract state")
	}
}

// TestWallFollowNeedsFlood: with no flood field there is no wall gradient to follow, so the demo
// falls back to wandering (wallFollowDir reports false).
func TestWallFollowNeedsFlood(t *testing.T) {
	g := &Game{}
	if _, _, ok := g.wallFollowDir(); ok {
		t.Fatal("with no flood field, wall-following must report false")
	}
}

// TestTitleCreditsFromC: C on the title rolls the credits (drops the title, enters credits mode).
func TestTitleCreditsFromC(t *testing.T) {
	lvl := boxLevel(400, 400, nil)
	lvl.PlayerStart = level.Start{X: 200, Y: 200, Angle: -90}
	g := New(asset.New(), lvl, "", false)
	g.mapName, g.startMap = "arena", "arena"

	g.enterTitle()
	g.enterCredits() // what pressing C does

	if g.titleMode {
		t.Fatal("entering the credits should drop the title")
	}
	if !g.creditsMode {
		t.Fatal("C should put the game into the credits")
	}
}

// TestCreditsDemoBasicLoadout: the attract demo now flies a basic ship — front gun + laser turret
// + computer, no maxed mods — so it digs far less, and it is audible (not muted).
func TestCreditsDemoBasicLoadout(t *testing.T) {
	lvl := boxLevel(400, 400, nil)
	lvl.PlayerStart = level.Start{X: 200, Y: 200, Angle: -90}
	g := New(asset.New(), lvl, "", false)
	g.mapName, g.startMap = "arena", "arena"

	g.enterCredits()

	if g.slotWeaponName(1) != "laser" {
		t.Fatalf("the demo's turret should hold the laser, got %q", g.slotWeaponName(1))
	}
	if g.fireLevel != 0 {
		t.Fatalf("the demo should run a basic loadout (no maxed fire mod), got fireLevel %d", g.fireLevel)
	}
	if !g.hasComputer {
		t.Fatal("the demo should carry the fire computer")
	}
}

// TestCreditsArenaRegenerates: the attract backdrop regenerates on the timer, re-arming it and
// keeping the credits scroll (so the map swaps without restarting the credits).
func TestCreditsArenaRegenerates(t *testing.T) {
	lvl := boxLevel(400, 400, nil)
	lvl.PlayerStart = level.Start{X: 200, Y: 200, Angle: -90}
	g := New(asset.New(), lvl, "", false)
	g.mapName, g.startMap = "arena", "arena"

	g.enterCredits()
	if g.creditsRegenCD != creditsRegenFrames {
		t.Fatalf("entering credits should arm the regen timer, got %d", g.creditsRegenCD)
	}

	g.creditsScroll = 999 // pretend the credits have scrolled a while
	g.creditsRegenCD = 1  // about to regenerate
	g.stepCreditsMeta()   // ticks to 0 -> regenerates the arena

	if g.creditsRegenCD != creditsRegenFrames {
		t.Fatalf("the regen should re-arm the timer, got %d", g.creditsRegenCD)
	}
	if g.creditsScroll != 999 {
		t.Fatalf("the regen must preserve the credits scroll, got %.0f", g.creditsScroll)
	}
	if !g.creditsMode {
		t.Fatal("still in the attract demo after a regen")
	}
}
