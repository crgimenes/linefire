package editor

import (
	"testing"

	"linefire/asset"
)

// frame simulates one Update tick: snapshot the asset, run the edit (if any),
// then let the history logic decide whether to commit a step.
func frame(e *Editor, fn func()) {
	before := e.asset.Clone()
	e.dirtyThisFrame = false
	if fn != nil {
		fn()
	}
	e.commitHistory(before)
}

// edit performs a discrete change followed by an idle frame so the change-group
// settles into a single undo step.
func edit(e *Editor, fn func()) {
	frame(e, fn)
	frame(e, nil)
}

func TestUndoRedo(t *testing.T) {
	e := newTestEditor()
	orig := len(e.asset.Hardpoints)

	edit(e, func() { e.addHardpoint(asset.KindWeapon, 1, 1) })
	if e.hist.UndoLen() != 1 {
		t.Fatalf("expected 1 undo step, got %d", e.hist.UndoLen())
	}
	if len(e.asset.Hardpoints) != orig+1 {
		t.Fatalf("edit not applied")
	}

	e.undo()
	if len(e.asset.Hardpoints) != orig {
		t.Fatalf("undo did not restore hardpoints")
	}
	if e.hist.RedoLen() != 1 {
		t.Fatalf("redo stack should hold 1, got %d", e.hist.RedoLen())
	}

	e.redo()
	if len(e.asset.Hardpoints) != orig+1 {
		t.Fatalf("redo did not re-apply the edit")
	}
}

func TestNewEditClearsRedo(t *testing.T) {
	e := newTestEditor()
	edit(e, func() { e.addHardpoint(asset.KindWeapon, 1, 1) })
	e.undo()
	if e.hist.RedoLen() != 1 {
		t.Fatalf("precondition: redo should have 1 entry")
	}
	edit(e, func() { e.addHardpoint(asset.KindThruster, 2, 2) })
	if e.hist.RedoLen() != 0 {
		t.Fatalf("a new edit must clear the redo stack, got %d", e.hist.RedoLen())
	}
}

func TestDragCoalescesIntoOneStep(t *testing.T) {
	e := newTestEditor()
	e.asset.Layers[0].Paths = []asset.Path{{Commands: []asset.Command{
		{Op: asset.OpMoveTo, X: 10, Y: 10},
	}}}
	e.active = handle{kind: handleVertex, x: 10, y: 10, layer: 0, path: 0, cmd: 0}
	e.hasActive = true
	e.dragging = true

	// Several frames of dragging the same handle.
	frame(e, func() { e.moveHandle(e.active, 11, 10); e.active.x = 11 })
	frame(e, func() { e.moveHandle(e.active, 12, 10); e.active.x = 12 })
	frame(e, nil) // still holding, no movement
	if e.hist.UndoLen() != 0 {
		t.Fatalf("must not commit while dragging, got %d", e.hist.UndoLen())
	}

	e.dragging = false
	frame(e, nil) // release settles the drag
	if e.hist.UndoLen() != 1 {
		t.Fatalf("a drag should be a single undo step, got %d", e.hist.UndoLen())
	}

	e.undo()
	if e.asset.Layers[0].Paths[0].Commands[0].X != 10 {
		t.Fatalf("undo should restore the pre-drag position, got %g",
			e.asset.Layers[0].Paths[0].Commands[0].X)
	}
}

func TestUndoOnEmptyHistory(t *testing.T) {
	e := newTestEditor()
	e.undo() // must not panic
	if e.hist.UndoLen() != 0 || e.hist.RedoLen() != 0 {
		t.Fatal("undo on empty history should be a no-op")
	}
}
