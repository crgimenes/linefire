package editapp

import (
	"testing"

	"github.com/crgimenes/linefire/level"
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

// TestPlaytestModeSwitch: F5 puts the game in the same window on the map being edited,
// and leaving it returns to the very same editor instance (the document is untouched).
func TestPlaytestModeSwitch(t *testing.T) {
	app := New(level.New(), "", "../gameassets")
	ed := app.mapEd

	app.playtest(level.New(), "map0001")
	app.applyPending()
	if app.cur == app.mapEd {
		t.Fatal("playtest should switch away from the map editor")
	}
	if !app.playing {
		t.Fatal("the host must know a playtest is running, or Termination would quit the app")
	}

	app.stopPlaytest()
	app.applyPending()
	if app.cur != ed {
		t.Fatal("leaving a playtest must return to the SAME editor, not a rebuilt one")
	}
	if app.playing {
		t.Fatal("playing should be false once back in the editor")
	}
}
