package procgen

import (
	"testing"

	"github.com/crgimenes/linefire/asset"
)

// The point of an arena is that its wall layer is a handful of segments, not
// thousands: it is for a caller that has to STROKE its walls, where a cave is
// affordable only because it is drawn as negative space.
func TestArenaHasFewWalls(t *testing.T) {
	lvl := GenArena(1)
	if len(lvl.Walls) != 1 {
		t.Fatalf("want one wall layer, got %d", len(lvl.Walls))
	}
	paths := lvl.Walls[0].Paths
	if len(paths) != 1 {
		t.Fatalf("want one perimeter path, got %d", len(paths))
	}
	if n := len(paths[0].Commands); n > 8 {
		t.Errorf("the perimeter is %d commands; an arena is a rectangle", n)
	}

	cave := GenCaveRoom(1, "")
	if len(cave.Walls[0].Paths) <= len(paths) {
		t.Fatal("the cave should have far more wall paths than the arena; the comparison is the whole point")
	}
}

// Whatever fights in it has to be inside it, and the perimeter has to actually
// close, or ships would fly out through the gap.
func TestArenaIsClosedAroundItsStart(t *testing.T) {
	lvl := GenArena(7)
	start := lvl.PlayerStart
	if start.X <= 0 || start.X >= lvl.Size.W || start.Y <= 0 || start.Y >= lvl.Size.H {
		t.Fatalf("start %.0f,%.0f is outside the %.0fx%.0f arena", start.X, start.Y, lvl.Size.W, lvl.Size.H)
	}

	cmds := lvl.Walls[0].Paths[0].Commands
	if cmds[len(cmds)-1].Op != asset.OpClose {
		t.Error("the perimeter does not close")
	}
	for _, c := range cmds {
		if c.Op == asset.OpClose {
			continue
		}
		if c.X < 0 || c.X > lvl.Size.W || c.Y < 0 || c.Y > lvl.Size.H {
			t.Errorf("perimeter point %.0f,%.0f is outside the declared size", c.X, c.Y)
		}
	}
}

// An arena is a place to fight, not a stage: what fights in it is the caller's.
func TestArenaShipsNothingToFight(t *testing.T) {
	if got := len(GenArena(3).Spawns); got != 0 {
		t.Errorf("GenArena placed %d spawns; the caller decides what fights", got)
	}
}

// Same seed, same arena — it is generated, so it has to be reproducible.
func TestArenaIsDeterministic(t *testing.T) {
	a, b := GenArena(42), GenArena(42)
	if a.Name != b.Name || a.Size != b.Size || a.PlayerStart != b.PlayerStart {
		t.Error("the same seed produced a different arena")
	}
}
