package mapeditor

import (
	"path/filepath"
	"testing"

	"github.com/crgimenes/linefire/level"
)

// TestMapStem: the map's name is its file stem with the .lfm extension dropped, and
// an unsaved map (no path) simply has no name.
func TestMapStem(t *testing.T) {
	cases := map[string]string{
		filepath.Join("a", "b", "map0005.lfm"): "map0005",
		"x.lfm":                                "x",
		"map0001.lfm":                          "map0001",
		"":                                     "",
	}
	for in, want := range cases {
		if got := mapStem(in); got != want {
			t.Fatalf("mapStem(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPlaytestHandsOutACopy: F5 must never let the play session reach the document
// being edited — the host receives a clone, not the editor's own level.
func TestPlaytestHandsOutACopy(t *testing.T) {
	e := New(level.New(), "gameassets/map0001.lfm", "gameassets")

	var got *level.Level
	var gotName string
	e.SetOnPlaytest(func(lvl *level.Level, name string) {
		got, gotName = lvl, name
	})
	e.playtest()

	if got == nil {
		t.Fatal("playtest did not reach the host")
	}
	if got == e.level {
		t.Fatal("host received the editor's own level; edits during play would corrupt the document")
	}
	if gotName != "map0001" {
		t.Fatalf("name = %q, want %q", gotName, "map0001")
	}
}

// TestPlaytestWithoutHostIsInert: standalone (no host wired), F5 reports rather than
// panicking on a nil callback.
func TestPlaytestWithoutHostIsInert(t *testing.T) {
	e := New(level.New(), "gameassets/map0001.lfm", "gameassets")
	e.playtest() // must not panic
	if e.status == "" {
		t.Fatal("playtest without a host should say so in the status line")
	}
}
