package editorkit

// History is a generic undo/redo stack over deep-copied snapshots of a document
// of type T. It coalesces a continuous gesture (e.g. a drag) into a single step:
// the pre-gesture snapshot is held until the gesture settles.
type History[T any] struct {
	clone  func(T) T
	limit  int
	before *T
	undo   []T
	redo   []T
}

// NewHistory creates a history that snapshots with clone and keeps at most limit
// undo steps.
func NewHistory[T any](clone func(T) T, limit int) *History[T] {
	return &History[T]{clone: clone, limit: limit}
}

// Commit turns per-frame change signals into undo steps. before is the document
// state captured at the start of the frame; changed reports whether the document
// changed this frame; busy is true while a continuous gesture is in progress
// (no step is cut mid-gesture).
func (h *History[T]) Commit(before T, changed, busy bool) {
	if changed {
		if h.before == nil {
			b := before
			h.before = &b
		}
		return
	}
	if h.before == nil || busy {
		return
	}
	h.undo = append(h.undo, *h.before)
	if h.limit > 0 && len(h.undo) > h.limit {
		h.undo = h.undo[len(h.undo)-h.limit:]
	}
	h.redo = h.redo[:0]
	h.before = nil
}

// Undo restores the previous snapshot, pushing current onto the redo stack.
// It returns the restored document and whether anything was undone.
func (h *History[T]) Undo(current T) (T, bool) {
	if len(h.undo) == 0 {
		var zero T
		return zero, false
	}
	h.redo = append(h.redo, h.clone(current))
	last := len(h.undo) - 1
	doc := h.undo[last]
	h.undo = h.undo[:last]
	h.before = nil
	return doc, true
}

// Redo re-applies the most recently undone snapshot.
func (h *History[T]) Redo(current T) (T, bool) {
	if len(h.redo) == 0 {
		var zero T
		return zero, false
	}
	h.undo = append(h.undo, h.clone(current))
	last := len(h.redo) - 1
	doc := h.redo[last]
	h.redo = h.redo[:last]
	h.before = nil
	return doc, true
}

// UndoLen and RedoLen report stack depths (useful for UI and tests).
func (h *History[T]) UndoLen() int { return len(h.undo) }
func (h *History[T]) RedoLen() int { return len(h.redo) }
