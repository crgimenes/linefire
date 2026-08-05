package mapeditor

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/editorkit"
)

// handleInput processes keyboard and mouse for the current frame.
func (e *MapEditor) handleInput() {
	e.snapDisabled = ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight)
	shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
	cmd := ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight) ||
		ebiten.IsKeyPressed(ebiten.KeyMetaLeft) || ebiten.IsKeyPressed(ebiten.KeyMetaRight)

	mxi, myi := ebiten.CursorPosition()
	mx, my := float64(mxi), float64(myi)
	e.mouseInCanvas = image.Pt(mxi, myi).In(e.canvasRect())
	if e.colorPanelHovered(mx, my) || e.newAssetHovered(mx, my) {
		e.mouseInCanvas = false // an open popup eats clicks over it
	}

	// The new-asset modal owns the keyboard/canvas while open (its buttons and
	// field were already handled this frame); Esc cancels it.
	if e.newAssetOpen {
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			e.closeNewAssetDialog()
		}
		e.lastMouseX, e.lastMouseY = mx, my
		return
	}

	// F5 plays the open map in this window, unsaved edits included; F5 again returns
	// (the host handles that half, since the editor is not running during play). It is
	// a function key, not a text char, so it fires even while a field has focus.
	if inpututil.IsKeyJustPressed(ebiten.KeyF5) {
		e.playtest()
	}

	typing := e.spawnPanel.HasFocus() || e.newAsset.HasFocus()
	if cmd {
		e.handleCommandKeys(shift)
	} else if !typing {
		e.handleKeys(shift)
		e.handleEditKeys(shift)
	}

	space := ebiten.IsKeyPressed(ebiten.KeySpace)
	e.handleCamera(mx, my, space, typing)

	var exclude *handle
	if e.tool == toolSelect && e.dragging {
		exclude = &e.active
	}
	cx, cy := e.resolveCursor(mx, my, exclude)
	e.cursor = asset.Point{X: cx, Y: cy}

	e.handleMouse(mx, my)

	e.lastMouseX = mx
	e.lastMouseY = my
}

// handleKeys handles tool selection and one-shot commands.
func (e *MapEditor) handleKeys(shift bool) {
	switch {
	case inpututil.IsKeyJustPressed(ebiten.Key1):
		e.setTool(toolSelect)
	case inpututil.IsKeyJustPressed(ebiten.Key2):
		e.setTool(toolWall)
	case inpututil.IsKeyJustPressed(ebiten.Key3):
		e.setTool(toolStart)
	case inpututil.IsKeyJustPressed(ebiten.Key4):
		e.setTool(toolSpawn)
	case inpututil.IsKeyJustPressed(ebiten.Key5):
		if e.tool == toolZone {
			e.cycleZoneKind()
		} else {
			e.setTool(toolZone)
		}
	case inpututil.IsKeyJustPressed(ebiten.Key6):
		e.setTool(toolDelete)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		e.cyclePalette()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		e.save() // S: save, or the native name panel when untitled (Cmd+S / Cmd+Shift+S too)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyI) {
		e.pickImportSVG() // I: import an SVG's shapes as walls
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		e.cancelCurrent()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		if e.drafting {
			e.commitWall()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || inpututil.IsKeyJustPressed(ebiten.KeyDelete) {
		e.removeLastOrSelected()
	}
}

// handleEditKeys handles fine editing: rotate, grid/glow toggles, snap. The
// arrow keys pan the camera (see handleCamera), so handles are moved by dragging.
func (e *MapEditor) handleEditKeys(shift bool) {
	if e.hasActive {
		astep := 15.0
		if shift {
			astep = 1.0
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft) {
			e.rotateSelected(-astep)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBracketRight) {
			e.rotateSelected(astep)
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyG) {
		if shift {
			e.level.Editor.SnapToGrid = !e.level.Editor.SnapToGrid
			e.markDirty()
			e.status = "grid snap " + onOff(e.level.Editor.SnapToGrid)
		} else {
			e.level.Editor.GridEnabled = !e.level.Editor.GridEnabled
			e.markDirty()
			e.status = "grid " + onOff(e.level.Editor.GridEnabled)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyB) {
		e.glowEnabled = !e.glowEnabled
		e.status = "glow " + onOff(e.glowEnabled)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyL) {
		e.toggleColorPanel()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		e.snapAllToGrid()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyN) {
		e.preview = !e.preview
		e.status = "negative-space preview " + onOff(e.preview)
	}

	// Reference backdrop (tracing template, never saved): M loads/replaces it; Shift+M shows/hides
	// it; -/= scale, ,/. fade — while one is loaded.
	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		if shift && e.backdrop != nil {
			e.backdropVisible = !e.backdropVisible
			e.status = "reference " + onOff(e.backdropVisible)
		} else if !shift {
			e.pickBackdrop()
		}
	}
	if e.backdrop != nil {
		switch {
		case inpututil.IsKeyJustPressed(ebiten.KeyMinus):
			e.scaleBackdrop(1 / 1.1)
		case inpututil.IsKeyJustPressed(ebiten.KeyEqual):
			e.scaleBackdrop(1.1)
		case inpututil.IsKeyJustPressed(ebiten.KeyComma):
			e.fadeBackdrop(-0.1)
		case inpututil.IsKeyJustPressed(ebiten.KeyPeriod):
			e.fadeBackdrop(0.1)
		}
	}
}

// handleCommandKeys handles Ctrl/Cmd shortcuts.
func (e *MapEditor) handleCommandKeys(shift bool) {
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyZ):
		if shift {
			e.redo()
		} else {
			e.undo()
		}
	case inpututil.IsKeyJustPressed(ebiten.KeyY):
		e.redo()
	case inpututil.IsKeyJustPressed(ebiten.KeyS):
		if shift {
			e.pickSaveMapAs() // Cmd+Shift+S: always name a new copy
		} else {
			e.save() // Cmd+S: write, or prompt for a name if untitled
		}
	case inpututil.IsKeyJustPressed(ebiten.KeyO):
		e.pickOpenMap() // Cmd+O: open a map
	}
}

// handleCamera handles wheel zoom, pan and framing keys. When typing is true a
// text field owns the keyboard, so the key-driven framing and arrow panning are
// suppressed (the wheel and drag-to-pan still work).
func (e *MapEditor) handleCamera(mx, my float64, space, typing bool) {
	_, wy := ebiten.Wheel()
	if wy != 0 && e.mouseInCanvas {
		e.cam.ZoomAt(mx, my, math.Pow(1.1, wy))
	}
	e.panning = ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) ||
		(space && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft))
	if e.panning {
		e.cam.Pan(mx-e.lastMouseX, my-e.lastMouseY)
		e.dragging = false
	}
	if typing {
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.Key0) {
		e.resetView()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		e.frameContent()
	}

	// Arrow keys walk the camera across the unbounded map (held = continuous).
	step := 8.0
	if ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight) {
		step = 24.0
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		e.cam.Pan(step, 0)
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		e.cam.Pan(-step, 0)
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		e.cam.Pan(0, step)
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		e.cam.Pan(0, -step)
	}
}

