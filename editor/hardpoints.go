package editor

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	ui "github.com/crgimenes/minigui"
)

// Hardpoints panel geometry (logical pixels). The backdrop sits at the panel
// origin minus the margin and spans these dimensions.
const (
	hpPanelX      = 12
	hpPanelY      = toolbarHeight + 12
	hpPanelMargin = 8
	hpPanelW      = 224
	hpPanelH      = 240
)

// toggleHardpointPanel shows or hides the hardpoints panel; opening it closes the
// other left-slot panels, and hiding it drops any field focus so suspended
// shortcuts come back.
func (e *Editor) toggleHardpointPanel() {
	e.hpPanel = !e.hpPanel
	if e.hpPanel {
		e.sndPanel = false
		e.colorPanel = false
	} else {
		e.gui.ClearFocus()
	}
	e.status = "hardpoints panel " + onOff(e.hpPanel)
}

// runHardpointPanel drives the ui toolkit for the panel each frame it is open: a
// list of hardpoints plus a name field for the selected one. Selecting in the
// list makes that hardpoint the active handle; editing the field renames it.
func (e *Editor) runHardpointPanel() {
	in := ui.InputFromEbiten()
	e.gui.Begin(in, hpPanelX, hpPanelY)
	e.gui.Label("HARDPOINTS (O)")

	n := len(e.asset.Hardpoints)
	if n == 0 {
		e.gui.ClearFocus()
		e.gui.Label("(none - add with 4 or 5)")
		e.gui.End()
		return
	}
	if e.hpSel >= n {
		e.hpSel = n - 1
	}
	if e.hpSel < 0 {
		e.hpSel = 0
	}

	names := make([]string, n)
	for i := range e.asset.Hardpoints {
		label := e.asset.Hardpoints[i].Name
		if label == "" {
			label = "(unnamed)"
		}
		names[i] = label
	}
	if e.gui.List("hp.list", names, &e.hpSel) {
		e.selectHardpoint(e.hpSel)
	}

	e.gui.Label("name:")
	if e.gui.TextField("hp.name", &e.asset.Hardpoints[e.hpSel].Name) {
		e.markDirty()
	}
	e.gui.End()
}

// selectHardpoint makes the hardpoint at index i the active handle so the canvas
// highlights it and the nudge/rotate keys act on it.
func (e *Editor) selectHardpoint(i int) {
	if i < 0 || i >= len(e.asset.Hardpoints) {
		e.hasActive = false
		return
	}
	hp := e.asset.Hardpoints[i]
	e.active = handle{kind: handleHardpoint, x: hp.X, y: hp.Y, index: i}
	e.hasActive = true
	e.hpSel = i
}

// hpPanelHovered reports whether the cursor is over the open panel, so its clicks
// and scroll are not also consumed by the canvas tools.
func (e *Editor) hpPanelHovered(mx, my float64) bool {
	if !e.hpPanel {
		return false
	}
	x := float64(hpPanelX - hpPanelMargin)
	y := float64(hpPanelY - hpPanelMargin)
	return mx >= x && mx < x+hpPanelW && my >= y && my < y+hpPanelH
}

// drawHardpointPanel renders the panel backdrop and the ui draw commands on top
// of the rest of the editor.
func (e *Editor) drawHardpointPanel(screen *ebiten.Image) {
	x := float32(hpPanelX - hpPanelMargin)
	y := float32(hpPanelY - hpPanelMargin)
	vector.FillRect(screen, x, y, hpPanelW, hpPanelH, colorPanelBg, false)
	vector.StrokeRect(screen, x, y, hpPanelW, hpPanelH, 1, colorBox, false)
	e.gui.Render(screen)
}
