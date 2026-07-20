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

// TestTouchTurnStep: the steering response curve — dead inside the angular
// deadzone (straight flight tolerates thumb error), proportional above it, and
// capped at the full keyboard turn rate only on a full throw. The flat full-rate
// response made straight flight nearly impossible: any misalignment turned the
// ship at 3°/frame indefinitely.
func TestTouchTurnStep(t *testing.T) {
	cases := []struct {
		delta, throw, want float64
	}{
		{0, 1, 0},                            // dead ahead: fly straight
		{touchTurnDeadDeg - 1, 1, 0},         // inside the angular deadzone: still straight
		{-(touchTurnDeadDeg - 1), 1, 0},      // symmetric
		{touchTurnRampDeg, 1, turnSpeed},     // ramp end, full throw: full rate
		{90, 1, turnSpeed},                   // hard push, full throw: capped at full rate
		{-90, 1, -turnSpeed},                 // capped, signed
		{90, 0, turnSpeed * touchThrowFloor}, // barely-deflected thumb: floor authority only
	}
	for _, c := range cases {
		if got := touchTurnStep(c.delta, c.throw); math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("touchTurnStep(%v, %v) = %v, want %v", c.delta, c.throw, got, c.want)
		}
	}
	// Midway up the ramp the response is proportional: more error, more turn,
	// but strictly under the cap.
	mid := touchTurnStep((touchTurnDeadDeg+touchTurnRampDeg)/2, 1)
	if mid <= 0 || mid >= turnSpeed {
		t.Fatalf("mid-ramp response should be a partial turn, got %v", mid)
	}
	// A half throw commands strictly less than a full throw at the same error.
	if a, b := touchTurnStep(90, 0.5), touchTurnStep(90, 1); a >= b {
		t.Fatalf("half throw (%v) should turn slower than full throw (%v)", a, b)
	}
}

// TestTouchThrow: the radial deflection maps onto turn authority — zero at the
// radial deadzone, full at touchThrowFullPx, clamped outside, DPI-scaled.
func TestTouchThrow(t *testing.T) {
	if got := touchThrow(touchDeadzonePx, 1); got != 0 {
		t.Fatalf("at the deadzone the throw should be 0, got %v", got)
	}
	if got := touchThrow(touchThrowFullPx, 1); got != 1 {
		t.Fatalf("at the full-throw radius the throw should be 1, got %v", got)
	}
	if got := touchThrow(touchThrowFullPx*3, 1); got != 1 {
		t.Fatalf("past full throw it clamps to 1, got %v", got)
	}
	half := touchThrow((touchDeadzonePx+touchThrowFullPx)/2, 1)
	if math.Abs(half-0.5) > 1e-9 {
		t.Fatalf("midway should be half authority, got %v", half)
	}
	if got := touchThrow(touchThrowFullPx*2, 2); math.Abs(got-1) > 1e-9 {
		t.Fatalf("the throw should scale with the DPI, got %v", got)
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
