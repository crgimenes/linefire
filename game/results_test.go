package game

import "testing"

func TestCheckLevelClearFiresOnce(t *testing.T) {
	g := &Game{runTicks: 600, levelStartTicks: 60}
	g.objectives = []objective{{kind: objEliminate, done: true}}

	g.checkLevelClear()
	if g.clearBannerTicks == 0 || !g.levelCleared {
		t.Fatal("clearing every objective should raise the non-blocking clear banner")
	}
	if g.clearTicks != 540 {
		t.Fatalf("clear time = runTicks - levelStartTicks, got %d", g.clearTicks)
	}

	g.clearBannerTicks = 0
	g.checkLevelClear()
	if g.clearBannerTicks != 0 {
		t.Fatal("the clear banner must not re-fire once latched")
	}
}

func TestRevisitClearedMapDoesNotReFire(t *testing.T) {
	// Returning to a map that was already cleared (progress restored from mapState)
	// arrives with objectives complete. It must NOT fire the results — that logged a
	// ~0s clear as a record.
	g := &Game{runTicks: 1000, levelStartTicks: 1000}
	g.objectives = []objective{{kind: objReach, zone: "exit", done: true}}

	g.syncLevelCleared()
	if !g.levelCleared {
		t.Fatal("an already-complete map should latch cleared on entry")
	}
	g.checkLevelClear()
	if g.clearBannerTicks != 0 {
		t.Fatal("revisiting a cleared map must not flash the banner (no 0s record)")
	}
}
