package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crgimenes/linefire/asset"
)

func TestSnapAllToGrid(t *testing.T) {
	e := newTestEditor()
	e.asset.Origin = asset.Point{X: 9.4, Y: 10.6}
	e.asset.Layers[0].Paths = []asset.Path{{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 31.6, Y: 4.2},
		{Op: asset.OpLineTo, X: 12.4, Y: 53.9},
		{Op: asset.OpClose},
	}}}
	e.asset.Hardpoints = []asset.Hardpoint{{Name: "g", Kind: asset.KindWeapon, X: 7.7, Y: 8.1, Angle: -90}}
	e.asset.Collisions = []asset.CollisionShape{{Kind: asset.CollisionCircle, Points: []asset.Point{{X: 2.4, Y: 2.6}}, Radius: 8.7}}

	e.dirty = false
	e.snapAllToGrid()

	cmds := e.asset.Layers[0].Paths[0].Commands
	checks := []struct {
		got, want float64
		name      string
	}{
		{e.asset.Origin.X, 9, "origin.X"},
		{e.asset.Origin.Y, 11, "origin.Y"},
		{cmds[0].X, 32, "cmd0.X"},
		{cmds[0].Y, 4, "cmd0.Y"},
		{cmds[1].X, 12, "cmd1.X"},
		{cmds[1].Y, 54, "cmd1.Y"},
		{e.asset.Hardpoints[0].X, 8, "hp.X"},
		{e.asset.Hardpoints[0].Y, 8, "hp.Y"},
		{e.asset.Collisions[0].Points[0].X, 2, "collision.X"},
		{e.asset.Collisions[0].Radius, 9, "collision.Radius"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %g, want %g", c.name, c.got, c.want)
		}
	}
	if !e.dirty {
		t.Error("snapAllToGrid should mark the asset dirty")
	}
	// The validated asset must still pass validation.
	err := asset.Validate(e.asset)
	if err != nil {
		t.Fatalf("asset invalid after snap: %v", err)
	}
}

// clickLine simulates a pen click (a corner vertex), closing the draft when the
// point lands on the start.
func clickLine(e *Editor, x, y float64) {
	e.cursor = asset.Point{X: x, Y: y}
	if e.atDraftStart() {
		e.closeLine()
		return
	}
	e.beginLineNode(asset.Point{X: x, Y: y})
	e.finishLineNode(asset.Point{X: x, Y: y})
}

// dragLine simulates a pen click-drag (a smooth vertex): the anchor is at (ax,ay)
// and the drag releases at (ex,ey), which sets the curve handle.
func dragLine(e *Editor, ax, ay, ex, ey float64) {
	e.beginLineNode(asset.Point{X: ax, Y: ay})
	e.cursor = asset.Point{X: ex, Y: ey}
	e.finishLineNode(asset.Point{X: ex, Y: ey})
}

func TestLineToolOpenVsClosed(t *testing.T) {
	e := newTestEditor()
	e.tool = toolLine

	// Finishing with commitDraft (right-click/Enter) leaves an open polyline.
	clickLine(e, 0, 0)
	clickLine(e, 10, 0)
	clickLine(e, 10, 10)
	e.commitDraft()
	paths := e.currentLayer().Paths
	if paths[len(paths)-1].Closed() {
		t.Fatal("expected an open path after commitDraft")
	}

	// Clicking back on the start closes the path.
	clickLine(e, 0, 0)
	clickLine(e, 10, 0)
	clickLine(e, 10, 10)
	clickLine(e, 0, 0)
	paths = e.currentLayer().Paths
	if !paths[len(paths)-1].Closed() {
		t.Fatal("expected a closed path after clicking the start")
	}
}

