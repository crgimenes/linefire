package editor

import (
	"os"
	"path/filepath"
	"testing"

	"linefire/asset"
)

func TestEventsForKind(t *testing.T) {
	for _, ev := range eventsFor("tank") {
		if ev == "pickup" || ev == "thruster" {
			t.Fatalf("an enemy kind should not suggest %q", ev)
		}
	}
	got := eventsFor("shield")
	if len(got) != 2 || got[0] != "pickup" || got[1] != "break" {
		t.Fatalf("a powerup kind should suggest pickup and break, got %v", got)
	}
	if len(eventsFor("something-new")) == 0 {
		t.Fatal("an unknown kind should fall back to the full event set")
	}
}

func TestAddSuggestsMissingEventsWithDefaults(t *testing.T) {
	a := asset.New()
	a.Kind = "tank"

	s := nextSuggestedSound(a) // nothing declared: the first suggestion
	if s.Event != "fire" || s.Base != "laser" {
		t.Fatalf("first suggestion should be a playable fire, got %+v", s)
	}
	a.Sounds = append(a.Sounds, s)

	s = nextSuggestedSound(a) // fire taken: the next missing one
	if s.Event != "destroy" || s.Base != "explosion" {
		t.Fatalf("next suggestion should be destroy, got %+v", s)
	}
	a.Sounds = append(a.Sounds, s, defaultSoundFor("theme"))

	s = nextSuggestedSound(a) // all declared: cycle back, never invent names
	if s.Event != "fire" {
		t.Fatalf("with everything declared it should restart at fire, got %+v", s)
	}

	if d := defaultSoundFor("theme"); d.Base != "battle" {
		t.Fatalf("a theme should default to a music mood, got %q", d.Base)
	}
	if d := defaultSoundFor("thruster"); !d.Continuous {
		t.Fatal("a thruster default should be continuous")
	}
}

func TestListMusicFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.mp3", "a.MP3", "notes.txt"} {
		err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}
	got := listMusicFiles(dir)
	if len(got) != 2 {
		t.Fatalf("should list only the mp3s, got %v", got)
	}
	for _, p := range got {
		if filepath.Dir(p) != dir {
			t.Fatalf("paths should keep the dir prefix (how docs reference them): %q", p)
		}
	}
	if listMusicFiles(filepath.Join(dir, "missing")) != nil {
		t.Fatal("a missing dir should list nothing")
	}
}

func TestSoundPanelToggleAndHover(t *testing.T) {
	e := New(asset.New(), "")
	if e.sndPanelHovered(sndPanelX, sndPanelY) {
		t.Fatal("a closed panel must not claim hover")
	}
	e.toggleSoundPanel()
	if !e.sndPanel {
		t.Fatal("toggle should open the panel")
	}
	if !e.sndPanelHovered(sndPanelX, sndPanelY) {
		t.Fatal("a point inside the open panel should hover")
	}
	if e.sndPanelHovered(900, 30) {
		t.Fatal("a point outside the panel must not hover")
	}
	e.toggleSoundPanel()
	if e.sndPanel {
		t.Fatal("toggle should close the panel")
	}
}
