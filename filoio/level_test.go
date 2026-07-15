package filoio

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"linefire/asset"
	"linefire/level"
)

// canonicalLevel drops nil-vs-empty on the slices the JSON encoder always wrote out.
// See canonical (asset_test.go) for why that distinction is noise and not data.
func canonicalLevel(l *level.Level) *level.Level {
	c := *l
	if len(c.Walls) == 0 {
		c.Walls = nil
	}
	if len(c.Spawns) == 0 {
		c.Spawns = nil
	}
	if len(c.Zones) == 0 {
		c.Zones = nil
	}
	if len(c.Entries) == 0 {
		c.Entries = nil
	}
	if len(c.Resolutions) == 0 {
		c.Resolutions = nil
	}
	if len(c.Tags) == 0 {
		c.Tags = nil
	}
	walls := make([]asset.Layer, len(c.Walls))
	for i, w := range c.Walls {
		walls[i] = w
		if len(w.Paths) == 0 {
			walls[i].Paths = nil
		}
	}
	c.Walls = walls
	if len(c.Walls) == 0 {
		c.Walls = nil
	}
	return &c
}

// TestLevelRoundTripsEveryRealMap is the same gate the assets pass: a map must survive
// file -> model -> Filo -> model untouched, or the map editor's Save eats the author's
// work. Maps carry far more distinct field kinds than assets (spawns, zones, entries,
// resolutions, a horde), so this is the stronger of the two.
func TestLevelRoundTripsEveryRealMap(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "gameassets", "*"+ExtLevel))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no maps found in gameassets: %v", err)
	}
	for _, p := range paths {
		want, err := LoadLevel(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		t.Run(filepath.Base(p), func(t *testing.T) {
			src := EmitLevel(want)
			got, err := ParseLevel(src)
			if err != nil {
				t.Fatalf("re-parsing what we emitted failed: %v\n--- emitted ---\n%s", err, src)
			}
			if !reflect.DeepEqual(canonicalLevel(want), canonicalLevel(got)) {
				t.Fatalf("round trip lost data\n--- emitted ---\n%s\n--- want %+v\n--- got  %+v", src, want, got)
			}
		})
	}
	t.Logf("round-tripped %d real maps", len(paths))
}

