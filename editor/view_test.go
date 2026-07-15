package editor

import (
	"math"
	"testing"

	"linefire/asset"
	"linefire/editorkit"
)

func TestZoomAtKeepsCursorPointFixed(t *testing.T) {
	e := newTestEditor()
	mx, my := 400.0, 300.0

	ax0, ay0 := e.cam.View.Unproject(mx, my)
	e.cam.ZoomAt(mx, my, 2.0)
	ax1, ay1 := e.cam.View.Unproject(mx, my)

	if math.Abs(ax0-ax1) > 1e-6 || math.Abs(ay0-ay1) > 1e-6 {
		t.Fatalf("asset point under cursor moved: (%g,%g) -> (%g,%g)", ax0, ay0, ax1, ay1)
	}
}

func TestZoomClampsScale(t *testing.T) {
	e := newTestEditor()
	for range 200 {
		e.cam.ZoomAt(400, 300, 2.0)
	}
	if e.cam.View.Scale > editorkit.MaxScale {
		t.Fatalf("scale exceeded max: %g", e.cam.View.Scale)
	}
	for range 200 {
		e.cam.ZoomAt(400, 300, 0.5)
	}
	if e.cam.View.Scale < editorkit.MinScale {
		t.Fatalf("scale below min: %g", e.cam.View.Scale)
	}
}

func TestResetViewFramesSizeBox(t *testing.T) {
	e := newTestEditor() // 64x64 default
	e.cam.View.Scale = 1
	e.resetView()

	// The asset center maps to the canvas center.
	r := e.canvasRect()
	cx, cy := e.cam.View.Project(e.asset.Size.W/2, e.asset.Size.H/2)
	wantX := float32(float64(r.Min.X) + float64(r.Dx())/2)
	wantY := float32(float64(r.Min.Y) + float64(r.Dy())/2)
	if math.Abs(float64(cx-wantX)) > 0.5 || math.Abs(float64(cy-wantY)) > 0.5 {
		t.Fatalf("size box not centered: (%v,%v) want (%v,%v)", cx, cy, wantX, wantY)
	}
	if e.cam.View.Scale <= 0 {
		t.Fatalf("scale must be positive, got %g", e.cam.View.Scale)
	}
}

func TestFrameContentUsesGeometryBounds(t *testing.T) {
	e := newTestEditor()
	e.asset.Layers[0].Paths = []asset.Path{{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 20, Y: 20},
		{Op: asset.OpLineTo, X: 40, Y: 30},
	}}}
	e.frameContent()

	// The content center (30,25) should map to the canvas center.
	r := e.canvasRect()
	cx, cy := e.cam.View.Project(30, 25)
	wantX := float32(float64(r.Min.X) + float64(r.Dx())/2)
	wantY := float32(float64(r.Min.Y) + float64(r.Dy())/2)
	if math.Abs(float64(cx-wantX)) > 0.5 || math.Abs(float64(cy-wantY)) > 0.5 {
		t.Fatalf("content not centered: (%v,%v) want (%v,%v)", cx, cy, wantX, wantY)
	}
}

func TestFrameContentFallsBackWithoutGeometry(t *testing.T) {
	e := newTestEditor() // no paths
	_, _, _, _, ok := e.contentBounds()
	if ok {
		t.Fatal("expected no content bounds on an empty asset")
	}
	e.frameContent() // must not panic and should frame the size box
}
