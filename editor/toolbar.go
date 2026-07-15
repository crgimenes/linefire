package editor

import (
	ui "github.com/crgimenes/minigui"
)

// runToolbar drives the clickable top tool bar each frame: the seven tools (with
// the active one highlighted), plus save/undo/redo and the grid and hardpoints
// toggles. Toolbar clicks land above the canvas, so they never reach the canvas
// tools. The number-key and letter shortcuts keep working alongside it.
func (e *Editor) runToolbar() {
	in := ui.InputFromEbiten()
	e.bar.Begin(in, 8, 4)

	// When hosted by the map editor, a Back button returns to the map (Esc too).
	if e.onBack != nil {
		if e.bar.Button("back", "< Map") {
			e.back()
		}
		e.bar.SameLine()
	}

	tools := []struct {
		id    ui.ID
		t     toolID
		label string
	}{
		{"t.select", toolSelect, "1 Select"},
		{"t.line", toolLine, "2 Line"},
		{"t.origin", toolOrigin, "3 Origin"},
		{"t.weapon", toolWeapon, "4 Weapon"},
		{"t.thruster", toolThruster, "5 Thruster"},
		{"t.collision", toolCollision, "6 Collision"},
		{"t.delete", toolDelete, "7 Delete"},
	}
	for _, b := range tools {
		if e.bar.Toggle(b.id, b.label, e.tool == b.t) {
			e.setTool(b.t)
		}
		e.bar.SameLine()
	}

	if e.bar.Button("save", "Save") {
		e.save()
	}
	e.bar.SameLine()
	if e.bar.Button("undo", "Undo") {
		e.undo()
	}
	e.bar.SameLine()
	if e.bar.Button("redo", "Redo") {
		e.redo()
	}
	e.bar.SameLine()
	if e.bar.Toggle("grid", "Grid", e.asset.Editor.GridEnabled) {
		e.toggleGrid()
	}
	e.bar.SameLine()
	if e.bar.Toggle("hp", "Hardpoints (O)", e.hpPanel) {
		e.toggleHardpointPanel()
	}
	e.bar.SameLine()
	if e.bar.Toggle("colors", "Colors (L)", e.colorPanel) {
		e.toggleColorPanel()
	}
	e.bar.SameLine()
	if e.bar.Toggle("snd", "Sounds (U)", e.sndPanel) {
		e.toggleSoundPanel()
	}

	e.bar.End()
}
