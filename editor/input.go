package editor

import (
	"image"
	"math"

	"github.com/crgimenes/devengine/log"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"linefire/asset"
	"linefire/editorkit"
	"linefire/filoio"
)

// handlePanelToggles maps the panel letter keys: O = hardpoints, L = colors,
// U = sounds.
func (e *Editor) handlePanelToggles() {
	if inpututil.IsKeyJustPressed(ebiten.KeyO) {
		e.toggleHardpointPanel()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyL) {
		e.toggleColorPanel()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyU) {
		e.toggleSoundPanel()
	}
}

// handleInput processes keyboard and mouse input for the current frame.
func (e *Editor) handleInput() {
	e.snapDisabled = ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight)
	shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
	cmd := ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight) ||
		ebiten.IsKeyPressed(ebiten.KeyMetaLeft) || ebiten.IsKeyPressed(ebiten.KeyMetaRight)

	mxi, myi := ebiten.CursorPosition()
	mx, my := float64(mxi), float64(myi)
	e.mouseInCanvas = image.Pt(mxi, myi).In(e.canvasRect())

	// Clickable tool bar at the top (above the canvas, so it never collides with
	// the canvas tools).
	e.runToolbar()

	// Floating panels. uiHovered is captured before running them so that a pick
	// which closes a panel does not also let the click fall through to the canvas
	// tools this frame.
	uiHovered := e.hpPanelHovered(mx, my) || e.colorPanelHovered(mx, my) || e.sndPanelHovered(mx, my)
	if e.hpPanel {
		e.runHardpointPanel()
	}
	if e.colorPanel {
		e.runColorPanel()
	}
	if e.sndPanel {
		e.runSoundPanel()
	}
	uiFocused := e.gui.HasFocus() || e.snd.HasFocus()

	// Esc returns to the map editor when hosted (no-op standalone).
	if e.onBack != nil && !uiFocused && inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		e.back()
		return
	}

	// Panel toggles are letter keys (the Fn row is awkward on macOS). They are
	// suppressed while a text field is focused so typing the letter does not also
	// toggle a panel.
	if !cmd && !uiFocused {
		e.handlePanelToggles()
	}

	// While Ctrl/Cmd is held, only the command shortcuts act so bare-key tools
	// are not triggered accidentally.
	if cmd {
		e.handleCommandKeys(shift)
	} else if !uiFocused {
		e.handleShortcuts()
		e.handleEditKeys(shift)
		e.handleLayerKeys(shift)
		e.handleTransformKeys(shift)
	}

	space := ebiten.IsKeyPressed(ebiten.KeySpace)
	if !uiHovered {
		e.handleView(mx, my, space)
	}

	// Resolve the cursor to asset space, excluding a dragged handle from snap.
	var exclude *handle
	if e.tool == toolSelect && e.dragging {
		exclude = &e.active
	}
	cx, cy := e.resolveCursor(mx, my, exclude)

	// Optional Shift constraint for the line tool: lock the new segment to a
	// multiple of 45 degrees relative to the previous point. Re-quantize to the
	// pixel grid afterwards (unless Alt requested sub-pixel placement).
	if e.tool == toolLine && e.drafting && shift && !e.snapActive {
		cx, cy = constrainAngle(e.draft[len(e.draft)-1].X, e.draft[len(e.draft)-1].Y, cx, cy)
		if !e.snapDisabled {
			cx, cy = e.quantizeFree(cx, cy)
		}
	}
	e.cursor = asset.Point{X: cx, Y: cy}

	if !uiHovered {
		e.handleMouse(mx, my)
	}

	e.lastMouseX = mx
	e.lastMouseY = my
}