// TestLevelGrammarByHand pins the surface and covers what the shipped maps do not:
// named entries, a rect zone, every resolution routine, a full horde.
func TestLevelGrammarByHand(t *testing.T) {
	src := `
(level "proving-ground"
  (version 2)
  (tags "test")
  (size 512 512)
  (music "music/theme.mp3")
  (music-seed 7)
  (player-start 100 120 -90)
  (entry "north" 40 10 -90)
  (entry "south" 40 500 90)
  (wall "outer"
    (stroke "#80ffff") (width 2) (fill "transparent") (glow 1.4)
    (path (move 20 20) (line 490 20) (line 490 490) (line 20 490) (close)))
  (spawn "e1" "tank" 300 220 90 (kind "enemy"))
  (spawn "exit" "portal" 480 40 0 (kind "portal") (target "map0002:north"))
  (zone "cp" "circle" 200 200 10 (trigger "checkpoint"))
  (zone "room" "rect" 0 0 100 200)
  (resolution "cleared" "spawn" (target "shield") (at 120 120))
  (resolution "cleared" "exit" (target "map0002"))
  (horde (types "enemy" "rusher") (interval 24) (max-alive 18) (ramp 600) (seed 7))
  (editor (snap) (snap-radius 8) (grid) (grid-size 8)))
`
	l, err := ParseLevel(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if l.Name != "proving-ground" || l.Music != "music/theme.mp3" || l.MusicSeed != 7 {
		t.Fatalf("header wrong: %+v", l)
	}
	if l.PlayerStart != (level.Start{X: 100, Y: 120, Angle: -90}) {
		t.Fatalf("player start wrong: %+v", l.PlayerStart)
	}
	if len(l.Entries) != 2 || l.Entries[1].Name != "south" {
		t.Fatalf("entries wrong: %+v", l.Entries)
	}
	if len(l.Walls) != 1 || len(l.Walls[0].Paths[0].Commands) != 5 {
		t.Fatalf("walls wrong: %+v", l.Walls)
	}
	if l.Spawns[0].Asset != "tank" || l.Spawns[0].Kind != "enemy" {
		t.Fatalf("spawn wrong: %+v", l.Spawns[0])
	}
	if l.Spawns[1].Target != "map0002:north" {
		t.Fatalf("portal target wrong: %+v", l.Spawns[1])
	}
	if l.Zones[0].Radius != 10 || l.Zones[0].Trigger != "checkpoint" {
		t.Fatalf("circle zone wrong: %+v", l.Zones[0])
	}
	if l.Zones[1].Kind != level.ZoneRect || len(l.Zones[1].Points) != 2 {
		t.Fatalf("rect zone wrong: %+v", l.Zones[1])
	}
	if l.Resolutions[0].Target != "shield" || l.Resolutions[0].X != 120 {
		t.Fatalf("spawn resolution wrong: %+v", l.Resolutions[0])
	}
	if l.Horde == nil || l.Horde.MaxAlive != 18 || l.Horde.Seed != 7 || len(l.Horde.Types) != 2 {
		t.Fatalf("horde wrong: %+v", l.Horde)
	}

	again, err := ParseLevel(EmitLevel(l))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if !reflect.DeepEqual(canonicalLevel(l), canonicalLevel(again)) {
		t.Fatalf("hand-written level did not survive a round trip\n%s", EmitLevel(l))
	}
}

// TestLevelIsAProgramNotJustData: a map can compute its own geometry. This is what an
// inline generator will grow out of — a room is a bound symbol away from a function.
// TestBossFlagRoundTrips: (boss) is a presence flag on a spawn — parsed to Spawn.Boss, emitted
// only when true, and surviving a round trip (the shielded-boss marker for the finale).
func TestBossFlagRoundTrips(t *testing.T) {
	src := `(level "arena"
  (version 2)
  (size 512 512)
  (player-start 256 256 -90)
  (wall "w" (stroke "#80ffff") (width 2) (fill "transparent") (glow 0.8)
    (path (move 0 0) (line 10 0) (line 10 10) (close)))
  (spawn "boss" "tank" 300 220 90 (kind "enemy") (boss))
  (spawn "grunt" "enemy" 100 100 -90 (kind "enemy"))
  (editor (snap) (snap-radius 8) (grid) (grid-size 8)))`

	l, err := ParseLevel(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !l.Spawns[0].Boss {
		t.Fatal("the (boss) spawn should parse as Boss=true")
	}
	if l.Spawns[1].Boss {
		t.Fatal("a spawn without (boss) should be Boss=false")
	}

	out := EmitLevel(l)
	if strings.Count(out, "(boss)") != 1 {
		t.Fatalf("(boss) should be emitted exactly once (only for the true one):\n%s", out)
	}

	again, err := ParseLevel(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if !again.Spawns[0].Boss || again.Spawns[1].Boss {
		t.Fatalf("the boss flag did not survive a round trip: %+v", again.Spawns)
	}
}

func TestLevelIsAProgramNotJustData(t *testing.T) {
	src := `
(def w 400)
(def m 20)
(def far (- w m))
(level "computed"
  (version 2)
  (size w w)
  (player-start 200 200 -90)
  (wall "outer" (stroke "#80ffff") (width 2) (fill "transparent") (glow 1)
    (path (move m m) (line far m) (line far far) (line m far) (close)))
  (editor (snap) (snap-radius 8) (grid) (grid-size 8)))
`
	l, err := ParseLevel(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cmds := l.Walls[0].Paths[0].Commands
	if cmds[1].X != 380 || cmds[2].Y != 380 {
		t.Fatalf("inline arithmetic did not reach the wall: %+v", cmds)
	}
}

// TestLevelRejectsAnAsset is the mirror of TestAssetRejectsALevel: each reader knows
// only its own top form, so the two document types can never be confused again.
func TestLevelRejectsAnAsset(t *testing.T) {
	_, err := ParseLevel(`(asset "player" (version 2) (size 64 64) (origin 32 32))`)
	if err == nil {
		t.Fatal("the level reader must refuse an asset document")
	}
}

// TestLevelRejectsBadInput: a document that lies fails loudly, naming the form.
func TestLevelRejectsBadInput(t *testing.T) {
	head := `(level "x" (version 2) (size 100 100) (player-start 50 50 0) `
	cases := map[string]string{
		"no level form":  `(version 2)`,
		"unknown form":   head + `(weather "rain"))`,
		"unknown zone":   head + `(zone "z" "hexagon" 1 2 3))`,
		"short zone":     head + `(zone "z" "rect" 1 2))`,
		"short spawn":    head + `(spawn "s" "tank" 1 2))`,
		"future version": `(level "x" (version 99) (size 1 1) (player-start 0 0 0))`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseLevel(src)
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// TestSaveLevelRoundTripsOnDisk: a save must produce a file LoadLevel accepts.
func TestSaveLevelRoundTripsOnDisk(t *testing.T) {
	want := level.New()
	want.Name = "fresh"
	path := filepath.Join(t.TempDir(), "fresh"+ExtLevel)
	err := SaveLevel(path, want)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadLevel(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(canonicalLevel(want), canonicalLevel(got)) {
		t.Fatalf("save/load lost data\nwant %+v\ngot  %+v", want, got)
	}
}
