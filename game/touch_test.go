package game

import (
	"math"
	"testing"
)

// TestSteerDelta: the left stick's screen deflection maps onto a heading change
// relative to the ship's nose (screen-up = 0), so "push where you want to go" holds
// in every direction. Screen y grows DOWN.
func TestSteerDelta(t *testing.T) {
	cases := []struct {
		dx, dy, want float64
	}{
		{0, -1, 0},    // up: keep heading
		{1, 0, 90},    // right: turn right
		{-1, 0, -90},  // left: turn left
		{0, 1, 180},   // down: turn around
		{1, -1, 45},   // up-right diagonal
		{-1, -1, -45}, // up-left diagonal
	}
	for _, c := range cases {
		got := steerDelta(c.dx, c.dy)
		if math.Abs(normDeg(got-c.want)) > 1e-9 {
			t.Fatalf("steerDelta(%v,%v) = %v, want %v", c.dx, c.dy, got, c.want)
		}
	}
}

// TestClaimThumb: a fresh touch joins the half its ORIGIN is in, an occupied half
// refuses a second finger, and a new right touch always starts in nose-fire mode.
func TestClaimThumb(t *testing.T) {
	var tc touchCtl
	const mid = 400.0

	l := tc.claimThumb(1, 100, 500, mid)
	if l != &tc.left || !tc.left.active || !tc.left.just {
		t.Fatal("a touch left of the middle should claim the left thumb")
	}
	if tc.claimThumb(2, 50, 200, mid) != nil {
		t.Fatal("an occupied half must refuse a second finger")
	}

	tc.turret = true // stale from a previous touch: the new claim must clear it
	r := tc.claimThumb(3, 700, 500, mid)
	if r != &tc.right || !tc.right.active {
		t.Fatal("a touch right of the middle should claim the right thumb")
	}
	if tc.turret {
		t.Fatal("a fresh right touch starts as nose fire, not turret")
	}

	// The deflection is measured from the landing point, not the screen.
	tc.right.x, tc.right.y = 760, 460
	dx, dy := tc.right.deflection()
	if dx != 60 || dy != -40 {
		t.Fatalf("deflection = (%v,%v), want (60,-40)", dx, dy)
	}
}

// TestTouchAimPoint: the turret stick synthesizes a "cursor" at touchAimReachPx from
// the screen center along the deflection; without turret mode there is no point and
// callers fall back to the mouse.
func TestTouchAimPoint(t *testing.T) {
	g := &Game{dpr: 1, sw: 800, sh: 600}
	touchPad = touchCtl{}
	defer func() { touchPad = touchCtl{} }()

	if _, _, ok := g.touchAimPoint(); ok {
		t.Fatal("no touch: no synthetic aim point")
	}

	touchPad.right = thumb{active: true, ox: 600, oy: 400, x: 700, y: 400} // dragged right
	touchPad.turret = true
	x, y, ok := g.touchAimPoint()
	if !ok {
		t.Fatal("turret mode should yield an aim point")
	}
	if math.Abs(x-(400+touchAimReachPx)) > 1e-9 || math.Abs(y-300) > 1e-9 {
		t.Fatalf("aim point = (%v,%v), want (%v,300)", x, y, 400+touchAimReachPx)
	}
}

// TestCapScaleNative: the native profile leaves the device scale factor alone (the
// web profile caps it at 1× — a const, exercised by the wasm build).
func TestCapScaleNative(t *testing.T) {
	if got := capScale(2.0); got != 2.0 {
		t.Fatalf("native capScale(2) = %v, want 2 (uncapped)", got)
	}
	if got := capScale(0); got != 1.0 {
		t.Fatalf("capScale(0) = %v, want the 1.0 fallback", got)
	}
}
