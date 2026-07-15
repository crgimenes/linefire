package mapeditor

import (
	"os"
	"path/filepath"
	"testing"

	"linefire/asset"
	"linefire/level"
)

// TestImportSVGFileAppendsWalls: importing an SVG APPENDS its shapes to the open map's wall
// layer (keeping any existing wall), snaps the coordinates to whole units, and dirties the map.
func TestImportSVGFileAppendsWalls(t *testing.T) {
	dir := t.TempDir()
	svg := filepath.Join(dir, "in.svg")
	if err := os.WriteFile(svg, []byte(`<svg><circle cx="50" cy="50" r="20"/></svg>`), 0o600); err != nil {
		t.Fatal(err)
	}

	e := &MapEditor{level: level.New()}
	// a wall already on the map that the import must not clobber
	e.level.Walls[0].Paths = append(e.level.Walls[0].Paths, asset.Path{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 5, Y: 5}, {Op: asset.OpLineTo, X: 9, Y: 9}, {Op: asset.OpClose},
	}})

	e.importSVGFile(svg)

	if got := len(e.level.Walls[0].Paths); got != 2 {
		t.Fatalf("import should append and keep the existing wall (want 2 paths), got %d", got)
	}
	if !e.dirty {
		t.Fatal("importing SVG should mark the map dirty (so it saves and undoes)")
	}
	for _, c := range e.level.Walls[0].Paths[1].Commands {
		if c.X != float64(int(c.X)) || c.Y != float64(int(c.Y)) {
			t.Fatalf("imported coordinates should be whole units, got (%v,%v)", c.X, c.Y)
		}
	}
}

// TestImportSVGPlacesSpawnsAndStart: labeled shapes become spawns (palette asset by name,
// portal:target, player-start) at their centers; unlabeled or unknown-label shapes are walls.
func TestImportSVGPlacesSpawnsAndStart(t *testing.T) {
	dir := t.TempDir()
	svg := filepath.Join(dir, "in.svg")
	doc := `<svg>
		<rect x="0" y="0" width="200" height="200"/>
		<circle id="turret" cx="100" cy="100" r="8"/>
		<circle id="player-start" cx="50" cy="60" r="8"/>
		<circle id="portal:map0002" cx="300" cy="40" r="8"/>
		<circle id="mystery" cx="10" cy="10" r="8"/>
	</svg>`
	if err := os.WriteFile(svg, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}

	e := &MapEditor{level: level.New()}
	e.palette = []paletteEntry{{ref: "turret", name: "turret", asset: &asset.Asset{Kind: "turret"}}}

	e.importSVGFile(svg)

	// walls: the rect + the unknown "mystery" circle (unknown label falls back to wall)
	if got := len(e.level.Walls[0].Paths); got != 2 {
		t.Fatalf("want 2 walls (rect + unknown), got %d", got)
	}
	// spawns: turret + portal (player-start is not a spawn, it moves PlayerStart)
	if got := len(e.level.Spawns); got != 2 {
		t.Fatalf("want 2 spawns (turret + portal), got %d: %+v", got, e.level.Spawns)
	}
	var turret, portal *level.Spawn
	for i := range e.level.Spawns {
		switch e.level.Spawns[i].Kind {
		case "turret":
			turret = &e.level.Spawns[i]
		case "portal":
			portal = &e.level.Spawns[i]
		}
	}
	if turret == nil || turret.Asset != "turret" || turret.X != 100 || turret.Y != 100 {
		t.Fatalf("turret spawn should reference the palette asset at its center, got %+v", turret)
	}
	if portal == nil || portal.Target != "map0002" || portal.X != 300 || portal.Y != 40 {
		t.Fatalf("portal should carry its target and center, got %+v", portal)
	}
	if e.level.PlayerStart.X != 50 || e.level.PlayerStart.Y != 60 {
		t.Fatalf("player-start marker should move PlayerStart, got %+v", e.level.PlayerStart)
	}
}

// TestPollSVGImportCancel: cancelling the SVG panel imports nothing and leaves the map clean.
func TestPollSVGImportCancel(t *testing.T) {
	e := &MapEditor{level: level.New()}
	ch := make(chan string, 1)
	ch <- ""
	e.svgResult = ch

	e.pollSVGImport()

	if e.svgResult != nil {
		t.Fatal("the SVG channel should clear even on cancel")
	}
	if len(e.level.Walls[0].Paths) != 0 {
		t.Fatal("cancelling the panel must import nothing")
	}
	if e.dirty {
		t.Fatal("cancelling must not dirty the map")
	}
}