// handleMouse dispatches clicks to the active tool.
func (e *MapEditor) handleMouse(mx, my float64) {
	if e.panning {
		return
	}
	// Right-click finishes the current wall as an open line.
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) && e.drafting {
		e.commitWall()
		return
	}

	pressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	down := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	released := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)

	// Direct manipulation: an in-progress drag, then a press that lands on a handle,
	// grab it in ANY tool — so a vertex or curve control is editable straight away,
	// without first switching to Select. Suspended while drawing a wall or a zone,
	// where presses add points instead.
	if e.dragging {
		e.continueDrag(down, released, mx, my)
		return
	}
	// Grab-to-move a handle in the manipulation tools. NOT the pen/delete tools: those own their
	// clicks (draw a fresh vertex, delete a node) and must never silently grab a point instead.
	grabTool := e.tool != toolWall && e.tool != toolDelete
	if pressed && e.mouseInCanvas && grabTool && !e.drafting && len(e.zoneDraft) == 0 && e.beginGrab(mx, my) {
		return
	}

	if e.tool == toolWall {
		e.handleWallPen(pressed, released, mx, my)
		return
	}

	if !pressed || !e.mouseInCanvas {
		return
	}
	switch e.tool {
	case toolStart:
		e.level.PlayerStart.X = e.cursor.X
		e.level.PlayerStart.Y = e.cursor.Y
		e.markDirty()
		e.status = "player start set"
	case toolSpawn:
		e.placeSpawn(e.cursor.X, e.cursor.Y)
	case toolZone:
		e.placeZonePoint(e.cursor.X, e.cursor.Y)
	case toolDelete:
		e.deleteAtCursor(mx, my)
	}
}

// continueDrag advances an in-progress handle drag, or resolves it as a JOIN CLICK on release: a
// press on a loose end stays a pending join until the mouse travels past the threshold (kutta's
// click-vs-drag discrimination), so a clean click welds and a real drag moves the endpoint.
func (e *MapEditor) continueDrag(down, released bool, mx, my float64) {
	if e.pendingJoin && math.Hypot(mx-e.pressX, my-e.pressY) > dragThresholdPx {
		e.pendingJoin = false
	}
	if down && !e.pendingJoin && (e.cursor.X != e.active.x || e.cursor.Y != e.active.y) {
		e.moveHandle(e.active, e.cursor.X, e.cursor.Y)
		e.active.x = e.cursor.X
		e.active.y = e.cursor.Y
	}
	if released {
		e.dragging = false
		if e.pendingJoin {
			e.pendingJoin = false
			e.clickLooseEnd(e.active) // a clean click on a red end: arm or complete the join
		}
	}
}