// handleShortcuts handles the keyboard tool/command shortcuts.
func (e *Editor) handleShortcuts() {
	switch {
	case inpututil.IsKeyJustPressed(ebiten.Key1):
		e.setTool(toolSelect)
	case inpututil.IsKeyJustPressed(ebiten.Key2):
		e.setTool(toolLine)
	case inpututil.IsKeyJustPressed(ebiten.Key3):
		e.setTool(toolOrigin)
	case inpututil.IsKeyJustPressed(ebiten.Key4):
		e.setTool(toolWeapon)
	case inpututil.IsKeyJustPressed(ebiten.Key5):
		e.setTool(toolThruster)
	case inpututil.IsKeyJustPressed(ebiten.Key6):
		// Pressing 6 again while on the collision tool cycles the shape kind.
		if e.tool == toolCollision {
			e.cycleCollKind()
		} else {
			e.setTool(toolCollision)
		}
	case inpututil.IsKeyJustPressed(ebiten.Key7):
		e.setTool(toolDelete)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		e.save()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyC) {
		e.cycleStroke()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyV) {
		e.cycleFill()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyG) {
		e.toggleGrid()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		e.cancelCurrent()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		if e.drafting {
			e.commitDraft()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || inpututil.IsKeyJustPressed(ebiten.KeyDelete) {
		e.removeLastOrSelected()
	}
}

// handleEditKeys handles fine-editing keys: nudging the selected handle,
// rotating a selected hardpoint, adjusting the layer stroke width, and snapping
// the whole asset to the grid.
func (e *Editor) handleEditKeys(shift bool) {
	if e.hasActive {
		step := 1.0
		if shift {
			step = 10.0
		}
		dx, dy := 0.0, 0.0
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) {
			dx = -step
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) {
			dx = step
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
			dy = -step
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
			dy = step
		}
		if dx != 0 || dy != 0 {
			e.moveHandle(e.active, e.active.x+dx, e.active.y+dy)
			e.active.x += dx
			e.active.y += dy
			e.status = "nudged"
		}

		// Hardpoint angle: coarse 15 degrees, fine 1 degree with Shift.
		astep := 15.0
		if shift {
			astep = 1.0
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft) {
			e.rotateHardpoint(-astep)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBracketRight) {
			e.rotateHardpoint(astep)
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) {
		e.adjustStrokeWidth(-0.5)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) {
		e.adjustStrokeWidth(0.5)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		e.snapAllToGrid()
	}
}

// handleTransformKeys handles mirror/rotate of the current transform scope.
// Plain H flips horizontally; Shift+H is layer visibility (handled elsewhere),
// so the flip only fires without Shift.
func (e *Editor) handleTransformKeys(shift bool) {
	if inpututil.IsKeyJustPressed(ebiten.KeyT) {
		e.cycleScope()
	}
	if !shift && inpututil.IsKeyJustPressed(ebiten.KeyH) {
		e.flipHorizontal()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyJ) {
		e.flipVertical()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		if shift {
			e.rotate(15)
		} else {
			e.rotate(90)
		}
	}
}

// handleCommandKeys handles Ctrl/Cmd shortcuts: undo, redo and save.
func (e *Editor) handleCommandKeys(shift bool) {
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
		e.save()
	}
}

// handleLayerKeys handles layer management: selecting, adding, removing,
// reordering, visibility and glow of the current layer.
func (e *Editor) handleLayerKeys(shift bool) {
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		if shift {
			e.selectLayer(-1)
		} else {
			e.selectLayer(1)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyN) {
		if shift {
			e.deleteLayer()
		} else {
			e.addLayer()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
		e.moveLayer(1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
		e.moveLayer(-1)
	}
	if shift && inpututil.IsKeyJustPressed(ebiten.KeyH) {
		e.toggleLayerVisibility()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySemicolon) {
		e.adjustGlow(-0.1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyQuote) {
		e.adjustGlow(0.1)
	}
}

// toggleGrid toggles the background grid, or grid snapping when Shift is held.
func (e *Editor) toggleGrid() {
	shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
	if shift {
		e.asset.Editor.SnapToGrid = !e.asset.Editor.SnapToGrid
		e.markDirty()
		e.status = "grid snap " + onOff(e.asset.Editor.SnapToGrid)
		return
	}
	e.asset.Editor.GridEnabled = !e.asset.Editor.GridEnabled
	e.markDirty()
	e.status = "grid " + onOff(e.asset.Editor.GridEnabled)
}

// isDoubleClick reports whether the current click closely follows the previous
// one in both time and screen position.
func (e *Editor) isDoubleClick(mx, my float64) bool {
	return editorkit.DoubleClick(ebiten.Tick(), e.lastClickTick, mx-e.lastClickX, my-e.lastClickY)
}

// recordClick stores the time and position of a click for double-click checks.
func (e *Editor) recordClick(mx, my float64) {
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

// handleMouse dispatches mouse clicks to the active tool.
func (e *Editor) handleMouse(mx, my float64) {
	// While panning the left button drives the camera, not the tools.
	if e.panning {
		return
	}

	// Right-click finishes the current path as an open line.
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) && e.drafting {
		e.commitDraft()
		return
	}

	pressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	down := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	released := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)

	// Direct manipulation: an in-progress drag, then a press that lands on a handle,
	// grab it in ANY tool — so a vertex or curve control is editable straight away
	// without first switching to Select. Suspended while drawing a path or a
	// collision shape, where presses add points instead.
	if e.dragging {
		// Only mutate when the position actually changes, so a click without a drag
		// does not create an empty undo step.
		if down && (e.cursor.X != e.active.x || e.cursor.Y != e.active.y) {
			e.moveHandle(e.active, e.cursor.X, e.cursor.Y)
			e.active.x = e.cursor.X
			e.active.y = e.cursor.Y
		}
		if released {
			e.dragging = false
		}
		return
	}
	if pressed && e.mouseInCanvas && !e.drafting && len(e.collDraft) == 0 {
		h, ok := e.handleAt(mx, my)
		if ok {
			e.active = h
			e.hasActive = true
			e.dragging = true
			return
		}
	}

	if e.tool == toolLine {
		e.handleLinePen(pressed, released, mx, my)
		return
	}

	if !pressed || !e.mouseInCanvas {
		return
	}

	switch e.tool {
	case toolDelete:
		e.deleteAtCursor(mx, my)
	case toolOrigin:
		e.asset.Origin = e.cursor
		e.markDirty()
		e.status = "origin set"
	case toolWeapon:
		e.addHardpoint(asset.KindWeapon, e.cursor.X, e.cursor.Y)
	case toolThruster:
		e.addHardpoint(asset.KindThruster, e.cursor.X, e.cursor.Y)
	case toolCollision:
		e.placeCollisionPoint(e.cursor.X, e.cursor.Y)
	}
}

// handleLinePen drives the pen tool: a press starts a vertex, the drag until
// release is its curve handle, a press on the start closes the shape, and a
// double-click finishes it as an open line.
func (e *Editor) handleLinePen(pressed, released bool, mx, my float64) {
	if pressed && e.mouseInCanvas {
		switch {
		case e.drafting && e.isDoubleClick(mx, my):
			e.commitDraft() // double-click finishes an open path
		case e.atDraftStart():
			e.closeLine()
		default:
			e.beginLineNode(e.cursor)
			e.recordClick(mx, my)
		}
		return
	}
	if released && e.penActive {
		e.finishLineNode(e.cursor)
	}
}

// setTool switches the active tool, cancelling any operation that does not
// carry over to the new tool.
func (e *Editor) setTool(t toolID) {
	if t == e.tool {
		return
	}
	if e.drafting && t != toolLine {
		e.cancelDraft()
	}
	if len(e.collDraft) > 0 && t != toolCollision {
		e.collDraft = nil
	}
	e.tool = t
	e.status = "tool: " + toolName(t)
}

// cancelCurrent aborts whatever operation is in progress.
func (e *Editor) cancelCurrent() {
	switch {
	case e.drafting:
		e.cancelDraft()
	case len(e.collDraft) > 0:
		e.collDraft = nil
		e.status = "collision cancelled"
	case e.hasActive:
		e.hasActive = false
		e.dragging = false
		e.status = "selection cleared"
	}
}

// removeLastOrSelected removes the last draft point while drawing, otherwise
// the selected vertex.
func (e *Editor) removeLastOrSelected() {
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
	e.deleteSelected()
}

// save writes the asset to disk, reporting the outcome in the status line and
// on the terminal.
func (e *Editor) save() {
	err := filoio.SaveAsset(e.savePath, e.asset)
	if err != nil {
		e.status = "save error: " + err.Error()
		log.Printf("save error: %v", err)
		return
	}
	e.dirty = false
	e.status = "saved " + e.savePath
	log.Printf("saved %s", e.savePath)
}

// constrainAngle snaps the segment from (lx,ly) to (x,y) onto the nearest
// multiple of 45 degrees, preserving its length.
func constrainAngle(lx, ly, x, y float64) (float64, float64) {
	dx := x - lx
	dy := y - ly
	dist := math.Hypot(dx, dy)
	if dist == 0 {
		return x, y
	}
	step := math.Pi / 4
	angle := math.Round(math.Atan2(dy, dx)/step) * step
	return lx + math.Cos(angle)*dist, ly + math.Sin(angle)*dist
}
