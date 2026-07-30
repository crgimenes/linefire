package game

import (
	"math"
	"testing"

	"github.com/crgimenes/linefire/filoio"
)

func newTestSkirmish(t *testing.T) *Game {
	t.Helper()
	g, err := NewSkirmish(filoio.OSFS(), "../gameassets", SkirmishOptions{})
	if err != nil {
		t.Fatalf("NewSkirmish: %v", err)
	}
	return g
}

// Skirmish is the attract demo on a transparent window, so it has to come up as
// exactly that: the autonomous demo running, no title over it, no floor to paint
// and no fog to veil the desktop, and silent unless asked.
func TestSkirmishBootsIntoTheAttractDemo(t *testing.T) {
	g := newTestSkirmish(t)
	switch {
	case !g.creditsMode:
		t.Fatal("skirmish is not running the attract demo")
	case g.titleMode:
		t.Fatal("the title screen is up over the skirmish demo")
	case !g.skirmishMode || !g.transparent:
		t.Fatal("the overlay flags are not set")
	case g.floodView:
		t.Fatal("the flood view is on; its fills cannot carry a transparent screen")
	case g.sfx != nil:
		t.Fatal("skirmish built an audio context without being asked")
	}
	if g.horde == nil {
		t.Fatal("no endless horde: the demo has nothing to fight")
	}
	for i := range 300 {
		if err := g.Update(); err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
	}
	if g.over || g.health <= 0 {
		t.Fatal("the attract ship should be indestructible")
	}
}

// The camera is the first real difference from linefire: it must not ride the
// ship. With several ships a side, a view that turned with one of them would swing
// everything else around the screen.
func TestSkirmishCameraDoesNotRideTheShip(t *testing.T) {
	g := newTestSkirmish(t)
	g.sw, g.sh = 800, 600
	g.dpr = 1

	before := g.cameraGeoM()
	g.x, g.y, g.angle = g.x+300, g.y-200, g.angle+90
	if after := g.cameraGeoM(); after != before {
		t.Fatal("the camera moved when the ship did")
	}

	// And the pose is the middle of the map, unturned.
	cx, cy, angle := g.camPose()
	wantX := (g.bounds.minX + g.bounds.maxX) / 2
	wantY := (g.bounds.minY + g.bounds.maxY) / 2
	if cx != wantX || cy != wantY {
		t.Errorf("camera at %.0f,%.0f, want the map centre %.0f,%.0f", cx, cy, wantX, wantY)
	}
	if angle != -90 {
		t.Errorf("camera angle %v; -90 is what cancels the rotation", angle)
	}

	// A still camera has no camera motion, so nothing to smear.
	if got := g.blurSamples(); got != 1 {
		t.Errorf("blurSamples = %d with a fixed camera, want 1", got)
	}
}

// With the camera still, the hull has to move and turn on screen — so it is drawn
// in world space, where every other entity is, not pinned to the centre.
func TestSkirmishShipIsDrawnInTheWorld(t *testing.T) {
	g := newTestSkirmish(t)
	g.sw, g.sh = 800, 600
	g.dpr = 1

	camX, camY, camAngle := g.camPose()
	before := g.playerWorldGeoM(camX, camY, camAngle)
	pinnedBefore := g.playerGeoM()

	g.x += 250
	g.angle += 40

	if g.playerWorldGeoM(camX, camY, camAngle) == before {
		t.Fatal("the hull draws in the same place after moving and turning")
	}
	// The screen-centre transform is what it must NOT be using: that one never
	// notices the ship moved, which is exactly why it cannot be the arena path.
	if g.playerGeoM() != pinnedBefore {
		t.Fatal("playerGeoM moved; it is supposed to be pinned to the centre")
	}
}

// The arena is sized to what the camera shows, so the whole field is visible with
// its walls on the edges — and it is rebuilt if that ever stops being true.
func TestSkirmishArenaFitsTheView(t *testing.T) {
	g := newTestSkirmish(t)
	g.sw, g.sh = 1200, 800
	g.dpr = 1
	if g.arenaFitsView() {
		t.Fatal("the arena built before any Layout should not already fit the view")
	}

	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !g.arenaFitsView() {
		w, h := g.arenaWorldSize()
		t.Fatalf("arena is %.0fx%.0f, view wants %.0fx%.0f", g.level.Size.W, g.level.Size.H, w, h)
	}

	wantW, wantH := g.arenaWorldSize()
	if math.Abs(g.level.Size.W-wantW) > 1 || math.Abs(g.level.Size.H-wantH) > 1 {
		t.Errorf("arena %.0fx%.0f does not match the view %.0fx%.0f", g.level.Size.W, g.level.Size.H, wantW, wantH)
	}
}

// The arena regenerates every ~20s through buildCreditsArena, which rebuilds the
// whole Game — the overlay flags have to survive that, or the first regen would
// silently turn skirmish back into the credits screen with an opaque floor.
func TestSkirmishSurvivesTheArenaRegen(t *testing.T) {
	g := newTestSkirmish(t)
	g.debugHUD = true
	g.creditsRegenCD = 1
	if err := g.Update(); err != nil {
		t.Fatalf("regen update: %v", err)
	}
	switch {
	case !g.skirmishMode || !g.transparent:
		t.Fatal("the overlay flags did not survive the regen")
	case g.floodView:
		t.Fatal("the regen turned the flood view back on")
	case !g.debugHUD:
		t.Fatal("the debug HUD choice did not survive the regen")
	case !g.creditsMode || g.titleMode:
		t.Fatal("the regen left the demo in the wrong screen")
	case !g.arenaCam:
		t.Fatal("the arena camera did not survive the regen")
	}
	// And the show goes on.
	for range 120 {
		if err := g.Update(); err != nil {
			t.Fatalf("post-regen update: %v", err)
		}
	}
}

// The attract ship cannot die, which is what makes an unattended overlay safe:
// there is no game-over screen to strand it on.
func TestSkirmishShipIsIndestructible(t *testing.T) {
	g := newTestSkirmish(t)
	for range 10 {
		g.hurtPlayer(1000)
	}
	if g.over || g.health <= 0 {
		t.Fatalf("the skirmish ship died: over=%v health=%d", g.over, g.health)
	}
}
