package editapp

import (
	"testing"

	"linefire/level"
)

// TestModeSwitch checks the host swaps between the map editor and the asset editor
// and back, the core of the single-window two-mode design.
func TestModeSwitch(t *testing.T) {
	app := New(level.New(), "", "")
	if app.cur != app.mapEd {
		t.Fatal("should start in the map editor")
	}

	app.openAsset("../gameassets/enemy.lfa")
	app.applyPending() // the swap is staged until the frame boundary
	if app.cur == app.mapEd {
		t.Fatal("openAsset should switch away from the map editor")
	}

	app.backToMap()
	app.applyPending()
	if app.cur != app.mapEd {
		t.Fatal("backToMap should return to the map editor")
	}
}

// TestOpenAssetUnloadableStays checks a bad path does not switch modes.
func TestOpenAssetUnloadableStays(t *testing.T) {
	app := New(level.New(), "", "")
	app.openAsset("does-not-exist.lfa")
	app.applyPending()
	if app.cur != app.mapEd {
		t.Fatal("an unloadable asset should leave the map editor active")
	}
}
