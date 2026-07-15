package game

import (
	"linefire/level"
	"strings"
	"testing"
)

func TestLogAppendsCapsAndExpires(t *testing.T) {
	g := &Game{}
	for i := range 10 {
		g.logf("line %d", i)
	}
	if len(g.log) != logMax {
		t.Fatalf("the feed should cap at %d lines, got %d", logMax, len(g.log))
	}
	if g.log[0].text != "line 4" || g.log[logMax-1].text != "line 9" {
		t.Fatalf("the oldest lines should scroll off, got %q..%q", g.log[0].text, g.log[logMax-1].text)
	}

	for range logLife {
		g.stepLog()
	}
	if len(g.log) != 0 {
		t.Fatalf("aged-out lines should vanish, %d left", len(g.log))
	}
}

func TestEventsNarrateTheFeed(t *testing.T) {
	// Collecting a weapon narrates the pickup...
	g := New(loadoutTestPlayer(), level.New(), "", false)
	g.x, g.y = 0, 0
	g.entities = []entity{{kind: kindWeapon, power: "laser", x: 0, y: 0, radius: 16, spawn: 0}}
	g.resolveWeaponPickups()
	if !feedContains(g, "PICKUP") {
		t.Fatalf("collecting a weapon should hit the feed, got %+v", g.log)
	}

	// ...and breaking a pickup narrates the loss.
	g = breakableGame()
	shootRight(g, 99)
	if !feedContains(g, "DESTROYED") {
		t.Fatalf("a broken pickup should hit the feed, got %+v", g.log)
	}
}

func feedContains(g *Game, needle string) bool {
	for _, l := range g.log {
		if strings.Contains(l.text, needle) {
			return true
		}
	}
	return false
}
