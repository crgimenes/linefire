package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crgimenes/linefire/filoio"
)

// TestRunImportsAndLoadsBack: the tool turns an SVG into a map the game can actually load —
// the emitted .lfm round-trips through filoio with the walls, size and title in place.
func TestRunImportsAndLoadsBack(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.svg")
	out := filepath.Join(dir, "out.lfm")
	svg := `<svg><rect x="10" y="20" width="100" height="80"/><circle cx="400" cy="300" r="50"/></svg>`
	if err := os.WriteFile(in, []byte(svg), 0o600); err != nil {
		t.Fatal(err)
	}

	n, err := run(in, out, 1, "Import Test")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("want 2 wall paths (rect + circle), got %d", n)
	}

	lvl, err := filoio.LoadLevel(out)
	if err != nil {
		t.Fatalf("the emitted map should load through filoio: %v", err)
	}
	if len(lvl.Walls) != 1 || len(lvl.Walls[0].Paths) != 2 {
		t.Fatalf("want one wall layer with 2 paths, got %+v", lvl.Walls)
	}
	if lvl.Title != "Import Test" {
		t.Fatalf("title should carry into the map, got %q", lvl.Title)
	}
	// Coordinates are snapped to whole world units (no curve-flattening float noise).
	for _, c := range lvl.Walls[0].Paths[1].Commands {
		if c.X != float64(int(c.X)) || c.Y != float64(int(c.Y)) {
			t.Fatalf("imported coordinates should be whole units, got (%v,%v)", c.X, c.Y)
		}
	}
}
