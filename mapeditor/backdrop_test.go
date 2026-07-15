package mapeditor

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"linefire/asset"
	"linefire/level"
)

// TestBackdropFitScaleFade covers the reference-image math: fit-to-Size (aspect-preserving,
// centered), scale-keeps-center, and opacity clamping.
func TestBackdropFitScaleFade(t *testing.T) {
	e := New(level.New(), "", "")
	e.level.Size = asset.Size{W: 1000, H: 600}
	e.backdrop = ebiten.NewImage(200, 100)
	e.backdropAlpha = backdropDefaultAlpha

	e.fitBackdrop()
	if e.backdropScale != 5 { // min(1000/200, 600/100) = 5
		t.Fatalf("fit scale = %.3f, want 5", e.backdropScale)
	}
	if e.backdropPos.X != 0 || e.backdropPos.Y != 50 { // centered in the 1000x600 box
		t.Fatalf("fit pos = %+v, want {0,50}", e.backdropPos)
	}

	// Scaling keeps the image centered rather than drifting to the origin.
	cx0 := e.backdropPos.X + 200*e.backdropScale/2
	cy0 := e.backdropPos.Y + 100*e.backdropScale/2
	e.scaleBackdrop(2)
	if e.backdropScale != 10 {
		t.Fatalf("scale after x2 = %.1f, want 10", e.backdropScale)
	}
	cx1 := e.backdropPos.X + 200*e.backdropScale/2
	cy1 := e.backdropPos.Y + 100*e.backdropScale/2
	if cx1 != cx0 || cy1 != cy0 {
		t.Fatalf("scale drifted the center from (%.0f,%.0f) to (%.0f,%.0f)", cx0, cy0, cx1, cy1)
	}

	// Opacity clamps to [0.1, 1].
	e.backdropAlpha = 0.15
	e.fadeBackdrop(-0.5)
	if e.backdropAlpha != 0.1 {
		t.Fatalf("fade floor = %.2f, want 0.10", e.backdropAlpha)
	}
	e.fadeBackdrop(5)
	if e.backdropAlpha != 1 {
		t.Fatalf("fade ceil = %.2f, want 1", e.backdropAlpha)
	}
}
