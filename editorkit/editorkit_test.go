package editorkit

import (
	"image"
	"math"
	"testing"

	"linefire/render"
)

func TestVisibleStep(t *testing.T) {
	cases := []struct {
		name               string
		step, scale, minPx float64
		want               float64
	}{
		{"zoomed in keeps step", 16, 1, 8, 16},      // 16px on screen >= 8
		{"just at threshold", 16, 0.5, 8, 16},       // 8px exactly
		{"zoomed out coarsens", 16, 0.25, 8, 32},    // 4px -> double to 8
		{"far out coarsens more", 16, 0.05, 8, 256}, // 16->32->64->128->256: 256*0.05=12.8>=8
		{"non-positive step", 0, 1, 8, 0},
		{"non-positive scale", 16, 0, 8, 0},
	}
	for _, c := range cases {
		got := visibleStep(c.step, c.scale, c.minPx)
		if got != c.want {
			t.Errorf("%s: visibleStep(%g,%g,%g) = %g, want %g", c.name, c.step, c.scale, c.minPx, got, c.want)
		}
	}
}

func TestCameraZoomKeepsPointFixed(t *testing.T) {
	c := &Camera{View: render.View{OffsetX: 10, OffsetY: 20, Scale: 1}}
	ax0, ay0 := c.View.Unproject(100, 80)
	c.ZoomAt(100, 80, 2)
	ax1, ay1 := c.View.Unproject(100, 80)
	if math.Abs(ax0-ax1) > 1e-6 || math.Abs(ay0-ay1) > 1e-6 {
		t.Fatalf("point under cursor moved: (%g,%g) -> (%g,%g)", ax0, ay0, ax1, ay1)
	}
}

func TestClampScale(t *testing.T) {
	if ClampScale(1000) != MaxScale {
		t.Errorf("expected clamp to MaxScale")
	}
	if ClampScale(0.001) != MinScale {
		t.Errorf("expected clamp to MinScale")
	}
	if ClampScale(4) != 4 {
		t.Errorf("in-range scale should pass through")
	}
}

func TestCameraCustomZoomRange(t *testing.T) {
	c := &Camera{View: render.View{Scale: 1}, MinScale: 0.01, MaxScale: 256}

	for range 500 {
		c.ZoomAt(0, 0, 0.5)
	}
	if c.View.Scale > 0.011 {
		t.Fatalf("custom min not respected: %g", c.View.Scale)
	}

	c.View.Scale = 1
	for range 500 {
		c.ZoomAt(0, 0, 2)
	}
	if c.View.Scale < 100 { // well past the default max of 64
		t.Fatalf("custom max not respected: %g", c.View.Scale)
	}
}

func TestFitBoxCenters(t *testing.T) {
	c := &Camera{}
	region := image.Rect(0, 0, 200, 200)
	c.FitBox(region, 0, 0, 100, 100)

	cx, cy := c.View.Project(50, 50) // box center -> region center
	if math.Abs(float64(cx)-100) > 0.5 || math.Abs(float64(cy)-100) > 0.5 {
		t.Fatalf("box not centered: (%v,%v) want (100,100)", cx, cy)
	}
}

func TestHistoryUndoRedo(t *testing.T) {
	h := NewHistory(func(x int) int { return x }, 10)

	// One settled edit from state 1.
	h.Commit(1, true, false)   // change happened this frame
	h.Commit(99, false, false) // idle frame settles the step
	if h.UndoLen() != 1 {
		t.Fatalf("expected 1 undo step, got %d", h.UndoLen())
	}

	got, ok := h.Undo(2) // current state is 2
	if !ok || got != 1 {
		t.Fatalf("undo = (%d,%v), want (1,true)", got, ok)
	}
	if h.RedoLen() != 1 {
		t.Fatalf("redo should hold 1, got %d", h.RedoLen())
	}

	got, ok = h.Redo(1)
	if !ok || got != 2 {
		t.Fatalf("redo = (%d,%v), want (2,true)", got, ok)
	}
}

func TestHistoryCoalescesWhileBusy(t *testing.T) {
	h := NewHistory(func(x int) int { return x }, 10)

	// Multiple changed frames while busy (a drag) must not commit.
	h.Commit(1, true, true)
	h.Commit(1, true, true)
	h.Commit(1, false, true) // still busy, no change this frame
	if h.UndoLen() != 0 {
		t.Fatalf("must not commit while busy, got %d", h.UndoLen())
	}
	h.Commit(1, false, false) // gesture released and settled
	if h.UndoLen() != 1 {
		t.Fatalf("a gesture should be one step, got %d", h.UndoLen())
	}
}

func TestDoubleClick(t *testing.T) {
	// Close in time and position -> double-click.
	if !DoubleClick(105, 100, 2, 1) {
		t.Error("expected a double-click for a close, quick second click")
	}
	// Too slow.
	if DoubleClick(200, 100, 0, 0) {
		t.Error("a slow second click is not a double-click")
	}
	// Too far.
	if DoubleClick(102, 100, 20, 20) {
		t.Error("a distant second click is not a double-click")
	}
	// Same tick (dt == 0) does not count.
	if DoubleClick(100, 100, 0, 0) {
		t.Error("a zero time gap is not a double-click")
	}
}

func TestHistoryEmptyUndo(t *testing.T) {
	h := NewHistory(func(x int) int { return x }, 10)
	_, ok := h.Undo(5)
	if ok {
		t.Fatal("undo on empty history should report false")
	}
}
