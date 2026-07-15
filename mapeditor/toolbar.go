package mapeditor

import ui "github.com/crgimenes/minigui"

// runToolbar drives the clickable top tool bar each frame: the tools (with the
// active one highlighted), save/undo/redo, and the grid/glow/negative-preview
// toggles. Toolbar clicks land above the canvas (y < toolbarHeight), so they
// never reach the canvas tools. The keyboard shortcuts keep working alongside it.
func (e *MapEditor) runToolbar() {
	e.bar.Begin(ui.InputFromEbiten(), 8, 4)

	tools := []struct {
		id    ui.ID
		t     toolID
		label string
	}{
		{"t.select", toolSelect, "1 Select"},
		{"t.wall", toolWall, "2 Wall"},
		{"t.start", toolStart, "3 Start"},
		{"t.spawn", toolSpawn, "4 Spawn"},
		{"t.zone", toolZone, "5 Zone"},
		{"t.delete", toolDelete, "6 Del node"},
	}
	// Row 1 — tools + edit actions.
	for _, b := range tools {
		if e.bar.Toggle(b.id, b.label, e.tool == b.t) {
			if b.t == toolZone && e.tool == toolZone {
				e.cycleZoneKind() // clicking Zone again cycles rect/circle
			} else {
				e.setTool(b.t)
			}
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
	if e.bar.Button("delete", "Del") {
		e.deleteSelectedOrHint()
	}
	// No SameLine here: the next button drops to ROW 2 (view + content).

	if e.bar.Toggle("grid", "Grid", e.level.Editor.GridEnabled) {
		e.level.Editor.GridEnabled = !e.level.Editor.GridEnabled
		e.markDirty()
	}
	e.bar.SameLine()
	if e.bar.Toggle("glow", "Glow", e.glowEnabled) {
		e.glowEnabled = !e.glowEnabled
	}
	e.bar.SameLine()
	if e.bar.Toggle("color", "Color", e.colorOpen) {
		e.toggleColorPanel()
	}
	e.bar.SameLine()
	if e.bar.Toggle("preview", "Negative (N)", e.preview) {
		e.preview = !e.preview
	}
	e.bar.SameLine()
	if e.bar.Button("fit", "Fit") {
		e.frameContent()
	}
	e.bar.SameLine()
	if e.bar.Button("newasset", "New asset") {
		e.openNewAssetDialog()
	}
	e.bar.SameLine()
	if e.bar.Button("importsvg", "Import SVG…") {
		e.pickImportSVG()
	}
	e.bar.SameLine()
	// The tracing reference image (M): loads behind the canvas, never saved into the map.
	refLabel := "Ref img…"
	if e.backdrop != nil {
		refLabel = "Ref img ✓"
	}
	if e.bar.Button("refimg", refLabel) {
		e.pickBackdrop()
	}
	e.bar.SameLine()
	if e.bar.Button("assetsdir", "Assets dir…") {
		e.pickAssetsDir()
	}
	e.bar.SameLine()
	// The palette label is variable-width (the asset name), so it goes LAST — if anything runs to
	// the edge it is this, and Tab cycles the palette too.
	asset := "asset: none"
	entry, ok := e.currentPaletteEntry()
	if ok {
		asset = "asset: " + entry.name + " [" + assetKind(entry) + "]"
	}
	if e.bar.Button("palette", asset) { // click cycles the spawn asset (like Tab)
		e.cyclePalette()
	}

	e.bar.End()
}
