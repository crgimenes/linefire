package editor

import (
	"math"
	"testing"

	"linefire/asset"
)

// newTestEditor builds an editor over a fresh asset for pure (non-GUI) tests.
func newTestEditor() *Editor {
	return New(asset.New(), "")
}

func TestResolveCursorQuantizesToPixelGrid(t *testing.T) {
	e := newTestEditor()
	// Default asset has no paths; the only handle is the origin at (32,32),
	// which the chosen target stays far away from, so no snap happens.
	view := e.canvasView()

	// Aim at a deliberately fractional asset coordinate.
	wantX, wantY := 2.0, 4.0
	sx, sy := view.Project(2.4, 3.6)

	gotX, gotY := e.resolveCursor(float64(sx), float64(sy), nil)

	if e.snapActive {
		t.Fatalf("did not expect a snap for a free coordinate")
	}
	if gotX != wantX || gotY != wantY {
		t.Fatalf("got (%g,%g), want (%g,%g)", gotX, gotY, wantX, wantY)
	}
	if gotX != math.Trunc(gotX) || gotY != math.Trunc(gotY) {
		t.Fatalf("result must be integer-valued, got (%g,%g)", gotX, gotY)
	}
}

func TestResolveCursorSnapsToExistingVertex(t *testing.T) {
	e := newTestEditor()
	e.asset.Layers[0].Paths = []asset.Path{{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 10, Y: 10},
		{Op: asset.OpLineTo, X: 20, Y: 40},
	}}}
	view := e.canvasView()

	// Place the mouse a few screen pixels off the vertex but inside the radius.
	sx, sy := view.Project(10, 10)
	gotX, gotY := e.resolveCursor(float64(sx)+3, float64(sy)-2, nil)

	if !e.snapActive {
		t.Fatalf("expected a snap near an existing vertex")
	}
	if gotX != 10 || gotY != 10 {
		t.Fatalf("got (%g,%g), want (10,10)", gotX, gotY)
	}
}

func TestResolveCursorAltGivesSubPixel(t *testing.T) {
	e := newTestEditor()
	e.snapDisabled = true
	view := e.canvasView()

	rawX, rawY := 2.4, 3.6
	sx, sy := view.Project(rawX, rawY)

	gotX, gotY := e.resolveCursor(float64(sx), float64(sy), nil)

	if e.snapActive {
		t.Fatalf("Alt must disable snapping")
	}
	// Should preserve the sub-pixel coordinate (within float32 projection error).
	if math.Abs(gotX-rawX) > 0.01 || math.Abs(gotY-rawY) > 0.01 {
		t.Fatalf("got (%g,%g), want ~(%g,%g)", gotX, gotY, rawX, rawY)
	}
}

func TestQuantizeFree(t *testing.T) {
	e := newTestEditor()
	e.asset.Editor.GridSize = 4

	// Grid snapping off: round to the integer pixel grid.
	e.asset.Editor.SnapToGrid = false
	x, y := e.quantizeFree(9.2, 9.2)
	if x != 9 || y != 9 {
		t.Fatalf("pixel grid: got (%g,%g), want (9,9)", x, y)
	}

	// Grid snapping on: round to the nearest multiple of grid_size.
	e.asset.Editor.SnapToGrid = true
	x, y = e.quantizeFree(9.2, 9.2)
	if x != 8 || y != 8 {
		t.Fatalf("grid snap: got (%g,%g), want (8,8)", x, y)
	}
}

func TestResolveCursorSnapsToGridMultiples(t *testing.T) {
	e := newTestEditor()
	e.asset.Editor.GridSize = 4
	e.asset.Editor.SnapToGrid = true
	view := e.canvasView()

	// Aim near (9,9); with a 4px grid the free coordinate lands on (8,8).
	sx, sy := view.Project(9, 9)
	gotX, gotY := e.resolveCursor(float64(sx), float64(sy), nil)

	if gotX != 8 || gotY != 8 {
		t.Fatalf("got (%g,%g), want (8,8)", gotX, gotY)
	}
}

func TestResolveCursorExcludesDraggedHandle(t *testing.T) {
	e := newTestEditor()
	e.asset.Layers[0].Paths = []asset.Path{{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 10, Y: 10},
	}}}
	view := e.canvasView()

	// The dragged handle is the vertex itself; excluding it must prevent the
	// cursor from snapping onto its own position.
	dragged := handle{kind: handleVertex, x: 10, y: 10, layer: 0, path: 0, cmd: 0}
	sx, sy := view.Project(10, 10)
	gotX, gotY := e.resolveCursor(float64(sx), float64(sy), &dragged)

	if e.snapActive {
		t.Fatalf("a dragged handle must not snap to itself")
	}
	if gotX != 10 || gotY != 10 {
		t.Fatalf("got (%g,%g), want quantized (10,10)", gotX, gotY)
	}
}
