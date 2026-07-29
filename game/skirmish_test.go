package game

import (
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