func TestPenDrawsCurve(t *testing.T) {
	e := newTestEditor()
	e.tool = toolLine
	clickLine(e, 0, 0)         // corner start
	dragLine(e, 50, 0, 50, 30) // drag -> smooth cubic vertex
	if len(e.draft) != 2 {
		t.Fatalf("want 2 draft commands, got %d", len(e.draft))
	}
	if e.draft[1].Op != asset.OpCubicTo || len(e.draft[1].Ctrl) != 2 {
		t.Fatalf("a drag should produce a cubic with two controls, got %+v", e.draft[1])
	}
}

func TestEditCurveControl(t *testing.T) {
	e := newTestEditor()
	e.tool = toolLine
	clickLine(e, 0, 0)
	dragLine(e, 50, 0, 50, 30)
	e.commitDraft()

	var ctrl handle
	found := false
	for _, h := range e.collectHandles() {
		if h.kind == handleControl {
			ctrl, found = h, true
			break
		}
	}
	if !found {
		t.Fatal("a committed curve should expose control handles")
	}

	e.moveHandle(ctrl, 99, 99)
	cmds := e.currentLayer().Paths[0].Commands
	last := cmds[len(cmds)-1]
	moved := false
	for _, cp := range last.Ctrl {
		if cp.X == 99 && cp.Y == 99 {
			moved = true
		}
	}
	if !moved {
		t.Fatalf("dragging the control handle should move the control point: %+v", last.Ctrl)
	}
}

func TestRotateHardpoint(t *testing.T) {
	e := newTestEditor()
	e.asset.Hardpoints = []asset.Hardpoint{{Name: "g", Kind: asset.KindWeapon, Angle: -90}}
	e.active = handle{kind: handleHardpoint, index: 0}
	e.hasActive = true

	e.rotateHardpoint(15)
	if e.asset.Hardpoints[0].Angle != -75 {
		t.Fatalf("angle = %g, want -75", e.asset.Hardpoints[0].Angle)
	}

	// Normalization: 175 + 15 = 190 -> -170.
	e.asset.Hardpoints[0].Angle = 175
	e.rotateHardpoint(15)
	if e.asset.Hardpoints[0].Angle != -170 {
		t.Fatalf("angle = %g, want -170", e.asset.Hardpoints[0].Angle)
	}
}

func TestAdjustStrokeWidth(t *testing.T) {
	e := newTestEditor()
	e.currentLayer().StrokeWidth = 2

	e.adjustStrokeWidth(0.5)
	if e.currentLayer().StrokeWidth != 2.5 {
		t.Fatalf("width = %g, want 2.5", e.currentLayer().StrokeWidth)
	}

	// Clamp at zero.
	e.adjustStrokeWidth(-10)
	if e.currentLayer().StrokeWidth != 0 {
		t.Fatalf("width = %g, want 0", e.currentLayer().StrokeWidth)
	}
}

func TestDeleteSelected(t *testing.T) {
	e := newTestEditor()
	e.asset.Hardpoints = []asset.Hardpoint{{Name: "a", Kind: asset.KindWeapon}}
	e.asset.Collisions = []asset.CollisionShape{{Kind: asset.CollisionCircle, Points: []asset.Point{{}}, Radius: 4}}

	e.active = handle{kind: handleHardpoint, index: 0}
	e.hasActive = true
	e.deleteSelected()
	if len(e.asset.Hardpoints) != 0 {
		t.Fatalf("hardpoint not removed: %d remain", len(e.asset.Hardpoints))
	}

	e.active = handle{kind: handleCollision, shapeIdx: 0}
	e.hasActive = true
	e.deleteSelected()
	if len(e.asset.Collisions) != 0 {
		t.Fatalf("collision not removed")
	}
}

func TestDirtyFlagAndSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	e := New(asset.New(), path)

	if e.dirty {
		t.Fatal("new editor should start clean")
	}
	e.addHardpoint(asset.KindWeapon, 10, 10)
	if !e.dirty {
		t.Fatal("mutation should mark the asset dirty")
	}

	e.save()
	if e.dirty {
		t.Fatal("save should clear the dirty flag")
	}
	_, err := os.Stat(path)
	if err != nil {
		t.Fatalf("save did not write the file: %v", err)
	}
}
