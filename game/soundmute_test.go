package game

import (
	"path/filepath"
	"testing"

	"linefire/filoio"
)

// TestDemoMuteNeverPersists is the regression guard for the bug that muted crg's config:
// the credits demo silences audio through demoMute, a TRANSIENT flag. A save that fires
// while the demo is running must write the player's real (unmuted) preference, so the
// next launch is not stranded silent.
func TestDemoMuteNeverPersists(t *testing.T) {
	b := &soundBank{cfgPath: filepath.Join(t.TempDir(), "config"+filoio.ExtConfig), master: 0.7}

	b.demoMute = true // the credits demo is playing silent
	if !b.silent() {
		t.Fatal("demoMute must silence playback")
	}
	if b.muted {
		t.Fatal("demoMute must not touch the saved mute preference")
	}

	b.saveConfig() // a save fires mid-demo (menu, F6, quit)

	got := filoio.LoadConfig(b.cfgPath)
	if got.Muted {
		t.Fatal("the transient demo mute leaked into the saved config — the launch-silent bug")
	}

	// The real mute, by contrast, does persist.
	b.muted = true
	b.saveConfig()
	if !filoio.LoadConfig(b.cfgPath).Muted {
		t.Fatal("the player's own mute choice must be saved")
	}
}
