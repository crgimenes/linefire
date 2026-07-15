package game

import "testing"

func TestClearPathRespectsRadius(t *testing.T) {
	g := &Game{segs: []segment{{ax: 10, ay: -50, bx: 10, by: 50}}} // wall at x=10

	// A path running 10 units from the wall is not clear for a radius-12 disc.
	if g.clearPath(0, 0, 0, 20, 12) {
		t.Fatal("path within radius of a wall should not be clear")
	}
	// The same path is clear for a smaller radius.
	if !g.clearPath(0, 0, 0, 20, 5) {
		t.Fatal("path clear of walls by more than the radius should be clear")
	}
}
