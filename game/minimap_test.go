package game

import (
	"math"
	"testing"
)

func TestClipSegToCircle(t *testing.T) {
	// Crossing a circle r=5 at the origin -> clipped to the inside chord.
	x0, y0, x1, y1, ok := clipSegToCircle(-10, 0, 10, 0, 0, 0, 5)
	if !ok || math.Abs(x0+5) > 1e-9 || math.Abs(x1-5) > 1e-9 || math.Abs(y0) > 1e-9 || math.Abs(y1) > 1e-9 {
		t.Fatalf("clip = (%v,%v)-(%v,%v) ok=%v, want (-5,0)-(5,0)", x0, y0, x1, y1, ok)
	}

	// Fully outside -> dropped.
	_, _, _, _, ok = clipSegToCircle(10, 10, 20, 20, 0, 0, 5)
	if ok {
		t.Fatal("a segment entirely outside the circle should be dropped")
	}

	// Fully inside -> unchanged.
	a, b, c, d, ok := clipSegToCircle(-1, 0, 1, 0, 0, 0, 5)
	if !ok || a != -1 || b != 0 || c != 1 || d != 0 {
		t.Fatalf("inside segment changed: (%v,%v)-(%v,%v) ok=%v", a, b, c, d, ok)
	}
}
