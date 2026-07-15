package editor

import "linefire/asset"

// commitHistory feeds the per-frame change signals to the shared history.
// before is the asset state cloned at the start of the frame.
func (e *Editor) commitHistory(before *asset.Asset) {
	// A focused text field is a continuous gesture too: defer the snapshot until
	// the field loses focus so a rename coalesces into a single undo step.
	busy := e.dragging || e.gui.HasFocus() || e.snd.HasFocus()
	e.hist.Commit(before, e.dirtyThisFrame, busy)
}

// undo restores the previous asset state.
func (e *Editor) undo() {
	doc, ok := e.hist.Undo(e.asset)
	if !ok {
		e.status = "nothing to undo"
		return
	}
	e.asset = doc
	e.afterHistorySwap()
	e.status = "undo"
}

// redo re-applies the most recently undone state.
func (e *Editor) redo() {
	doc, ok := e.hist.Redo(e.asset)
	if !ok {
		e.status = "nothing to redo"
		return
	}
	e.asset = doc
	e.afterHistorySwap()
	e.status = "redo"
}

// afterHistorySwap resets transient editing state whose references may not
// survive an asset swap, and marks the document dirty.
func (e *Editor) afterHistorySwap() {
	e.hasActive = false
	e.dragging = false
	e.drafting = false
	e.draft = nil
	e.collDraft = nil
	e.dirty = true
	e.syncPaletteIndices()
}