// beginGrab starts a drag from a press: a handle under the cursor is grabbed (a loose end arms a
// pending join), else Shift+click on a wall edge inserts a vertex there and grabs it. Reports
// whether the press was consumed.
func (e *MapEditor) beginGrab(mx, my float64) bool {
	h, ok := e.handleAt(mx, my)
	if ok {
		e.active = h
		e.hasActive = true
		e.dragging = true
		e.pressX, e.pressY = mx, my
		// A press on a LOOSE END is a pending join (kutta: click two red ends to weld); it
		// resolves as a join on release unless the mouse drags past the threshold first.
		_, isEnd := e.wallEndpoint(h)
		e.pendingJoin = e.tool == toolSelect && isEnd
		return true
	}
	// Shift+click on a wall edge inserts a vertex there and grabs it (kutta-style), so a
	// straight run can be bent — or a joined triangle grown back into a square.
	shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
	if e.tool == toolSelect && shift {
		nh, ok := e.insertEdgeVertexAt(mx, my)
		if ok {
			e.active = nh
			e.hasActive = true
			e.dragging = true
			e.pressX, e.pressY = mx, my
			e.pendingJoin = false
			return true
		}
	}
	return false
}

// handleWallPen drives the pen tool: a press starts a vertex, the drag until
// release is its curve handle, a press on the start closes the shape, and a
// double-click finishes it as an open line.
func (e *MapEditor) handleWallPen(pressed, released bool, mx, my float64) {
	if pressed && e.mouseInCanvas {
		switch {
		case e.drafting && e.isDoubleClick(mx, my):
			e.commitWall() // double-click finishes an open wall
		case e.atDraftStart():
			e.closeWall()
		default:
			e.beginWallNode(e.cursor)
			e.recordClick(mx, my)
		}
		return
	}
	if released && e.penActive {
		e.finishWallNode(e.cursor)
	}
}

// resetDraft clears the in-progress wall draft and the pen state.
func (e *MapEditor) resetDraft() {
	e.draft = nil
	e.drafting = false
	e.penActive = false
	e.draftLastHandle = asset.Point{}
}

// setTool switches tools, cancelling drafts that do not carry over.
func (e *MapEditor) setTool(t toolID) {
	if t == e.tool {
		return
	}
	if e.drafting && t != toolWall {
		e.resetDraft()
	}
	if len(e.zoneDraft) > 0 && t != toolZone {
		e.zoneDraft = nil
	}
	e.hasJoinFrom = false // a half-finished join does not carry into another tool
	e.pendingJoin = false
	e.tool = t
	e.status = "tool: " + toolName(t)
}

// cancelCurrent aborts whatever operation is in progress.
func (e *MapEditor) cancelCurrent() {
	switch {
	case e.drafting:
		e.resetDraft()
		e.status = "wall cancelled"
	case len(e.zoneDraft) > 0:
		e.zoneDraft = nil
		e.status = "zone cancelled"
	case e.hasJoinFrom:
		e.hasJoinFrom = false
		e.status = "join cancelled"
	case e.hasActive:
		e.hasActive = false
		e.dragging = false
		e.status = "selection cleared"
	}
}

// removeLastOrSelected drops the last wall draft point, else the selection.
func (e *MapEditor) removeLastOrSelected() {
	if e.drafting {
		if len(e.draft) > 0 {
			e.draft = e.draft[:len(e.draft)-1]
			e.penActive = false
			e.draftLastHandle = asset.Point{} // next segment starts as a corner
			if len(e.draft) == 0 {
				e.drafting = false
			}
			e.status = "removed last point"
		}
		return
	}
	if e.hasActive {
		e.deleteSelected()
		return
	}
	// Nothing selected: cut the node under the cursor (kutta: Del cuts the HOVERED vertex — no
	// select-first, no tool switch).
	mx, my := ebiten.CursorPosition()
	e.deleteAtCursor(float64(mx), float64(my))
}

// isDoubleClick reports whether the current click closely follows the previous
// one in both time and screen position.
func (e *MapEditor) isDoubleClick(mx, my float64) bool {
	return editorkit.DoubleClick(ebiten.Tick(), e.lastClickTick, mx-e.lastClickX, my-e.lastClickY)
}

// recordClick stores the time and position of a click for double-click checks.
func (e *MapEditor) recordClick(mx, my float64) {
	e.lastClickTick = ebiten.Tick()
	e.lastClickX = mx
	e.lastClickY = my
}

// onOff renders a boolean as a short status word.
func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// toolName returns the display name of a tool.
func toolName(t toolID) string {
	switch t {
	case toolSelect:
		return "Select"
	case toolWall:
		return "Wall"
	case toolStart:
		return "Start"
	case toolSpawn:
		return "Spawn"
	case toolZone:
		return "Zone"
	case toolDelete:
		return "Delete node"
	default:
		return "?"
	}
}
